package fetch

import (
	"context"
	"errors"

	"github.com/spacemeshos/go-spacemesh/codec"
	"github.com/spacemeshos/go-spacemesh/common/types"
	"github.com/spacemeshos/go-spacemesh/datastore"
	"github.com/spacemeshos/go-spacemesh/log"
	"github.com/spacemeshos/go-spacemesh/sql"
	"github.com/spacemeshos/go-spacemesh/sql/atxs"
	"github.com/spacemeshos/go-spacemesh/sql/ballots"
	"github.com/spacemeshos/go-spacemesh/sql/certificates"
	"github.com/spacemeshos/go-spacemesh/sql/identities"
	"github.com/spacemeshos/go-spacemesh/sql/layers"
)

const (
	fetchSubKey sql.QueryCacheSubKey = "epoch-info-req"
)

type handler struct {
	logger log.Log
	cdb    *datastore.CachedDB
	bs     *datastore.BlobStore
}

func newHandler(
	cdb *datastore.CachedDB,
	bs *datastore.BlobStore,
	lg log.Log,
) *handler {
	return &handler{
		logger: lg,
		cdb:    cdb,
		bs:     bs,
	}
}

// handleMaliciousIDsReq returns the IDs of all known malicious nodes.
func (h *handler) handleMaliciousIDsReq(ctx context.Context, _ []byte) ([]byte, error) {
	nodes, err := identities.GetMalicious(h.cdb)
	if err != nil {
		h.logger.With().Warning("serve: failed to get malicious IDs",
			log.Context(ctx), log.Err(err))
		return nil, err
	}
	h.logger.With().
		Debug("serve: responded to malicious IDs request",
			log.Context(ctx), log.Int("num_malicious", len(nodes)))
	malicious := &MaliciousIDs{
		NodeIDs: nodes,
	}
	data, err := codec.Encode(malicious)
	if err != nil {
		h.logger.With().Fatal("serve: failed to encode malicious IDs", log.Err(err))
	}
	return data, nil
}

// handleEpochInfoReq returns the ATXs published in the specified epoch.
func (h *handler) handleEpochInfoReq(ctx context.Context, msg []byte) ([]byte, error) {
	var epoch types.EpochID
	if err := codec.Decode(msg, &epoch); err != nil {
		return nil, err
	}

	cacheKey := sql.QueryCacheKey(atxs.CacheKindEpochATXs, epoch.String())
	return sql.WithCachedSubKey(ctx, h.cdb, cacheKey, fetchSubKey, func(ctx context.Context) ([]byte, error) {
		atxids, err := atxs.GetIDsByEpoch(ctx, h.cdb, epoch)
		if err != nil {
			h.logger.With().Warning("serve: failed to get epoch atx IDs",
				epoch, log.Err(err), log.Context(ctx))
			return nil, err
		}
		ed := EpochData{
			AtxIDs: atxids,
		}
		h.logger.With().Debug("serve: responded to epoch info request",
			epoch, log.Context(ctx), log.Int("atx_count", len(ed.AtxIDs)))
		bts, err := codec.Encode(&ed)
		if err != nil {
			h.logger.With().Fatal("serve: failed to serialize epoch atx",
				epoch, log.Context(ctx), log.Err(err))
		}
		return bts, nil
	})
}

// handleLayerDataReq returns all data in a layer, described in LayerData.
func (h *handler) handleLayerDataReq(ctx context.Context, req []byte) ([]byte, error) {
	var (
		lid types.LayerID
		ld  LayerData
		err error
	)
	if err := codec.Decode(req, &lid); err != nil {
		return nil, err
	}
	ld.Ballots, err = ballots.IDsInLayer(h.cdb, lid)
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		h.logger.With().Warning("serve: failed to get layer ballots",
			lid, log.Err(err), log.Context(ctx))
		return nil, err
	}

	out, err := codec.Encode(&ld)
	if err != nil {
		h.logger.With().Fatal(
			"serve: failed to serialize layer data response",
			log.Context(ctx), log.Err(err))
	}
	return out, nil
}

func (h *handler) handleLayerOpinionsReq2(ctx context.Context, data []byte) ([]byte, error) {
	var req OpinionRequest
	if err := codec.Decode(data, &req); err != nil {
		return nil, err
	}
	if req.Block != nil {
		return h.handleCertReq(ctx, req.Layer, *req.Block)
	}

	var (
		lid = req.Layer
		lo  LayerOpinion
		out []byte
		err error
	)

	opnReqV2.Inc()
	lo.PrevAggHash, err = layers.GetAggregatedHash(h.cdb, lid.Sub(1))
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		h.logger.With().Error("serve: failed to get prev agg hash", log.Context(ctx), lid, log.Err(err))
		return nil, err
	}
	bid, err := certificates.CertifiedBlock(h.cdb, lid)
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		h.logger.With().Error("serve: failed to get layer certified block",
			log.Context(ctx), lid, log.Err(err))
		return nil, err
	}
	if err == nil {
		lo.Certified = &bid
	}
	out, err = codec.Encode(&lo)
	if err != nil {
		h.logger.With().Fatal("serve: failed to serialize layer opinions response",
			log.Context(ctx), log.Err(err))
	}
	return out, nil
}

