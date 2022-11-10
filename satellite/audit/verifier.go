// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package audit

import (
	"bytes"
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
	"io"
	"math/rand"
	"os"

	"runtime"
	"time"

	"github.com/vivint/infectious"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/errs2"
	"storj.io/common/identity"
	"storj.io/common/memory"
	"storj.io/common/pb"
	"storj.io/common/pkcrypto"
	"storj.io/common/rpc"
	"storj.io/common/rpc/rpcpool"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/common/storj"
	"storj.io/storj/satellite/metabase"
	"storj.io/storj/satellite/orders"
	"storj.io/storj/satellite/overlay"
	"storj.io/uplink/private/piecestore"
)

var (

	// ErrNotEnoughShares is the errs class for when not enough shares are available to do an audit.
	ErrNotEnoughShares = errs.Class("not enough shares for successful audit")
	// ErrSegmentDeleted is the errs class when the audited segment was deleted during the audit.
	ErrSegmentDeleted = errs.Class("segment deleted during audit")
	// ErrSegmentModified is the errs class used when a segment has been changed in any way.
	ErrSegmentModified = errs.Class("segment has been modified")
)

// Share represents required information about an audited share.
type Share struct {
	Error    error
	PieceNum int
	NodeID   storj.NodeID
	Data     []byte
}

// Verifier helps verify the correctness of a given stripe.
//
// architecture: Worker
type Verifier struct {
	log                *zap.Logger
	metabase           *metabase.DB
	orders             *orders.Service
	auditor            *identity.PeerIdentity
	dialer             rpc.Dialer
	overlay            *overlay.Service
	containment        Containment
	minBytesPerSecond  memory.Size
	minDownloadTimeout time.Duration

	nowFn                            func() time.Time
	OnTestingCheckSegmentAlteredHook func()
}

// NewVerifier creates a Verifier.
func NewVerifier(log *zap.Logger, metabase *metabase.DB, dialer rpc.Dialer, overlay *overlay.Service, containment Containment, orders *orders.Service, id *identity.FullIdentity, minBytesPerSecond memory.Size, minDownloadTimeout time.Duration) *Verifier {
	return &Verifier{
		log:                log,
		metabase:           metabase,
		orders:             orders,
		auditor:            id.PeerIdentity(),
		dialer:             dialer,
		overlay:            overlay,
		containment:        containment,
		minBytesPerSecond:  minBytesPerSecond,
		minDownloadTimeout: minDownloadTimeout,
		nowFn:              time.Now,
	}
}

