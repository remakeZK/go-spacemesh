package localsql

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/natefinch/atomic"
	"github.com/spacemeshos/post/initialization"
	"go.uber.org/zap"

	"github.com/spacemeshos/go-spacemesh/codec"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/sql"
)

var mainnetPoet2ServiceID []byte

func init() {
	var err error
	mainnetPoet2ServiceID, err = base64.StdEncoding.DecodeString("8RXEI0MwO3uJUINFFlOm/uTjJCneV9FidMpXmn55G8Y=")
	if err != nil {
		panic(fmt.Errorf("failed to decode mainnet poet 2 service id: %w", err))
	}
}

func New0003Migration(log *zap.Logger, dataDir string, poetClients []PoetClient) *migration0003 {
	return &migration0003{
		logger:      log,
		dataDir:     dataDir,
		poetClients: poetClients,
	}
}

type migration0003 struct {
	logger      *zap.Logger
	dataDir     string
	poetClients []PoetClient
}

func (migration0003) Name() string {
	return "add nipost builder state"
}

func (migration0003) Order() int {
	return 3
}

func (m migration0003) Rollback() error {
	filename := filepath.Join(m.dataDir, builderFilename)
	// skip if file exists
	if _, err := os.Stat(filename); err == nil {
		return nil
	}

	backupName := fmt.Sprintf("%s.bak", filename)
	if err := atomic.ReplaceFile(backupName, filename); err != nil {
		return fmt.Errorf("rolling back nipost builder state: %w", err)
	}
	return nil
}

