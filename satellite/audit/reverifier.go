// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package audit

import (
	"bytes"
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
	"io"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/errs2"
	"storj.io/common/pb"
	"storj.io/common/pkcrypto"
	"storj.io/common/rpc"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/common/signing"
	"storj.io/common/storj"
	"storj.io/common/uuid"
	"storj.io/storj/satellite/metabase"
	"storj.io/storj/satellite/overlay"
	"storj.io/uplink/private/eestream"
	"storj.io/uplink/private/piecestore"
)

// PieceLocator specifies all information necessary to look up a particular piece
// on a particular satellite.
type PieceLocator struct {
	StreamID uuid.UUID
	Position metabase.SegmentPosition
	NodeID   storj.NodeID
	PieceNum int
}

// ReverificationJob represents a job as received from the reverification
// audit queue.
type ReverificationJob struct {
	Locator       PieceLocator
	InsertedAt    time.Time
	ReverifyCount int
}

// Outcome enumerates the possible results of a piecewise audit.
//
// Note that it is very similar to reputation.AuditType, but it is
// different in scope and needs a slightly different set of values.
type Outcome int

const (
	// OutcomeNotPerformed indicates an audit was not performed, for any of a
	// variety of reasons, but that it should be reattempted later.
	OutcomeNotPerformed Outcome = iota
	// OutcomeNotNecessary indicates that an audit is no longer required,
	// for example because the segment has been updated or no longer exists.
	OutcomeNotNecessary
	// OutcomeSuccess indicates that an audit took place and the piece was
	// fully validated.
	OutcomeSuccess
	// OutcomeFailure indicates that an audit took place but that the node
	// failed the audit, either because it did not have the piece or the
	// data was incorrect.
	OutcomeFailure
	// OutcomeTimedOut indicates the audit could not be completed because
	// it took too long. The audit should be retried later.
	OutcomeTimedOut
	// OutcomeNodeOffline indicates that the audit could not be completed
	// because the node could not be contacted. The audit should be
	// retried later.
	OutcomeNodeOffline
	// OutcomeUnknownError indicates that the audit could not be completed
	// because of an error not otherwise expected or recognized. The
	// audit should be retried later.
	OutcomeUnknownError
)

// ReverifyPiece acquires a piece from a single node and verifies its
// contents, its hash, and its order limit.
func (verifier *Verifier) ReverifyPiece(ctx context.Context, locator PieceLocator) (keepInQueue bool) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	defer span.End()

	logger := verifier.log.With(
		zap.Stringer("stream-id", locator.StreamID),
		zap.Uint32("position-part", locator.Position.Part),
		zap.Uint32("position-index", locator.Position.Index),
		zap.Stringer("node-id", locator.NodeID),
		zap.Int("piece-num", locator.PieceNum))

	outcome, err := verifier.DoReverifyPiece(ctx, logger, locator)
	if err != nil {
		logger.Error("could not perform reverification due to error", zap.Error(err))
		return true
	}

	var (
		successes int
		offlines  int
		fails     int
		pending   int
		unknown   int
	)
	switch outcome {
	case OutcomeNotPerformed:
		keepInQueue = true
	case OutcomeNotNecessary:
	case OutcomeSuccess:
		successes++
	case OutcomeFailure:
		fails++
	case OutcomeTimedOut:
		pending++
		keepInQueue = true
	case OutcomeNodeOffline:
		offlines++
		keepInQueue = true
	case OutcomeUnknownError:
		unknown++
		keepInQueue = true
	}
	histCounter, _ := meter.SyncInt64().Histogram("reverify_successes_global")
	histCounter.Record(ctx, int64(successes))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_offlines_global")
	histCounter.Record(ctx, int64(offlines))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_fails_global")
	histCounter.Record(ctx, int64(fails))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_contained_global")
	histCounter.Record(ctx, int64(pending))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_unknown_global")
	histCounter.Record(ctx, int64(unknown))

	return keepInQueue
}

