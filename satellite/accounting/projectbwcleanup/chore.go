// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package projectbwcleanup

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/sync2"
	"storj.io/storj/satellite/accounting"
)

// Config is a configuration struct for the Chore.
type Config struct {
	Interval     time.Duration `help:"how often to remove unused project bandwidth rollups" default:"168h" testDefault:"$TESTINTERVAL"`
	RetainMonths int           `help:"number of months of project bandwidth rollups to retain, not including the current month" default:"2"`
}

// Chore to remove unused project bandwidth rollups.
//
// architecture: Chore
type Chore struct {
	log    *zap.Logger
	db     accounting.ProjectAccounting
	config Config

	Loop *sync2.Cycle
}

// NewChore creates new chore for removing unused project bandwidth rollups.
func NewChore(log *zap.Logger, db accounting.ProjectAccounting, config Config) *Chore {

	return &Chore{
		log:    log,
		db:     db,
		config: config,

		Loop: sync2.NewCycle(config.Interval),
	}
}

// Run starts the chore.
func (chore *Chore) Run(ctx context.Context) (err error) {
	return chore.Loop.Run(ctx, func(ctx context.Context) error {
		pc, _, _, _ := runtime.Caller(0)
		ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
		defer span.End()
		err := chore.RunOnce(ctx)
		if err != nil {
			chore.log.Error("error removing project bandwidth rollups", zap.Error(err))
		}
		return nil
	})
}

// RunOnce removes unused project bandwidth rollups.
func (chore *Chore) RunOnce(ctx context.Context) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	if chore.config.RetainMonths < 0 {
		return errs.New("retain months cannot be less than 0")
	}

	now := time.Now().UTC()
	beforeMonth := time.Date(now.Year(), now.Month()-time.Month(chore.config.RetainMonths), 1, 0, 0, 0, 0, time.UTC)

	return chore.db.DeleteProjectBandwidthBefore(ctx, beforeMonth)
}

// Close stops the chore.
func (chore *Chore) Close() error {
	chore.Loop.Close()
	return nil
}
