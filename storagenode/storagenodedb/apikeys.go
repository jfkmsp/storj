// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package storagenodedb

import (
	"context"
	"database/sql"
	"errors"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"

	"github.com/zeebo/errs"

	"storj.io/storj/private/multinodeauth"
	"storj.io/storj/storagenode/apikeys"
)

// ensures that apiKeysDB implements apikeys.DB interface.
var _ apikeys.DB = (*apiKeysDB)(nil)

// ErrAPIKeysDB represents errors from the api keys database.
var ErrAPIKeysDB = errs.Class("apikeysdb")

// APIKeysDBName represents the database name.
const APIKeysDBName = "secret"

// apiKeysDB works with node api keys DB.
type apiKeysDB struct {
	dbContainerImpl
}

// Store stores api key into database.
func (db *apiKeysDB) Store(ctx context.Context, apiKey apikeys.APIKey) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	query := `INSERT INTO secret (
			token,
			created_at
		) VALUES(?,?)`

	_, err = db.ExecContext(ctx, query,
		apiKey.Secret[:],
		apiKey.CreatedAt,
	)

	return ErrAPIKeysDB.Wrap(err)
}

// Check checks if api key exists in db by secret.
func (db *apiKeysDB) Check(ctx context.Context, secret multinodeauth.Secret) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	var bytes []uint8
	var createdAt string

	rowStub := db.QueryRowContext(ctx,
		`SELECT token, created_at FROM secret WHERE token = ?`,
		secret[:],
	)

	err = rowStub.Scan(
		&bytes,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apikeys.ErrNoAPIKey.Wrap(err)
		}
		return ErrAPIKeysDB.Wrap(err)
	}

	return nil
}

// Revoke removes api key from db.
func (db *apiKeysDB) Revoke(ctx context.Context, secret multinodeauth.Secret) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	query := `DELETE FROM secret WHERE token = ?`

	_, err = db.ExecContext(ctx, query, secret[:])

	return ErrAPIKeysDB.Wrap(err)
}
