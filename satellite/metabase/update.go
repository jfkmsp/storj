// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package metabase

import (
	"context"
	"database/sql"
	"errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"

	"storj.io/common/storj"
	"storj.io/common/uuid"
	"storj.io/storj/storage"
)

// UpdateSegmentPieces contains arguments necessary for updating segment pieces.
type UpdateSegmentPieces struct {
	StreamID uuid.UUID
	Position SegmentPosition

	OldPieces Pieces

	NewRedundancy storj.RedundancyScheme
	NewPieces     Pieces

	NewRepairedAt time.Time // sets new time of last segment repair (optional).
}

// UpdateSegmentPieces updates pieces for specified segment. If provided old pieces
// won't match current database state update will fail.
func (db *DB) UpdateSegmentPieces(ctx context.Context, opts UpdateSegmentPieces) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	defer span.End()

	if opts.StreamID.IsZero() {
		return ErrInvalidRequest.New("StreamID missing")
	}

	if err := opts.OldPieces.Verify(); err != nil {
		if ErrInvalidRequest.Has(err) {
			return ErrInvalidRequest.New("OldPieces: %v", errs.Unwrap(err))
		}
		return err
	}

	if opts.NewRedundancy.IsZero() {
		return ErrInvalidRequest.New("NewRedundancy zero")
	}

	// its possible that in this method we will have less pieces
	// than optimal shares (e.g. after repair)
	if len(opts.NewPieces) < int(opts.NewRedundancy.RepairShares) {
		return ErrInvalidRequest.New("number of new pieces is less than new redundancy repair shares value")
	}

	if err := opts.NewPieces.Verify(); err != nil {
		if ErrInvalidRequest.Has(err) {
			return ErrInvalidRequest.New("NewPieces: %v", errs.Unwrap(err))
		}
		return err
	}

	updateRepairAt := !opts.NewRepairedAt.IsZero()

	oldPieces, err := db.aliasCache.EnsurePiecesToAliases(ctx, opts.OldPieces)
	if err != nil {
		return Error.New("unable to convert pieces to aliases: %w", err)
	}

	newPieces, err := db.aliasCache.EnsurePiecesToAliases(ctx, opts.NewPieces)
	if err != nil {
		return Error.New("unable to convert pieces to aliases: %w", err)
	}

	var resultPieces AliasPieces
	err = db.db.QueryRowContext(ctx, `
		UPDATE segments SET
			remote_alias_pieces = CASE
				WHEN remote_alias_pieces = $3 THEN $4
				ELSE remote_alias_pieces
			END,
			redundancy = CASE
				WHEN remote_alias_pieces = $3 THEN $5
				ELSE redundancy
			END,
			repaired_at = CASE
				WHEN remote_alias_pieces = $3 AND $7 = true THEN $6
				ELSE repaired_at
			END
		WHERE
			stream_id     = $1 AND
			position      = $2
		RETURNING remote_alias_pieces
		`, opts.StreamID, opts.Position, oldPieces, newPieces, redundancyScheme{&opts.NewRedundancy}, opts.NewRepairedAt, updateRepairAt).
		Scan(&resultPieces)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrSegmentNotFound.New("segment missing")
		}
		return Error.New("unable to update segment pieces: %w", err)
	}

	if !EqualAliasPieces(newPieces, resultPieces) {
		return storage.ErrValueChanged.New("segment remote_alias_pieces field was changed")
	}

	counter, _ := meter.SyncInt64().Counter("segment_update")
	counter.Add(ctx, 1)

	return nil
}