func (h *handler) handleCertReq(ctx context.Context, lid types.LayerID, bid types.BlockID) ([]byte, error) {
	certReq.Inc()
	certs, err := certificates.Get(h.cdb, lid)
	if err != nil && !errors.Is(err, sql.ErrNotFound) {
		h.logger.With().Error("serve: failed to get certificate", log.Context(ctx), lid, log.Err(err))
		return nil, err
	}
	if err == nil {
		for _, cert := range certs {
			if cert.Block == bid {
				out, err := codec.Encode(cert.Cert)
				if err != nil {
					h.logger.With().Fatal("serve: failed to encode cert",
						log.Context(ctx), log.Err(err))
				}
				return out, nil
			}
		}
	}
	return nil, err
}

func (h *handler) handleHashReq(ctx context.Context, data []byte) ([]byte, error) {
	var requestBatch RequestBatch
	if err := codec.Decode(data, &requestBatch); err != nil {
		h.logger.With().Warning("serve: failed to parse request", log.Context(ctx), log.Err(err))
		return nil, errBadRequest
	}
	resBatch := ResponseBatch{
		ID:        requestBatch.ID,
		Responses: make([]ResponseMessage, 0, len(requestBatch.Requests)),
	}
	// this will iterate all requests and populate appropriate Responses, if there are any missing items they will not
	// be included in the response at all
	for _, r := range requestBatch.Requests {
		totalHashReqs.WithLabelValues(string(r.Hint)).Add(1)
		res, err := h.bs.Get(ctx, r.Hint, r.Hash.Bytes())
		if err != nil {
			h.logger.With().Debug("serve: remote peer requested nonexistent hash",
				log.Context(ctx),
				log.String("hash", r.Hash.ShortString()),
				log.String("hint", string(r.Hint)),
				log.Err(err))
			hashMissing.WithLabelValues(string(r.Hint)).Add(1)
			continue
		} else if res == nil {
			h.logger.With().Debug("serve: remote peer requested golden",
				log.Context(ctx),
				log.String("hash", r.Hash.ShortString()),
				log.Int("dataSize", len(res)))
			hashEmptyData.WithLabelValues(string(r.Hint)).Add(1)
			continue
		} else {
			h.logger.With().Debug("serve: responded to hash request",
				log.Context(ctx),
				log.String("hash", r.Hash.ShortString()),
				log.Int("dataSize", len(res)))
		}
		// add response to batch
		m := ResponseMessage{
			Hash: r.Hash,
			Data: res,
		}
		resBatch.Responses = append(resBatch.Responses, m)
	}

	bts, err := codec.Encode(&resBatch)
	if err != nil {
		h.logger.With().Fatal("serve: failed to encode batch id",
			log.Context(ctx),
			log.Err(err),
			log.String("batch_hash", resBatch.ID.ShortString()))
	}
	h.logger.With().Debug("serve: returning response for batch",
		log.Context(ctx),
		log.String("batch_hash", resBatch.ID.ShortString()),
		log.Int("count_responses", len(resBatch.Responses)),
		log.Int("data_size", len(bts)))
	return bts, nil
}

func (h *handler) handleMeshHashReq(ctx context.Context, reqData []byte) ([]byte, error) {
	var (
		req    MeshHashRequest
		hashes []types.Hash32
		data   []byte
		err    error
	)
	if err = codec.Decode(reqData, &req); err != nil {
		h.logger.With().Warning("serve: failed to parse mesh hash request",
			log.Context(ctx), log.Err(err))
		return nil, errBadRequest
	}
	if err := req.Validate(); err != nil {
		h.logger.With().Debug("failed to validate mesh hash request",
			log.Context(ctx), log.Err(err))
		return nil, err
	}
	hashes, err = layers.GetAggHashes(h.cdb, req.From, req.To, req.Step)
	if err != nil {
		h.logger.With().Warning("serve: failed to get mesh hashes",
			log.Context(ctx), log.Err(err))
		return nil, err
	}
	data, err = codec.EncodeSlice(hashes)
	if err != nil {
		h.logger.With().Fatal("serve: failed to encode hashes",
			log.Context(ctx), log.Err(err))
	}
	h.logger.With().Debug("serve: returning response for mesh hashes",
		log.Context(ctx),
		log.Stringer("layer_from", req.From),
		log.Stringer("layer_to", req.To),
		log.Uint32("by", req.Step),
		log.Int("count_hashes", len(hashes)),
	)
	return data, nil
}
