// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package metrics

import (
	"context"
	"go.opentelemetry.io/otel/metric/global"
	"os"

	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/sync2"
	"storj.io/storj/satellite/metabase/segmentloop"
)

var (
	// Error defines the metrics chore errors class.
	Error = errs.Class("metrics")
)

// Config contains configurable values for metrics collection.
type Config struct {
}

// Chore implements the metrics chore.
//
// architecture: Chore
type Chore struct {
	log         *zap.Logger
	config      Config
	Loop        *sync2.Cycle
	segmentLoop *segmentloop.Service
	Counter     *Counter
}

// NewChore creates a new instance of the metrics chore.
func NewChore(log *zap.Logger, config Config, loop *segmentloop.Service) *Chore {
	return &Chore{
		log:    log,
		config: config,
		// This chore monitors segment loop, so it's fine to use very small cycle time.
		Loop:        sync2.NewCycle(time.Nanosecond),
		segmentLoop: loop,
	}
}

// Run starts the metrics chore.
func (chore *Chore) Run(ctx context.Context) (err error) {
	return chore.Loop.Run(ctx, func(ctx context.Context) (err error) {
		var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))

		chore.Counter = NewCounter()

		err = chore.segmentLoop.Monitor(ctx, chore.Counter)
		if err != nil {
			chore.log.Error("error joining segment loop", zap.Error(err))
			return nil
		}
		histCounter, _ := meter.SyncInt64().Histogram("remote_dependent_object_count")
		histCounter.Record(ctx, chore.Counter.RemoteObjects)
		histCounter, _ = meter.SyncInt64().Histogram("inline_object_count")
		histCounter.Record(ctx, chore.Counter.InlineObjects)

		histCounter, _ = meter.SyncInt64().Histogram("total_inline_bytes")
		histCounter.Record(ctx, chore.Counter.TotalInlineBytes)
		histCounter, _ = meter.SyncInt64().Histogram("total_remote_bytes")
		histCounter.Record(ctx, chore.Counter.TotalRemoteBytes)

		histCounter, _ = meter.SyncInt64().Histogram("total_inline_segments")
		histCounter.Record(ctx, chore.Counter.TotalInlineSegments)
		histCounter, _ = meter.SyncInt64().Histogram("total_remote_segments")
		histCounter.Record(ctx, chore.Counter.TotalRemoteSegments)

		// TODO move this metric to a place where objects are iterated e.g. tally
		// or drop it completely as we can easily get this value with redash
		// mon.IntVal("total_object_count").Observe(chore.Counter.ObjectCount)

		return nil
	})
}

// Close closes metrics chore.
func (chore *Chore) Close() error {
	chore.Loop.Close()
	return nil
}
