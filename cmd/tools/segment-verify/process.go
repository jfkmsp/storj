// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"storj.io/common/storj"
	"storj.io/common/sync2"
	"storj.io/storj/satellite/metabase"
)

// Verify verifies a collection of segments.
func (service *Service) Verify(ctx context.Context, segments []*Segment) (err error) {
	defer mon.Task()(&ctx)(&err)

	for _, segment := range segments {
		segment.Status.Retry = int32(service.config.Check)
	}

	batches, err := service.CreateBatches(ctx, segments)
	if err != nil {
		return Error.Wrap(err)
	}

	err = service.VerifyBatches(ctx, batches)
	if err != nil {
		return Error.Wrap(err)
	}

	retrySegments := []*Segment{}
	for _, segment := range segments {
		if segment.Status.Retry > 0 {
			retrySegments = append(retrySegments, segment)
		}
	}

	// Reverse the pieces slice to ensure we pick different nodes this time.
	for _, segment := range retrySegments {
		xs := segment.AliasPieces
		for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
			xs[i], xs[j] = xs[j], xs[i]
		}
		// Also remove priority nodes, because we have already checked them.
		service.removePriorityPieces(segment)
	}

	retryBatches, err := service.CreateBatches(ctx, retrySegments)
	if err != nil {
		return Error.Wrap(err)
	}

	err = service.VerifyBatches(ctx, retryBatches)
	if err != nil {
		return Error.Wrap(err)
	}

	return nil
}

// VerifyBatches verifies batches.
func (service *Service) VerifyBatches(ctx context.Context, batches []*Batch) error {
	defer mon.Task()(&ctx)(nil)

	// Convert NodeAliases to NodeIDs
	aliases := make([]metabase.NodeAlias, len(batches))
	for i, b := range batches {
		aliases[i] = b.Alias
	}
	ids, err := service.metabase.ConvertAliasesToNodes(ctx, aliases)
	if err != nil {
		return Error.Wrap(err)
	}

	// TODO: fetch addresses for NodeID-s

	var mu sync.Mutex

	limiter := sync2.NewLimiter(service.config.Concurrency)
	for i, batch := range batches {
		nodeID := ids[i]
		batch := batch
		limiter.Go(ctx, func() {
			err := service.verifier.Verify(ctx, storj.NodeURL{
				ID: nodeID, // TODO: use NodeURL
			}, batch.Items)
			if err != nil {
				if ErrNodeOffline.Has(err) {
					mu.Lock()
					service.OfflineNodes.Add(batch.Alias)
					mu.Unlock()
				}
				service.log.Error("verifying a batch failed", zap.Error(err))
			}
		})
	}
	limiter.Wait()

	return nil
}