func (m migration0003) Apply(db sql.Executor) error {
	_, err := db.Exec("ALTER TABLE nipost RENAME TO challenge;", nil, nil)
	if err != nil {
		return fmt.Errorf("rename nipost table: %w", err)
	}

	_, err = db.Exec("ALTER TABLE challenge ADD COLUMN poet_proof_ref CHAR(32);", nil, nil)
	if err != nil {
		return fmt.Errorf("add poet_proof_ref column to challenge table: %w", err)
	}

	_, err = db.Exec("ALTER TABLE challenge ADD COLUMN poet_proof_membership VARCHAR;", nil, nil)
	if err != nil {
		return fmt.Errorf("add poet_proof_membership column to challenge table: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE poet_registration (
	    id            CHAR(32) NOT NULL,
	    hash          CHAR(32) NOT NULL,
	    address       VARCHAR NOT NULL,
	    round_id      VARCHAR NOT NULL,
	    round_end     INT NOT NULL,

	    PRIMARY KEY (id, address)
	) WITHOUT ROWID;`, nil, nil); err != nil {
		return fmt.Errorf("create poet_registration table: %w", err)
	}

	if _, err := db.Exec(`CREATE TABLE nipost (
		id            CHAR(32) PRIMARY KEY,
		post_nonce    UNSIGNED INT NOT NULL,
		post_indices  VARCHAR NOT NULL,
		post_pow      UNSIGNED LONG INT NOT NULL,

		num_units UNSIGNED INT NOT NULL,
		vrf_nonce UNSIGNED LONG INT NOT NULL,

		poet_proof_membership VARCHAR NOT NULL,
		poet_proof_ref        CHAR(32) NOT NULL,
		labels_per_unit       UNSIGNED INT NOT NULL
	) WITHOUT ROWID;`, nil, nil); err != nil {
		return fmt.Errorf("create nipost table: %w", err)
	}

	return m.moveNipostStateToDb(db, m.dataDir)
}

func (m migration0003) moveNipostStateToDb(db sql.Executor, dataDir string) error {
	state, err := loadBuilderState(dataDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil // no state to migrate
	case err != nil:
		return fmt.Errorf("load nipost builder state: %w", err)
	}

	meta, err := initialization.LoadMetadata(dataDir)
	if err != nil {
		return fmt.Errorf("load post metadata: %w", err)
	}

	challenge, err := m.getChallengeHash(db, types.BytesToNodeID(meta.NodeId))
	switch {
	case errors.Is(err, sql.ErrNotFound):
		m.logger.Warn("nipost state on disk but no challenge in database - skipping migration",
			zap.String("state_challenge", state.Challenge.ShortString()),
			zap.String("node_id", types.BytesToNodeID(meta.NodeId).ShortString()),
		)
		return discardBuilderState(dataDir)
	case err != nil:
		return fmt.Errorf("get challenge hash: %w", err)
	}

	if !bytes.Equal(challenge.Bytes(), state.Challenge.Bytes()) {
		m.logger.Warn("challenge mismatch - discarding builder state and skipping migration",
			zap.String("node_id", types.BytesToNodeID(meta.NodeId).ShortString()),
			zap.String("challenge", challenge.ShortString()),
			zap.String("state_challenge", state.Challenge.ShortString()),
		)
		return discardBuilderState(dataDir)
	}

	if len(state.PoetRequests) == 0 {
		return discardBuilderState(dataDir) // Phase 0: Submit PoET challenge to PoET services
	}
	// Phase 0 completed.
	for _, req := range state.PoetRequests {
		if bytes.Equal(req.PoetServiceID.ServiceID, mainnetPoet2ServiceID) {
			m.logger.Info("PoET `mainnet-poet-2.spacemesh.network` has been retired - skipping")
			continue
		}

		address, err := m.getAddress(req.PoetServiceID)
		if err != nil {
			return fmt.Errorf("get address for poet service id %x: %w", req.PoetServiceID.ServiceID, err)
		}

		enc := func(stmt *sql.Statement) {
			stmt.BindBytes(1, meta.NodeId)
			stmt.BindBytes(2, state.Challenge.Bytes())
			stmt.BindText(3, address)
			stmt.BindText(4, req.PoetRound.ID)
			stmt.BindInt64(5, req.PoetRound.End.IntoTime().Unix())
		}
		if _, err := db.Exec(`
			insert into poet_registration (id, hash, address, round_id, round_end)
			values (?1, ?2, ?3, ?4, ?5);`, enc, nil,
		); err != nil {
			return fmt.Errorf("insert poet registration for %s: %w", types.BytesToNodeID(meta.NodeId).ShortString(), err)
		}

		m.logger.Info("PoET registration added to database",
			zap.String("node_id", types.BytesToNodeID(meta.NodeId).ShortString()),
			zap.String("poet_service_id", base64.StdEncoding.EncodeToString(req.PoetServiceID.ServiceID)),
			zap.String("address", address),
			zap.String("round_id", req.PoetRound.ID),
			zap.Time("round_end", req.PoetRound.End.IntoTime()),
		)
	}

	if state.PoetProofRef == types.EmptyPoetProofRef {
		// Phase 1: query PoET services for proof
		return discardBuilderState(dataDir)
	}
	// Phase 1 completed.
	buf, err := codec.Encode(&state.NIPost.Membership)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	enc := func(stmt *sql.Statement) {
		stmt.BindBytes(1, meta.NodeId)
		stmt.BindBytes(2, state.PoetProofRef[:])
		stmt.BindBytes(3, buf)
	}
	rows, err := db.Exec(`
		update challenge set poet_proof_ref = ?2, poet_proof_membership = ?3
		where id = ?1 returning id;`, enc, nil)
	if err != nil {
		return fmt.Errorf("set poet proof ref for node id %s: %w", types.BytesToNodeID(meta.NodeId).ShortString(), err)
	}
	if rows == 0 {
		return fmt.Errorf("set poet proof ref for node id %s: %w",
			types.BytesToNodeID(meta.NodeId).ShortString(), sql.ErrNotFound,
		)
	}

	if state.NIPost.Post == nil {
		return discardBuilderState(dataDir) // Phase 2: Post execution
	}
	// Phase 2 completed.
	buf, err = codec.Encode(&state.NIPost.Membership)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}

	enc = func(stmt *sql.Statement) {
		stmt.BindBytes(1, meta.NodeId)
		stmt.BindInt64(2, int64(state.NIPost.Post.Nonce))
		stmt.BindBytes(3, state.NIPost.Post.Indices)
		stmt.BindInt64(4, int64(state.NIPost.Post.Pow))

		stmt.BindInt64(5, int64(meta.NumUnits))
		stmt.BindInt64(6, int64(*meta.Nonce))

		stmt.BindBytes(7, buf)

		stmt.BindBytes(8, state.NIPost.PostMetadata.Challenge)
		stmt.BindInt64(9, int64(state.NIPost.PostMetadata.LabelsPerUnit))
	}
	if _, err := db.Exec(`
		insert into nipost (id, post_nonce, post_indices, post_pow, num_units, vrf_nonce,
			 poet_proof_membership, poet_proof_ref, labels_per_unit
		) values (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9);`, enc, nil,
	); err != nil {
		return fmt.Errorf("insert nipost for %s: %w", types.BytesToNodeID(meta.NodeId).ShortString(), err)
	}

	return discardBuilderState(dataDir)
}

func (m migration0003) getAddress(serviceID PoetServiceID) (string, error) {
	for _, client := range m.poetClients {
		clientId := client.PoetServiceID(context.Background())
		if bytes.Equal(serviceID.ServiceID, clientId) {
			return client.Address(), nil
		}
	}
	return "", fmt.Errorf("no poet client found for service id %x", serviceID.ServiceID)
}

func (m migration0003) getChallengeHash(db sql.Executor, nodeID types.NodeID) (types.Hash32, error) {
	var ch *types.NIPostChallenge
	enc := func(stmt *sql.Statement) {
		stmt.BindBytes(1, nodeID.Bytes())
	}
	dec := func(stmt *sql.Statement) bool {
		ch = &types.NIPostChallenge{}
		ch.PublishEpoch = types.EpochID(stmt.ColumnInt64(0))
		ch.Sequence = uint64(stmt.ColumnInt64(1))
		stmt.ColumnBytes(2, ch.PrevATXID[:])
		stmt.ColumnBytes(3, ch.PositioningATX[:])
		ch.CommitmentATX = &types.ATXID{}
		if n := stmt.ColumnBytes(4, ch.CommitmentATX[:]); n == 0 {
			ch.CommitmentATX = nil
		}
		if n := stmt.ColumnLen(6); n > 0 {
			ch.InitialPost = &types.Post{
				Nonce:   uint32(stmt.ColumnInt64(5)),
				Indices: make([]byte, n),
				Pow:     uint64(stmt.ColumnInt64(7)),
			}
			stmt.ColumnBytes(6, ch.InitialPost.Indices)
		}
		return true
	}
	if _, err := db.Exec(`
		select epoch, sequence, prev_atx, pos_atx, commit_atx,
			post_nonce, post_indices, post_pow
		from challenge where id = ?1 limit 1;`, enc, dec,
	); err != nil {
		return types.Hash32{}, fmt.Errorf("get challenge from node id %s: %w", nodeID.ShortString(), err)
	}
	if ch == nil {
		return types.Hash32{}, fmt.Errorf("get challenge from node id %s: %w", nodeID.ShortString(), sql.ErrNotFound)
	}
	return ch.Hash(), nil
}
