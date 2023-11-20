package nipost

import (
	"testing"

	"github.com/spacemeshos/post/shared"
	"github.com/stretchr/testify/require"

	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/sql"
	"github.com/spacemeshos/go-spacemesh/sql/localsql"
)

func Test_AddInitialPost(t *testing.T) {
	db := localsql.InMemory(
		sql.WithMigration(localsql.New0002Migration(t.TempDir())),
	)

	nodeID := types.RandomNodeID()
	post := Post{
		Nonce:     1,
		Indices:   []byte{1, 2, 3},
		Pow:       1,
		Challenge: shared.ZeroChallenge,

		NumUnits:      2,
		CommitmentATX: types.RandomATXID(),
		VRFNonce:      3,
	}
	err := AddPost(db, nodeID, post)
	require.NoError(t, err)

	got, err := GetPost(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, post, *got)
}

func Test_OverwritePost(t *testing.T) {
	db := localsql.InMemory(
		sql.WithMigration(localsql.New0002Migration(t.TempDir())),
	)

	nodeID := types.RandomNodeID()
	post := Post{
		Nonce:     1,
		Indices:   []byte{1, 2, 3},
		Pow:       1,
		Challenge: shared.ZeroChallenge,

		NumUnits:      2,
		CommitmentATX: types.RandomATXID(),
		VRFNonce:      3,
	}
	require.NoError(t, AddPost(db, nodeID, post))

	got, err := GetPost(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, post, *got)

	// Overwrite
	post2 := Post{
		Nonce:     11,
		Indices:   []byte{4, 5, 6},
		Pow:       11,
		Challenge: []byte("challenge"),

		NumUnits:      22,
		CommitmentATX: types.RandomATXID(),
		VRFNonce:      33,
	}
	require.NoError(t, AddPost(db, nodeID, post2))

	got, err = GetPost(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, post2, *got)
}

func Test_AddChallenge(t *testing.T) {
	commitmentATX := types.RandomATXID()
	tt := []struct {
		name string
		ch   *types.NIPostChallenge
	}{
		{
			name: "nil commitment ATX & post",
			ch: &types.NIPostChallenge{
				PublishEpoch:   4,
				Sequence:       0,
				PrevATXID:      types.RandomATXID(),
				PositioningATX: types.RandomATXID(),
				CommitmentATX:  &commitmentATX,
				InitialPost:    &types.Post{Nonce: 1, Indices: []byte{1, 2, 3}, Pow: 1},
			},
		},
		{
			name: "commitment and initial post",
			ch: &types.NIPostChallenge{
				PublishEpoch:   77,
				Sequence:       13,
				PrevATXID:      types.RandomATXID(),
				PositioningATX: types.RandomATXID(),
				CommitmentATX:  nil,
				InitialPost:    nil,
			},
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			db := localsql.InMemory()

			nodeID := types.RandomNodeID()
			err := AddChallenge(db, nodeID, tc.ch)
			require.NoError(t, err)

			challenge, err := Challenge(db, nodeID)
			require.NoError(t, err)
			require.NotNil(t, challenge)
			require.Equal(t, tc.ch, challenge)

			err = RemoveChallenge(db, nodeID)
			require.NoError(t, err)

			challenge, err = Challenge(db, nodeID)
			require.ErrorIs(t, err, sql.ErrNotFound)
			require.Nil(t, challenge)
		})
	}
}

func Test_AddState_NoDuplicates(t *testing.T) {
	db := localsql.InMemory()

	ch1 := &types.NIPostChallenge{
		PublishEpoch:   4,
		Sequence:       2,
		PrevATXID:      types.RandomATXID(),
		PositioningATX: types.RandomATXID(),
		CommitmentATX:  nil,
		InitialPost:    nil,
	}
	ch2 := &types.NIPostChallenge{
		PublishEpoch:   4,
		Sequence:       3,
		PrevATXID:      types.RandomATXID(),
		PositioningATX: types.RandomATXID(),
		CommitmentATX:  nil,
		InitialPost:    nil,
	}

	nodeID := types.RandomNodeID()
	err := AddChallenge(db, nodeID, ch1)
	require.NoError(t, err)

	// fail to add challenge for same node
	err = AddChallenge(db, nodeID, ch2)
	require.Error(t, err)

	// succeed to add challenge for different node
	err = AddChallenge(db, types.RandomNodeID(), ch2)
	require.NoError(t, err)
}

func Test_UpdateState(t *testing.T) {
	db := localsql.InMemory()

	commitmentATX := types.RandomATXID()
	ch := &types.NIPostChallenge{
		PublishEpoch:   6,
		Sequence:       0,
		PrevATXID:      types.RandomATXID(),
		PositioningATX: types.RandomATXID(),
		CommitmentATX:  &commitmentATX,
		InitialPost:    &types.Post{Nonce: 1, Indices: []byte{1, 2, 3}, Pow: 1},
	}

	nodeID := types.RandomNodeID()
	err := AddChallenge(db, nodeID, ch)
	require.NoError(t, err)

	// update challenge
	ch.PublishEpoch = 7
	err = UpdateChallenge(db, nodeID, ch)
	require.NoError(t, err)

	challenge, err := Challenge(db, nodeID)
	require.NoError(t, err)
	require.NotNil(t, challenge)
	require.Equal(t, ch, challenge)
}