// DoReverifyPiece acquires a piece from a single node and verifies its
// contents, its hash, and its order limit.
func (verifier *Verifier) DoReverifyPiece(ctx context.Context, logger *zap.Logger, locator PieceLocator) (outcome Outcome, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	// First, we must ensure that the specified node still holds the indicated piece.
	segment, err := verifier.metabase.GetSegmentByPosition(ctx, metabase.GetSegmentByPosition{
		StreamID: locator.StreamID,
		Position: locator.Position,
	})
	if err != nil {
		if metabase.ErrSegmentNotFound.Has(err) {
			logger.Debug("segment no longer exists")
			return OutcomeNotNecessary, nil
		}
		return OutcomeNotPerformed, Error.Wrap(err)
	}
	if segment.Expired(verifier.nowFn()) {
		logger.Debug("segment expired before ReverifyPiece")
		return OutcomeNotNecessary, nil
	}
	piece, found := segment.Pieces.FindByNum(locator.PieceNum)
	if !found || piece.StorageNode != locator.NodeID {
		logger.Debug("piece is no longer held by the indicated node")
		return OutcomeNotNecessary, nil
	}

	// TODO remove this when old entries with empty StreamID will be deleted
	if locator.StreamID.IsZero() {
		logger.Debug("ReverifyPiece: skip pending audit with empty StreamID")
		return OutcomeNotNecessary, nil
	}

	redundancy, err := eestream.NewRedundancyStrategyFromStorj(segment.Redundancy)
	if err != nil {
		return OutcomeNotPerformed, Error.Wrap(err)
	}

	pieceSize := eestream.CalcPieceSize(int64(segment.EncryptedSize), redundancy)

	limit, piecePrivateKey, cachedNodeInfo, err := verifier.orders.CreateAuditPieceOrderLimit(ctx, locator.NodeID, uint16(locator.PieceNum), segment.RootPieceID, int32(pieceSize))
	if err != nil {
		if overlay.ErrNodeDisqualified.Has(err) {
			logger.Debug("ReverifyPiece: order limit not created (node is already disqualified)")
			return OutcomeNotNecessary, nil
		}
		if overlay.ErrNodeFinishedGE.Has(err) {
			logger.Debug("ReverifyPiece: order limit not created (node has completed graceful exit)")
			return OutcomeNotNecessary, nil
		}
		if overlay.ErrNodeOffline.Has(err) {
			logger.Debug("ReverifyPiece: order limit not created (node considered offline)")
			return OutcomeNotPerformed, nil
		}
		return OutcomeNotPerformed, Error.Wrap(err)
	}

	pieceData, pieceHash, pieceOriginalLimit, err := verifier.GetPiece(ctx, limit, piecePrivateKey, cachedNodeInfo.LastIPPort, int32(pieceSize))
	if err != nil {
		if rpc.Error.Has(err) {
			if errs.Is(err, context.DeadlineExceeded) {
				// dial timeout
				return OutcomeTimedOut, nil
			}
			if errs2.IsRPC(err, rpcstatus.Unknown) {
				// dial failed -- offline node
				return OutcomeNodeOffline, nil
			}
			// unknown transport error
			logger.Info("ReverifyPiece: unknown transport error", zap.Error(err))
			return OutcomeUnknownError, nil
		}
		if errs2.IsRPC(err, rpcstatus.NotFound) {
			// Fetch the segment metadata again and see if it has been altered in the interim
			err := verifier.checkIfSegmentAltered(ctx, segment)
			if err != nil {
				// if so, we skip this audit
				logger.Debug("ReverifyPiece: audit source segment changed during reverification", zap.Error(err))
				return OutcomeNotNecessary, nil
			}
			// missing share
			logger.Info("ReverifyPiece: audit failure; node indicates piece not found")
			return OutcomeFailure, nil
		}
		if errs2.IsRPC(err, rpcstatus.DeadlineExceeded) {
			// dial successful, but download timed out
			return OutcomeTimedOut, nil
		}
		// unknown error
		logger.Info("ReverifyPiece: unknown error from node", zap.Error(err))
		return OutcomeUnknownError, nil
	}

	// We have successfully acquired the piece from the node. Now, we must verify its contents.

	if pieceHash == nil {
		logger.Info("ReverifyPiece: audit failure; node did not send piece hash as requested")
		return OutcomeFailure, nil
	}
	if pieceOriginalLimit == nil {
		logger.Info("ReverifyPiece: audit failure; node did not send original order limit as requested")
		return OutcomeFailure, nil
	}
	// check for the correct size
	if int64(len(pieceData)) != pieceSize {
		logger.Info("ReverifyPiece: audit failure; downloaded piece has incorrect size", zap.Int64("expected-size", pieceSize), zap.Int("received-size", len(pieceData)))
		outcome = OutcomeFailure
		// continue to run, so we can check if the piece was legitimately changed before
		// blaming the node
	} else {
		// check for a matching hash
		downloadedHash := pkcrypto.SHA256Hash(pieceData)
		if !bytes.Equal(downloadedHash, pieceHash.Hash) {
			logger.Info("ReverifyPiece: audit failure; downloaded piece does not match hash", zap.ByteString("downloaded", downloadedHash), zap.ByteString("expected", pieceHash.Hash))
			outcome = OutcomeFailure
			// continue to run, so we can check if the piece was legitimately changed
			// before blaming the node
		} else {
			// check that the order limit and hash sent by the storagenode were
			// correctly signed (order limit signed by this satellite, hash signed
			// by the uplink public key in the order limit)
			signer := signing.SigneeFromPeerIdentity(verifier.auditor)
			if err := signing.VerifyOrderLimitSignature(ctx, signer, pieceOriginalLimit); err != nil {
				return OutcomeFailure, nil
			}
			if err := signing.VerifyUplinkPieceHashSignature(ctx, pieceOriginalLimit.UplinkPublicKey, pieceHash); err != nil {
				return OutcomeFailure, nil
			}
		}
	}

	if err := verifier.checkIfSegmentAltered(ctx, segment); err != nil {
		logger.Debug("ReverifyPiece: audit source segment changed during reverification", zap.Error(err))
		return OutcomeNotNecessary, nil
	}
	if outcome == OutcomeFailure {
		return OutcomeFailure, nil
	}

	return OutcomeSuccess, nil
}

