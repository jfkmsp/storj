// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"context"
	"io"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/errs2"
	"storj.io/common/rpc"
	"storj.io/common/rpc/rpcstatus"
	"storj.io/common/storj"
	"storj.io/common/sync2"
	"storj.io/storj/satellite/orders"
	"storj.io/uplink/private/piecestore"
)

// ErrNodeOffline is returned when it was not possible to contact a node or the node was not responding.
var ErrNodeOffline = errs.Class("node offline")

// VerifierConfig contains configurations for operation.
type VerifierConfig struct {
	PerPieceTimeout    time.Duration `help:"duration to wait per piece download" default:"800ms"`
	OrderRetryThrottle time.Duration `help:"how much to wait before retrying order creation" default:"50ms"`

	// TODO: throttle per download.
}

// NodeVerifier implements segment verification by dialing nodes.
type NodeVerifier struct {
	log *zap.Logger

	config VerifierConfig

	dialer rpc.Dialer
	orders *orders.Service
}

var _ Verifier = (*NodeVerifier)(nil)

// NewVerifier creates a new segment verifier using the specified dialer.
func NewVerifier(log *zap.Logger, dialer rpc.Dialer, orders *orders.Service, config VerifierConfig) *NodeVerifier {
	return &NodeVerifier{
		log:    log,
		config: config,
		dialer: dialer,
		orders: orders,
	}
}

// Verify a collection of segments by attempting to download a byte from each segment from the target node.
func (service *NodeVerifier) Verify(ctx context.Context, target storj.NodeURL, segments []*Segment) error {
	// TODO: add dial timeout
	client, err := piecestore.Dial(ctx, service.dialer, target, piecestore.DefaultConfig)
	if err != nil {
		return ErrNodeOffline.Wrap(err)
	}
	defer func() { _ = client.Close() }()

	for _, segment := range segments {
		err := service.verifySegment(ctx, client, target, segment)
		if err != nil {
			return Error.Wrap(err)
		}
	}
	return nil
}

// verifySegment tries to verify the segment by downloading a single byte from the specified segment.
func (service *NodeVerifier) verifySegment(ctx context.Context, client *piecestore.Client, target storj.NodeURL, segment *Segment) error {
	limit, piecePrivateKey, _, err := service.orders.CreateAuditOrderLimit(ctx, target.ID, 0, segment.RootPieceID, 1)
	if err != nil {
		service.log.Error("failed to create order limit",
			zap.Stringer("retrying in", service.config.OrderRetryThrottle),
			zap.String("stream-id", segment.StreamID.String()),
			zap.Uint64("position", segment.Position.Encode()),
			zap.Error(err))

		if !sync2.Sleep(ctx, service.config.OrderRetryThrottle) {
			return Error.Wrap(ctx.Err())
		}

		limit, piecePrivateKey, _, err = service.orders.CreateAuditOrderLimit(ctx, target.ID, 0, segment.RootPieceID, 1)
		if err != nil {
			return Error.Wrap(err)
		}
	}

	timedCtx, cancel := context.WithTimeout(ctx, service.config.PerPieceTimeout)
	defer cancel()

	downloader, err := client.Download(timedCtx, limit.GetLimit(), piecePrivateKey, 0, 1)
	if err != nil {
		if errs2.IsRPC(err, rpcstatus.NotFound) {
			service.log.Info("segment not found",
				zap.String("stream-id", segment.StreamID.String()),
				zap.Uint64("position", segment.Position.Encode()),
				zap.Error(err))
			segment.Status.MarkNotFound()
			return nil
		}

		service.log.Error("download failed",
			zap.String("stream-id", segment.StreamID.String()),
			zap.Uint64("position", segment.Position.Encode()),
			zap.Error(err))
		return ErrNodeOffline.Wrap(err)
	}
	defer func() {
		errClose := downloader.Close()
		if errClose != nil {
			// TODO: should we try reconnect in this case?
			service.log.Error("close failed",
				zap.String("stream-id", segment.StreamID.String()),
				zap.Uint64("position", segment.Position.Encode()),
				zap.Error(err))
			err = errs.Combine(err, ErrNodeOffline.Wrap(errClose))
		}
	}()

	buf := [1]byte{}
	_, err = io.ReadFull(downloader, buf[:])
	if err != nil {
		if errs2.IsRPC(err, rpcstatus.NotFound) {
			service.log.Info("segment not found",
				zap.String("stream-id", segment.StreamID.String()),
				zap.Uint64("position", segment.Position.Encode()),
				zap.Error(err))
			segment.Status.MarkNotFound()
			return nil
		}

		service.log.Error("read failed",
			zap.String("stream-id", segment.StreamID.String()),
			zap.Uint64("position", segment.Position.Encode()),
			zap.Error(err))
		return ErrNodeOffline.Wrap(err)
	}
	segment.Status.MarkFound()

	return nil
}
