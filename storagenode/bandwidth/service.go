// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

// Package bandwidth implements bandwidth usage rollup loop.
package bandwidth

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"time"

	"go.uber.org/zap"

	"storj.io/common/sync2"
)

// Config defines parameters for storage node Collector.
type Config struct {
	Interval time.Duration `help:"how frequently bandwidth usage rollups are calculated" default:"1h0m0s"`
}

// Service implements the bandwidth usage rollup service.
//
// architecture: Chore
type Service struct {
	log  *zap.Logger
	db   DB
	Loop *sync2.Cycle
}

// NewService creates a new bandwidth service.
func NewService(log *zap.Logger, db DB, config Config) *Service {
	return &Service{
		log:  log,
		db:   db,
		Loop: sync2.NewCycle(config.Interval),
	}
}

// Run starts the background process for rollups of bandwidth usage.
func (service *Service) Run(ctx context.Context) (err error) {
	return service.Loop.Run(ctx, service.Rollup)
}

// Rollup calls bandwidth DB Rollup method and logs any errors.
func (service *Service) Rollup(ctx context.Context) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	service.log.Info("Performing bandwidth usage rollups")
	err = service.db.Rollup(ctx)
	if err != nil {
		service.log.Error("Could not rollup bandwidth usage", zap.Error(err))
	}
	return nil
}

// Close stops the background process for rollups of bandwidth usage.
func (service *Service) Close() (err error) {
	service.Loop.Close()
	return nil
}