// GetPiece uses the piecestore client to download a piece (and the associated
// original OrderLimit and PieceHash) from a node.
func (verifier *Verifier) GetPiece(ctx context.Context, limit *pb.AddressedOrderLimit, piecePrivateKey storj.PiecePrivateKey, cachedIPAndPort string, pieceSize int32) (pieceData []byte, hash *pb.PieceHash, origLimit *pb.OrderLimit, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	// determines number of seconds allotted for receiving data from a storage node
	timedCtx := ctx
	if verifier.minBytesPerSecond > 0 {
		maxTransferTime := time.Duration(int64(time.Second) * int64(pieceSize) / verifier.minBytesPerSecond.Int64())
		if maxTransferTime < verifier.minDownloadTimeout {
			maxTransferTime = verifier.minDownloadTimeout
		}
		var cancel func()
		timedCtx, cancel = context.WithTimeout(ctx, maxTransferTime)
		defer cancel()
	}

	targetNodeID := limit.GetLimit().StorageNodeId
	log := verifier.log.With(zap.Stringer("node-id", targetNodeID), zap.Stringer("piece-id", limit.GetLimit().PieceId))
	var ps *piecestore.Client

	// if cached IP is given, try connecting there first
	if cachedIPAndPort != "" {
		nodeAddr := storj.NodeURL{
			ID:      targetNodeID,
			Address: cachedIPAndPort,
		}
		ps, err = piecestore.Dial(timedCtx, verifier.dialer, nodeAddr, piecestore.DefaultConfig)
		if err != nil {
			log.Debug("failed to connect to audit target node at cached IP", zap.String("cached-ip-and-port", cachedIPAndPort), zap.Error(err))
		}
	}

	// if no cached IP was given, or connecting to cached IP failed, use node address
	if ps == nil {
		nodeAddr := storj.NodeURL{
			ID:      targetNodeID,
			Address: limit.GetStorageNodeAddress().Address,
		}
		ps, err = piecestore.Dial(timedCtx, verifier.dialer, nodeAddr, piecestore.DefaultConfig)
		if err != nil {
			return nil, nil, nil, Error.Wrap(err)
		}
	}

	defer func() {
		err := ps.Close()
		if err != nil {
			log.Error("audit verifier failed to close conn to node", zap.Error(err))
		}
	}()

	downloader, err := ps.Download(timedCtx, limit.GetLimit(), piecePrivateKey, 0, int64(pieceSize))
	if err != nil {
		return nil, nil, nil, Error.Wrap(err)
	}
	defer func() { err = errs.Combine(err, Error.Wrap(downloader.Close())) }()

	buf := make([]byte, pieceSize)
	_, err = io.ReadFull(downloader, buf)
	if err != nil {
		return nil, nil, nil, Error.Wrap(err)
	}
	hash, originLimit := downloader.GetHashAndLimit()

	return buf, hash, originLimit, nil
}
