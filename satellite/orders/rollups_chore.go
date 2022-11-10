// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package orders

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
	"os"
	"runtime"

	"storj.io/common/sync2"
)

// Chore for flushing orders write cache to the database.
//
// architecture: Chore
type Chore struct {
	log               *zap.Logger
	rollupsWriteCache *RollupsWriteCache
	Loop              *sync2.Cycle
}

// NewChore creates new chore for flushing the orders write cache to the database.
func NewChore(log *zap.Logger, rollupsWriteCache *RollupsWriteCache, config Config) *Chore {
	return &Chore{
		log:               log,
		rollupsWriteCache: rollupsWriteCache,
		Loop:              sync2.NewCycle(config.FlushInterval),
	}
}

// Run starts the orders write cache chore.
func (chore *Chore) Run(ctx context.Context) (err error) {
	return chore.Loop.Run(ctx, func(ctx context.Context) error {
		pc, _, _, _ := runtime.Caller(0)
		ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
		defer span.End()
		chore.rollupsWriteCache.Flush(ctx)
		return nil
	})
}

// Close stops the orders write cache chore.
func (chore *Chore) Close() error {
	chore.Loop.Close()
	return nil
}