// Verify downloads shares then verifies the data correctness at a random stripe.
func (verifier *Verifier) Verify(ctx context.Context, segment Segment, skip map[storj.NodeID]bool) (report Report, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	defer span.End()

	var segmentInfo metabase.Segment
	defer func() {
		recordStats(report, len(segmentInfo.Pieces), err)
	}()

	if segment.Expired(verifier.nowFn()) {
		verifier.log.Debug("segment expired before Verify")
		return Report{}, nil
	}

	segmentInfo, err = verifier.metabase.GetSegmentByPosition(ctx, metabase.GetSegmentByPosition{
		StreamID: segment.StreamID,
		Position: segment.Position,
	})
	if err != nil {
		if metabase.ErrSegmentNotFound.Has(err) {
			verifier.log.Debug("segment deleted before Verify")
			return Report{}, nil
		}
		return Report{}, err
	}

	randomIndex, err := GetRandomStripe(ctx, segmentInfo)
	if err != nil {
		return Report{}, err
	}

	var offlineNodes storj.NodeIDList
	var failedNodes storj.NodeIDList
	var unknownNodes storj.NodeIDList
	containedNodes := make(map[int]storj.NodeID)
	sharesToAudit := make(map[int]Share)

	orderLimits, privateKey, cachedNodesInfo, err := verifier.orders.CreateAuditOrderLimits(ctx, segmentInfo, skip)
	if err != nil {
		if orders.ErrDownloadFailedNotEnoughPieces.Has(err) {
			counter, _ := meter.SyncInt64().Counter("not_enough_shares_for_audit")
			counter.Add(context.Background(), 1)
			counter, _ = meter.SyncInt64().Counter("audit_not_enough_nodes_online")
			counter.Add(context.Background(), 1)
			err = ErrNotEnoughShares.Wrap(err)
		}
		return Report{}, err
	}
	cachedNodesReputation := make(map[storj.NodeID]overlay.ReputationStatus, len(cachedNodesInfo))
	for id, info := range cachedNodesInfo {
		cachedNodesReputation[id] = info.Reputation
	}
	defer func() { report.NodesReputation = cachedNodesReputation }()

	// NOTE offlineNodes will include disqualified nodes because they aren't in
	// the skip list
	offlineNodes = getOfflineNodes(segmentInfo, orderLimits, skip)
	if len(offlineNodes) > 0 {
		verifier.log.Debug("Verify: order limits not created for some nodes (offline/disqualified)",
			zap.Strings("Node IDs", offlineNodes.Strings()),
			zap.String("Segment", segmentInfoString(segment)))
	}

	shares, err := verifier.DownloadShares(ctx, orderLimits, privateKey, cachedNodesInfo, randomIndex, segmentInfo.Redundancy.ShareSize)
	if err != nil {
		return Report{
			Offlines: offlineNodes,
		}, err
	}

	err = verifier.checkIfSegmentAltered(ctx, segmentInfo)
	if err != nil {
		if ErrSegmentDeleted.Has(err) {
			verifier.log.Debug("segment deleted during Verify")
			return Report{}, nil
		}
		if ErrSegmentModified.Has(err) {
			verifier.log.Debug("segment modified during Verify")
			return Report{}, nil
		}
		return Report{
			Offlines: offlineNodes,
		}, err
	}

	for pieceNum, share := range shares {
		if share.Error == nil {
			// no error -- share downloaded successfully
			sharesToAudit[pieceNum] = share
			continue
		}
		if rpc.Error.Has(share.Error) {
			if errs.Is(share.Error, context.DeadlineExceeded) {
				// dial timeout
				offlineNodes = append(offlineNodes, share.NodeID)
				verifier.log.Debug("Verify: dial timeout (offline)",
					zap.Stringer("Node ID", share.NodeID),
					zap.String("Segment", segmentInfoString(segment)),
					zap.Error(share.Error))
				continue
			}
			if errs2.IsRPC(share.Error, rpcstatus.Unknown) {
				// dial failed -- offline node
				offlineNodes = append(offlineNodes, share.NodeID)
				verifier.log.Debug("Verify: dial failed (offline)",
					zap.Stringer("Node ID", share.NodeID),
					zap.String("Segment", segmentInfoString(segment)),
					zap.Error(share.Error))
				continue
			}
			// unknown transport error
			unknownNodes = append(unknownNodes, share.NodeID)
			verifier.log.Info("Verify: unknown transport error (skipped)",
				zap.Stringer("Node ID", share.NodeID),
				zap.String("Segment", segmentInfoString(segment)),
				zap.Error(share.Error))
			continue
		}

		if errs2.IsRPC(share.Error, rpcstatus.NotFound) {
			// missing share
			failedNodes = append(failedNodes, share.NodeID)
			verifier.log.Info("Verify: piece not found (audit failed)",
				zap.Stringer("Node ID", share.NodeID),
				zap.String("Segment", segmentInfoString(segment)),
				zap.Error(share.Error))
			continue
		}

		if errs2.IsRPC(share.Error, rpcstatus.DeadlineExceeded) {
			// dial successful, but download timed out
			containedNodes[pieceNum] = share.NodeID
			verifier.log.Info("Verify: download timeout (contained)",
				zap.Stringer("Node ID", share.NodeID),
				zap.String("Segment", segmentInfoString(segment)),
				zap.Error(share.Error))
			continue
		}

		// unknown error
		unknownNodes = append(unknownNodes, share.NodeID)
		verifier.log.Info("Verify: unknown error (skipped)",
			zap.Stringer("Node ID", share.NodeID),
			zap.String("Segment", segmentInfoString(segment)),
			zap.Error(share.Error))
	}
	histCounter, _ := meter.SyncInt64().Histogram("verify_shares_downloaded_successfully")
	histCounter.Record(ctx, int64(len(sharesToAudit)))

	required := segmentInfo.Redundancy.RequiredShares
	total := segmentInfo.Redundancy.TotalShares

	if len(sharesToAudit) < int(required) {
		counter, _ := meter.SyncInt64().Counter("not_enough_shares_for_audit")
		counter.Add(context.Background(), 1)
		// if we have reached this point, most likely something went wrong
		// like a network problem or a forgotten delete. Don't fail nodes.
		// We have an alert on this. Check the logs and see what happened.
		if len(offlineNodes)+len(containedNodes) > len(sharesToAudit)+len(failedNodes)+len(unknownNodes) {
			counter, _ := meter.SyncInt64().Counter("audit_suspected_network_problem")
			counter.Add(context.Background(), 1)
		} else {
			counter, _ := meter.SyncInt64().Counter("audit_not_enough_shares_acquired")
			counter.Add(context.Background(), 1)
		}
		report := Report{
			Offlines: offlineNodes,
			Unknown:  unknownNodes,
		}
		return report, ErrNotEnoughShares.New("got: %d, required: %d, failed: %d, offline: %d, unknown: %d, contained: %d",
			len(sharesToAudit), required, len(failedNodes), len(offlineNodes), len(unknownNodes), len(containedNodes))
	}
	// ensure we get values, even if only zero values, so that redash can have an alert based on these
	counter, _ := meter.SyncInt64().Counter("not_enough_shares_for_audit")
	counter.Add(context.Background(), 0)
	counter, _ = meter.SyncInt64().Counter("audit_not_enough_nodes_online")
	counter.Add(context.Background(), 0)
	counter, _ = meter.SyncInt64().Counter("audit_not_enough_shares_acquired")
	counter.Add(context.Background(), 0)
	counter, _ = meter.SyncInt64().Counter("could_not_verify_audit_shares")
	counter.Add(context.Background(), 0)
	counter, _ = meter.SyncInt64().Counter("audit_suspected_network_problem")
	counter.Add(context.Background(), 0)

	pieceNums, correctedShares, err := auditShares(ctx, required, total, sharesToAudit)
	if err != nil {
		counter, _ := meter.SyncInt64().Counter("could_not_verify_audit_shares")
		counter.Add(context.Background(), 1)
		verifier.log.Error("could not verify shares", zap.String("Segment", segmentInfoString(segment)), zap.Error(err))
		return Report{
			Fails:    failedNodes,
			Offlines: offlineNodes,
			Unknown:  unknownNodes,
		}, err
	}

	for _, pieceNum := range pieceNums {
		verifier.log.Info("Verify: share data altered (audit failed)",
			zap.Stringer("Node ID", shares[pieceNum].NodeID),
			zap.String("Segment", segmentInfoString(segment)))
		failedNodes = append(failedNodes, shares[pieceNum].NodeID)
	}

	successNodes := getSuccessNodes(ctx, shares, failedNodes, offlineNodes, unknownNodes, containedNodes)

	pendingAudits, err := createPendingAudits(ctx, containedNodes, correctedShares, segment, segmentInfo, randomIndex)
	if err != nil {
		return Report{
			Successes: successNodes,
			Fails:     failedNodes,
			Offlines:  offlineNodes,
			Unknown:   unknownNodes,
		}, err
	}

	return Report{
		Successes:     successNodes,
		Fails:         failedNodes,
		Offlines:      offlineNodes,
		PendingAudits: pendingAudits,
		Unknown:       unknownNodes,
	}, nil
}

