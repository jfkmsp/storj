// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package main

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"sync"

	"go.uber.org/zap"

	"storj.io/common/storj"
	"storj.io/common/sync2"
	"storj.io/storj/satellite/metabase"
)

// Verify verifies a collection of segments.
func (service *Service) Verify(ctx context.Context, segments []*Segment) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

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

	if len(retrySegments) == 0 {
		return nil
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
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	var mu sync.Mutex

	limiter := sync2.NewLimiter(service.config.Concurrency)
	for _, batch := range batches {
		batch := batch

		nodeURL, err := service.convertAliasToNodeURL(ctx, batch.Alias)
		if err != nil {
			return Error.Wrap(err)
		}

		ignoreThrottle := service.priorityNodes.Contains(batch.Alias)

		limiter.Go(ctx, func() {
			verifiedCount, err := service.verifier.Verify(ctx, batch.Alias, nodeURL, batch.Items, ignoreThrottle)
			if err != nil {
				if ErrNodeOffline.Has(err) {
					mu.Lock()
					if verifiedCount == 0 {
						service.onlineNodes.Remove(batch.Alias)
					} else {
						service.offlineCount[batch.Alias]++
						if service.offlineCount[batch.Alias] >= service.config.MaxOffline {
							service.onlineNodes.Remove(batch.Alias)
						}
					}
					mu.Unlock()
				}
				service.log.Error("verifying a batch failed", zap.Error(err))
			} else {
				mu.Lock()
				if service.offlineCount[batch.Alias] > 0 {
					service.offlineCount[batch.Alias]--
				}
				mu.Unlock()
			}
		})
	}
	limiter.Wait()

	return nil
}

// convertAliasToNodeURL converts a node alias to node url, using a cache if needed.
func (service *Service) convertAliasToNodeURL(ctx context.Context, alias metabase.NodeAlias) (_ storj.NodeURL, err error) {
	nodeURL, ok := service.aliasToNodeURL[alias]
	if !ok {
		nodeID, ok := service.aliasMap.Node(alias)
		if !ok {
			latest, err := service.metabase.LatestNodesAliasMap(ctx)
			if !ok {
				return storj.NodeURL{}, Error.Wrap(err)
			}
			service.aliasMap = latest

			nodeID, ok = service.aliasMap.Node(alias)
			if !ok {
				return storj.NodeURL{}, Error.Wrap(err)
			}
		}

		info, err := service.overlay.Get(ctx, nodeID)
		if err != nil {
			return storj.NodeURL{}, Error.Wrap(err)
		}

		nodeURL = storj.NodeURL{
			ID:      info.Id,
			Address: info.Address.Address,
		}

		service.aliasToNodeURL[alias] = nodeURL
	}
	return nodeURL, nil
}
