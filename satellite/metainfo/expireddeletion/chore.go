// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package expireddeletion

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/sync2"
	"storj.io/storj/satellite/metabase"
)

var (
	// Error defines the expireddeletion chore errors class.
	Error = errs.Class("expired deletion")
)

// Config contains configurable values for expired segment cleanup.
type Config struct {
	Interval  time.Duration `help:"the time between each attempt to go through the db and clean up expired segments" releaseDefault:"24h" devDefault:"10s" testDefault:"$TESTINTERVAL"`
	Enabled   bool          `help:"set if expired segment cleanup is enabled or not" releaseDefault:"true" devDefault:"true"`
	ListLimit int           `help:"how many expired objects to query in a batch" default:"100"`
}

// Chore implements the expired segment cleanup chore.
//
// architecture: Chore
type Chore struct {
	log      *zap.Logger
	config   Config
	metabase *metabase.DB

	nowFn func() time.Time
	Loop  *sync2.Cycle
}

// NewChore creates a new instance of the expireddeletion chore.
func NewChore(log *zap.Logger, config Config, metabase *metabase.DB) *Chore {
	return &Chore{
		log:      log,
		config:   config,
		metabase: metabase,

		nowFn: time.Now,
		Loop:  sync2.NewCycle(config.Interval),
	}
}

// Run starts the expireddeletion loop service.
func (chore *Chore) Run(ctx context.Context) (err error) {
	if !chore.config.Enabled {
		return nil
	}

	return chore.Loop.Run(ctx, chore.deleteExpiredObjects)
}

// Close stops the expireddeletion chore.
func (chore *Chore) Close() error {
	chore.Loop.Close()
	return nil
}

// SetNow allows tests to have the server act as if the current time is whatever they want.
func (chore *Chore) SetNow(nowFn func() time.Time) {
	chore.nowFn = nowFn
}

func (chore *Chore) deleteExpiredObjects(ctx context.Context) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	chore.log.Debug("deleting expired objects")

	// TODO log error instead of crashing core until we will be sure
	// that queries for deleting expired objects are stable
	err = chore.metabase.DeleteExpiredObjects(ctx, metabase.DeleteExpiredObjects{
		ExpiredBefore: chore.nowFn(),
		BatchSize:     chore.config.ListLimit,
	})
	if err != nil {
		chore.log.Error("deleting expired objects failed", zap.Error(err))
	}

	return nil
}