func segmentInfoString(segment Segment) string {
	return fmt.Sprintf("%s/%d",
		segment.StreamID.String(),
		segment.Position.Encode(),
	)
}

// DownloadShares downloads shares from the nodes where remote pieces are located.
func (verifier *Verifier) DownloadShares(ctx context.Context, limits []*pb.AddressedOrderLimit, piecePrivateKey storj.PiecePrivateKey, cachedNodesInfo map[storj.NodeID]overlay.NodeReputation, stripeIndex int32, shareSize int32) (shares map[int]Share, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	shares = make(map[int]Share, len(limits))
	ch := make(chan *Share, len(limits))

	for i, limit := range limits {
		if limit == nil {
			ch <- nil
			continue
		}

		var ipPort string
		node, ok := cachedNodesInfo[limit.Limit.StorageNodeId]
		if ok && node.LastIPPort != "" {
			ipPort = node.LastIPPort
		}

		go func(i int, limit *pb.AddressedOrderLimit) {
			share, err := verifier.GetShare(ctx, limit, piecePrivateKey, ipPort, stripeIndex, shareSize, i)
			if err != nil {
				share = Share{
					Error:    err,
					PieceNum: i,
					NodeID:   limit.GetLimit().StorageNodeId,
					Data:     nil,
				}
			}
			ch <- &share
		}(i, limit)
	}

	for range limits {
		share := <-ch
		if share != nil {
			shares[share.PieceNum] = *share
		}
	}

	return shares, nil
}

// Reverify reverifies the contained nodes in the stripe.
func (verifier *Verifier) Reverify(ctx context.Context, segment Segment) (report Report, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	defer span.End()

	// result status enum
	const (
		skipped = iota
		success
		offline
		failed
		contained
		unknown
		erred
	)

	type result struct {
		nodeID       storj.NodeID
		status       int
		pendingAudit *PendingAudit
		reputation   overlay.ReputationStatus
		release      bool
		err          error
	}

	if segment.Expired(verifier.nowFn()) {
		verifier.log.Debug("segment expired before Reverify")
		return Report{}, nil
	}

	segmentInfo, err := verifier.metabase.GetSegmentByPosition(ctx, metabase.GetSegmentByPosition{
		StreamID: segment.StreamID,
		Position: segment.Position,
	})
	if err != nil {
		if metabase.ErrSegmentNotFound.Has(err) {
			verifier.log.Debug("segment deleted before Reverify")
			return Report{}, nil
		}
		return Report{}, err
	}

	pieces := segmentInfo.Pieces
	ch := make(chan result, len(pieces))
	var containedInSegment int64

	for _, piece := range pieces {
		pending, err := verifier.containment.Get(ctx, piece.StorageNode)
		if err != nil {
			if ErrContainedNotFound.Has(err) {
				ch <- result{nodeID: piece.StorageNode, status: skipped}
				continue
			}
			ch <- result{nodeID: piece.StorageNode, status: erred, err: err}
			verifier.log.Debug("Reverify: error getting from containment db", zap.Stringer("Node ID", piece.StorageNode), zap.Error(err))
			continue
		}

		// TODO remove this when old entries with empty StreamID will be deleted
		if pending.StreamID.IsZero() {
			verifier.log.Debug("Reverify: skip pending audit with empty StreamID", zap.Stringer("Node ID", piece.StorageNode))
			ch <- result{nodeID: piece.StorageNode, status: skipped}
			continue
		}

		containedInSegment++

		go func(pending *PendingAudit) {
			pendingSegment, err := verifier.metabase.GetSegmentByPosition(ctx, metabase.GetSegmentByPosition{
				StreamID: pending.StreamID,
				Position: pending.Position,
			})
			if err != nil {
				if metabase.ErrSegmentNotFound.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
					return
				}

				ch <- result{nodeID: pending.NodeID, status: erred, err: err}
				verifier.log.Debug("Reverify: error getting pending segment from metabase", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}

			if pendingSegment.Expired(verifier.nowFn()) {
				verifier.log.Debug("Reverify: segment already expired", zap.Stringer("Node ID", pending.NodeID))
				ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
				return
			}

			// TODO: is this check still necessary? If the segment was found by its StreamID and position, the RootPieceID should not had changed.
			if pendingSegment.RootPieceID != pending.PieceID {
				ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
				return
			}
			var pieceNum uint16
			found := false
			for _, piece := range pendingSegment.Pieces {
				if piece.StorageNode == pending.NodeID {
					pieceNum = piece.Number
					found = true
				}
			}
			if !found {
				ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
				return
			}

			limit, piecePrivateKey, cachedNodeInfo, err := verifier.orders.CreateAuditOrderLimit(ctx, pending.NodeID, pieceNum, pending.PieceID, pending.ShareSize)
			if err != nil {
				if overlay.ErrNodeDisqualified.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
					verifier.log.Debug("Reverify: order limit not created (disqualified)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				if overlay.ErrNodeFinishedGE.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
					verifier.log.Debug("Reverify: order limit not created (completed graceful exit)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				if overlay.ErrNodeOffline.Has(err) {
					ch <- result{nodeID: pending.NodeID, status: offline, reputation: cachedNodeInfo.Reputation}
					verifier.log.Debug("Reverify: order limit not created (offline)", zap.Stringer("Node ID", pending.NodeID))
					return
				}
				ch <- result{nodeID: pending.NodeID, status: erred, err: err}
				verifier.log.Debug("Reverify: error creating order limit", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}

			share, err := verifier.GetShare(ctx, limit, piecePrivateKey, cachedNodeInfo.LastIPPort, pending.StripeIndex, pending.ShareSize, int(pieceNum))

			// check if the pending audit was deleted while downloading the share
			_, getErr := verifier.containment.Get(ctx, pending.NodeID)
			if getErr != nil {
				if ErrContainedNotFound.Has(getErr) {
					ch <- result{nodeID: pending.NodeID, status: skipped}
					verifier.log.Debug("Reverify: pending audit deleted during reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(getErr))
					return
				}
				ch <- result{nodeID: pending.NodeID, status: erred, err: getErr}
				verifier.log.Debug("Reverify: error getting from containment db", zap.Stringer("Node ID", pending.NodeID), zap.Error(getErr))
				return
			}

			// analyze the error from GetShare
			if err != nil {
				if rpc.Error.Has(err) {
					if errs.Is(err, context.DeadlineExceeded) {
						// dial timeout
						ch <- result{nodeID: pending.NodeID, status: offline, reputation: cachedNodeInfo.Reputation}
						verifier.log.Debug("Reverify: dial timeout (offline)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						return
					}
					if errs2.IsRPC(err, rpcstatus.Unknown) {
						// dial failed -- offline node
						verifier.log.Debug("Reverify: dial failed (offline)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						ch <- result{nodeID: pending.NodeID, status: offline, reputation: cachedNodeInfo.Reputation}
						return
					}
					// unknown transport error
					ch <- result{nodeID: pending.NodeID, status: unknown, pendingAudit: pending, reputation: cachedNodeInfo.Reputation, release: true}
					verifier.log.Info("Reverify: unknown transport error (skipped)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				if errs2.IsRPC(err, rpcstatus.NotFound) {
					// Get the original segment
					err := verifier.checkIfSegmentAltered(ctx, pendingSegment)
					if err != nil {
						ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
						verifier.log.Debug("Reverify: audit source changed before reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
						return
					}
					// missing share
					ch <- result{nodeID: pending.NodeID, status: failed, reputation: cachedNodeInfo.Reputation, release: true}
					verifier.log.Info("Reverify: piece not found (audit failed)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				if errs2.IsRPC(err, rpcstatus.DeadlineExceeded) {
					// dial successful, but download timed out
					ch <- result{nodeID: pending.NodeID, status: contained, pendingAudit: pending, reputation: cachedNodeInfo.Reputation}
					verifier.log.Info("Reverify: download timeout (contained)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				// unknown error
				ch <- result{nodeID: pending.NodeID, status: unknown, pendingAudit: pending, reputation: cachedNodeInfo.Reputation, release: true}
				verifier.log.Info("Reverify: unknown error (skipped)", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
				return
			}
			downloadedHash := pkcrypto.SHA256Hash(share.Data)
			if bytes.Equal(downloadedHash, pending.ExpectedShareHash) {
				ch <- result{nodeID: pending.NodeID, status: success, reputation: cachedNodeInfo.Reputation, release: true}
				verifier.log.Info("Reverify: hashes match (audit success)", zap.Stringer("Node ID", pending.NodeID))
			} else {
				err := verifier.checkIfSegmentAltered(ctx, pendingSegment)
				if err != nil {
					ch <- result{nodeID: pending.NodeID, status: skipped, release: true}
					verifier.log.Debug("Reverify: audit source changed before reverification", zap.Stringer("Node ID", pending.NodeID), zap.Error(err))
					return
				}
				verifier.log.Info("Reverify: hashes mismatch (audit failed)", zap.Stringer("Node ID", pending.NodeID),
					zap.Binary("expected hash", pending.ExpectedShareHash), zap.Binary("downloaded hash", downloadedHash))
				ch <- result{nodeID: pending.NodeID, status: failed, reputation: cachedNodeInfo.Reputation, release: true}
			}
		}(pending)
	}

	reputations := make(map[storj.NodeID]overlay.ReputationStatus)
	for range pieces {
		result := <-ch

		reputations[result.nodeID] = result.reputation

		switch result.status {
		case skipped:
		case success:
			report.Successes = append(report.Successes, result.nodeID)
		case offline:
			report.Offlines = append(report.Offlines, result.nodeID)
		case failed:
			report.Fails = append(report.Fails, result.nodeID)
		case contained:
			report.PendingAudits = append(report.PendingAudits, result.pendingAudit)
		case unknown:
			report.Unknown = append(report.Unknown, result.nodeID)
		case erred:
			err = errs.Combine(err, result.err)
		default:
		}
		if result.release {
			_, errDelete := verifier.containment.Delete(ctx, result.nodeID)
			if errDelete != nil {
				verifier.log.Debug("Error deleting node from containment db", zap.Stringer("Node ID", result.nodeID), zap.Error(errDelete))
			}
		}
	}
	report.NodesReputation = reputations

	histCounter, _ := meter.SyncInt64().Histogram("reverify_successes_global")
	histCounter.Record(ctx, int64(len(report.Successes)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_offlines_global")
	histCounter.Record(ctx, int64(len(report.Offlines)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_fails_global")
	histCounter.Record(ctx, int64(len(report.Fails)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_contained_global")
	histCounter.Record(ctx, int64(len(report.PendingAudits)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_unknown_global")
	histCounter.Record(ctx, int64(len(report.Unknown)))

	histCounter, _ = meter.SyncInt64().Histogram("reverify_successes")
	histCounter.Record(ctx, int64(len(report.Successes)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_offlines")
	histCounter.Record(ctx, int64(len(report.Offlines)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_fails")
	histCounter.Record(ctx, int64(len(report.Fails)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_contained")
	histCounter.Record(ctx, int64(len(report.PendingAudits)))
	histCounter, _ = meter.SyncInt64().Histogram("reverify_unknown")
	histCounter.Record(ctx, int64(len(report.Unknown)))

	histCounter, _ = meter.SyncInt64().Histogram("reverify_contained_in_segment")
	histCounter.Record(ctx, containedInSegment)
	histCounter, _ = meter.SyncInt64().Histogram("reverify_total_in_segment")
	histCounter.Record(ctx, int64(len(pieces)))

	return report, err
}

// GetShare use piece store client to download shares from nodes.
func (verifier *Verifier) GetShare(ctx context.Context, limit *pb.AddressedOrderLimit, piecePrivateKey storj.PiecePrivateKey, cachedIPAndPort string, stripeIndex, shareSize int32, pieceNum int) (share Share, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	bandwidthMsgSize := shareSize

	// determines number of seconds allotted for receiving data from a storage node
	timedCtx := ctx
	if verifier.minBytesPerSecond > 0 {
		maxTransferTime := time.Duration(int64(time.Second) * int64(bandwidthMsgSize) / verifier.minBytesPerSecond.Int64())
		if maxTransferTime < verifier.minDownloadTimeout {
			maxTransferTime = verifier.minDownloadTimeout
		}
		var cancel func()
		timedCtx, cancel = context.WithTimeout(ctx, maxTransferTime)
		defer cancel()
	}

	targetNodeID := limit.GetLimit().StorageNodeId
	log := verifier.log.Named(targetNodeID.String())
	var ps *piecestore.Client

	// if cached IP is given, try connecting there first
	if cachedIPAndPort != "" {
		nodeAddr := storj.NodeURL{
			ID:      targetNodeID,
			Address: cachedIPAndPort,
		}
		ps, err = piecestore.Dial(rpcpool.WithForceDial(timedCtx), verifier.dialer, nodeAddr, piecestore.DefaultConfig)
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
		ps, err = piecestore.Dial(rpcpool.WithForceDial(timedCtx), verifier.dialer, nodeAddr, piecestore.DefaultConfig)
		if err != nil {
			return Share{}, Error.Wrap(err)
		}
	}

	defer func() {
		err := ps.Close()
		if err != nil {
			verifier.log.Error("audit verifier failed to close conn to node: %+v", zap.Error(err))
		}
	}()

	offset := int64(shareSize) * int64(stripeIndex)

	downloader, err := ps.Download(timedCtx, limit.GetLimit(), piecePrivateKey, offset, int64(shareSize))
	if err != nil {
		return Share{}, err
	}
	defer func() { err = errs.Combine(err, downloader.Close()) }()

	buf := make([]byte, shareSize)
	_, err = io.ReadFull(downloader, buf)
	if err != nil {
		return Share{}, err
	}

	return Share{
		Error:    nil,
		PieceNum: pieceNum,
		NodeID:   targetNodeID,
		Data:     buf,
	}, nil
}

// checkIfSegmentAltered checks if oldSegment has been altered since it was selected for audit.
func (verifier *Verifier) checkIfSegmentAltered(ctx context.Context, oldSegment metabase.Segment) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	if verifier.OnTestingCheckSegmentAlteredHook != nil {
		verifier.OnTestingCheckSegmentAlteredHook()
	}

	newSegment, err := verifier.metabase.GetSegmentByPosition(ctx, metabase.GetSegmentByPosition{
		StreamID: oldSegment.StreamID,
		Position: oldSegment.Position,
	})
	if err != nil {
		if metabase.ErrSegmentNotFound.Has(err) {
			return ErrSegmentDeleted.New("StreamID: %q Position: %d", oldSegment.StreamID.String(), oldSegment.Position.Encode())
		}
		return err
	}

	if !oldSegment.Pieces.Equal(newSegment.Pieces) {
		return ErrSegmentModified.New("StreamID: %q Position: %d", oldSegment.StreamID.String(), oldSegment.Position.Encode())
	}
	return nil
}

// SetNow allows tests to have the server act as if the current time is whatever they want.
func (verifier *Verifier) SetNow(nowFn func() time.Time) {
	verifier.nowFn = nowFn
}

// auditShares takes the downloaded shares and uses infectious's Correct function to check that they
// haven't been altered. auditShares returns a slice containing the piece numbers of altered shares,
// and a slice of the corrected shares.
func auditShares(ctx context.Context, required, total int16, originals map[int]Share) (pieceNums []int, corrected []infectious.Share, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	f, err := infectious.NewFEC(int(required), int(total))
	if err != nil {
		return nil, nil, err
	}

	copies, err := makeCopies(ctx, originals)
	if err != nil {
		return nil, nil, err
	}

	err = f.Correct(copies)
	if err != nil {
		return nil, nil, err
	}

	for _, share := range copies {
		if !bytes.Equal(originals[share.Number].Data, share.Data) {
			pieceNums = append(pieceNums, share.Number)
		}
	}
	return pieceNums, copies, nil
}

// makeCopies takes in a map of audit Shares and deep copies their data to a slice of infectious Shares.
func makeCopies(ctx context.Context, originals map[int]Share) (copies []infectious.Share, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	copies = make([]infectious.Share, 0, len(originals))
	for _, original := range originals {
		copies = append(copies, infectious.Share{
			Data:   append([]byte{}, original.Data...),
			Number: original.PieceNum})
	}
	return copies, nil
}

// getOfflines nodes returns these storage nodes from the segment which have no
// order limit nor are skipped.
func getOfflineNodes(segment metabase.Segment, limits []*pb.AddressedOrderLimit, skip map[storj.NodeID]bool) storj.NodeIDList {
	var offlines storj.NodeIDList

	nodesWithLimit := make(map[storj.NodeID]bool, len(limits))
	for _, limit := range limits {
		if limit != nil {
			nodesWithLimit[limit.GetLimit().StorageNodeId] = true
		}
	}

	for _, piece := range segment.Pieces {
		if !nodesWithLimit[piece.StorageNode] && !skip[piece.StorageNode] {
			offlines = append(offlines, piece.StorageNode)
		}
	}

	return offlines
}

// getSuccessNodes uses the failed nodes, offline nodes and contained nodes arrays to determine which nodes passed the audit.
func getSuccessNodes(ctx context.Context, shares map[int]Share, failedNodes, offlineNodes, unknownNodes storj.NodeIDList, containedNodes map[int]storj.NodeID) (successNodes storj.NodeIDList) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	fails := make(map[storj.NodeID]bool)
	for _, fail := range failedNodes {
		fails[fail] = true
	}
	for _, offline := range offlineNodes {
		fails[offline] = true
	}
	for _, unknown := range unknownNodes {
		fails[unknown] = true
	}
	for _, contained := range containedNodes {
		fails[contained] = true
	}

	for _, share := range shares {
		if !fails[share.NodeID] {
			successNodes = append(successNodes, share.NodeID)
		}
	}

	return successNodes
}

func createPendingAudits(ctx context.Context, containedNodes map[int]storj.NodeID, correctedShares []infectious.Share, segment Segment, segmentInfo metabase.Segment, randomIndex int32) (pending []*PendingAudit, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	if len(containedNodes) == 0 {
		return nil, nil
	}

	required := int(segmentInfo.Redundancy.RequiredShares)
	total := int(segmentInfo.Redundancy.TotalShares)
	shareSize := segmentInfo.Redundancy.ShareSize

	fec, err := infectious.NewFEC(required, total)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	stripeData, err := rebuildStripe(ctx, fec, correctedShares, int(shareSize))
	if err != nil {
		return nil, Error.Wrap(err)
	}

	for pieceNum, nodeID := range containedNodes {
		share := make([]byte, shareSize)
		err = fec.EncodeSingle(stripeData, share, pieceNum)
		if err != nil {
			return nil, Error.Wrap(err)
		}
		pending = append(pending, &PendingAudit{
			NodeID:            nodeID,
			PieceID:           segmentInfo.RootPieceID,
			StripeIndex:       randomIndex,
			ShareSize:         shareSize,
			ExpectedShareHash: pkcrypto.SHA256Hash(share),
			StreamID:          segment.StreamID,
			Position:          segment.Position,
		})
	}

	return pending, nil
}

func rebuildStripe(ctx context.Context, fec *infectious.FEC, corrected []infectious.Share, shareSize int) (_ []byte, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	stripe := make([]byte, fec.Required()*shareSize)
	err = fec.Rebuild(corrected, func(share infectious.Share) {
		copy(stripe[share.Number*shareSize:], share.Data)
	})
	if err != nil {
		return nil, err
	}
	return stripe, nil
}

// GetRandomStripe takes a segment and returns a random stripe index within that segment.
func GetRandomStripe(ctx context.Context, segment metabase.Segment) (index int32, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	// the last segment could be smaller than stripe size
	if segment.EncryptedSize < segment.Redundancy.StripeSize() {
		return 0, nil
	}

	var src cryptoSource
	rnd := rand.New(src)
	numStripes := segment.Redundancy.StripeCount(segment.EncryptedSize)
	randomStripeIndex := rnd.Int31n(numStripes)

	return randomStripeIndex, nil
}

func recordStats(report Report, totalPieces int, verifyErr error) {
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	// If an audit was able to complete without auditing any nodes, that means
	// the segment has been altered.
	if verifyErr == nil && len(report.Successes) == 0 {
		return
	}

	numOffline := len(report.Offlines)
	numSuccessful := len(report.Successes)
	numFailed := len(report.Fails)
	numContained := len(report.PendingAudits)
	numUnknown := len(report.Unknown)

	totalAudited := numSuccessful + numFailed + numOffline + numContained
	auditedPercentage := float64(totalAudited) / float64(totalPieces)
	offlinePercentage := float64(0)
	successfulPercentage := float64(0)
	failedPercentage := float64(0)
	containedPercentage := float64(0)
	unknownPercentage := float64(0)
	if totalAudited > 0 {
		offlinePercentage = float64(numOffline) / float64(totalAudited)
		successfulPercentage = float64(numSuccessful) / float64(totalAudited)
		failedPercentage = float64(numFailed) / float64(totalAudited)
		containedPercentage = float64(numContained) / float64(totalAudited)
		unknownPercentage = float64(numUnknown) / float64(totalAudited)
	}

	ctx := context.Background()
	histCounter, _ := meter.SyncInt64().Histogram("audit_success_nodes_global")
	histCounter.Record(ctx, int64(numSuccessful))
	histCounter, _ = meter.SyncInt64().Histogram("audit_fail_nodes_global")
	histCounter.Record(ctx, int64(numFailed))
	histCounter, _ = meter.SyncInt64().Histogram("audit_offline_nodes_global")
	histCounter.Record(ctx, int64(numOffline))
	histCounter, _ = meter.SyncInt64().Histogram("audit_contained_nodes_global")
	histCounter.Record(ctx, int64(numContained))
	histCounter, _ = meter.SyncInt64().Histogram("audit_unknown_nodes_global")
	histCounter.Record(ctx, int64(numUnknown))
	histCounter, _ = meter.SyncInt64().Histogram("audit_total_nodes_global")
	histCounter.Record(ctx, int64(totalAudited))
	histCounter, _ = meter.SyncInt64().Histogram("audit_total_pointer_nodes_global")
	histCounter.Record(ctx, int64(totalPieces))

	histCounter, _ = meter.SyncInt64().Histogram("audit_success_nodes")
	histCounter.Record(ctx, int64(numSuccessful))
	histCounter, _ = meter.SyncInt64().Histogram("audit_fail_nodes")
	histCounter.Record(ctx, int64(numFailed))
	histCounter, _ = meter.SyncInt64().Histogram("audit_offline_nodes")
	histCounter.Record(ctx, int64(numOffline))
	histCounter, _ = meter.SyncInt64().Histogram("audit_contained_nodes")
	histCounter.Record(ctx, int64(numContained))
	histCounter, _ = meter.SyncInt64().Histogram("audit_unknown_nodes")
	histCounter.Record(ctx, int64(numUnknown))
	histCounter, _ = meter.SyncInt64().Histogram("audit_total_nodes")
	histCounter.Record(ctx, int64(totalAudited))
	histCounter, _ = meter.SyncInt64().Histogram("audit_total_pointer_nodes")
	histCounter.Record(ctx, int64(totalPieces))
	histCounter, _ = meter.SyncInt64().Histogram("audited_percentage")
	histCounter.Record(ctx, int64(auditedPercentage))
	histCounter, _ = meter.SyncInt64().Histogram("audit_offline_percentage")
	histCounter.Record(ctx, int64(offlinePercentage))
	histCounter, _ = meter.SyncInt64().Histogram("audit_successful_percentage")
	histCounter.Record(ctx, int64(successfulPercentage))
	histCounter, _ = meter.SyncInt64().Histogram("audit_failed_percentage")
	histCounter.Record(ctx, int64(failedPercentage))
	histCounter, _ = meter.SyncInt64().Histogram("audit_contained_percentage")
	histCounter.Record(ctx, int64(containedPercentage))
	histCounter, _ = meter.SyncInt64().Histogram("audit_unknown_percentage")
	histCounter.Record(ctx, int64(unknownPercentage))
}
