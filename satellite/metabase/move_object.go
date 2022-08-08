// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package metabase

import (
	"context"
	"database/sql"
	"errors"

	pgxerrcode "github.com/jackc/pgerrcode"

	"storj.io/common/storj"
	"storj.io/common/uuid"
	"storj.io/private/dbutil/pgutil"
	"storj.io/private/dbutil/pgutil/pgerrcode"
	"storj.io/private/dbutil/txutil"
	"storj.io/private/tagsql"
)

// BeginMoveObjectResult holds data needed to finish move object.
type BeginMoveObjectResult struct {
	StreamID uuid.UUID
	Version  Version
	// TODO we need metadata because of an uplink issue with how we are storing key and nonce
	EncryptedMetadata         []byte
	EncryptedMetadataKeyNonce []byte
	EncryptedMetadataKey      []byte
	EncryptedKeysNonces       []EncryptedKeyAndNonce
	EncryptionParameters      storj.EncryptionParameters
}

// EncryptedKeyAndNonce holds single segment position, encrypted key and nonce.
type EncryptedKeyAndNonce struct {
	Position          SegmentPosition
	EncryptedKeyNonce []byte
	EncryptedKey      []byte
}

// BeginMoveObject holds all data needed begin move object method.
type BeginMoveObject struct {
	ObjectLocation
}

// BeginMoveObject collects all data needed to begin object move procedure.
func (db *DB) BeginMoveObject(ctx context.Context, opts BeginMoveObject) (result BeginMoveObjectResult, err error) {
	defer mon.Task()(&ctx)(&err)

	if err := opts.ObjectLocation.Verify(); err != nil {
		return BeginMoveObjectResult{}, err
	}

	object, err := db.GetObjectLastCommitted(ctx, GetObjectLastCommitted{
		ObjectLocation: ObjectLocation{
			ProjectID:  opts.ProjectID,
			BucketName: opts.BucketName,
			ObjectKey:  opts.ObjectKey,
		},
	})
	if err != nil {
		return BeginMoveObjectResult{}, err
	}

	if int64(object.SegmentCount) > MoveSegmentLimit {
		return BeginMoveObjectResult{}, ErrInvalidRequest.New("object to move has too many segments (%d). Limit is %d.", object.SegmentCount, MoveSegmentLimit)
	}

	err = withRows(db.db.QueryContext(ctx, `
		SELECT
			position, encrypted_key_nonce, encrypted_key
		FROM segments
		WHERE stream_id = $1
		ORDER BY stream_id, position ASC
	`, object.StreamID))(func(rows tagsql.Rows) error {
		for rows.Next() {
			var keys EncryptedKeyAndNonce

			err = rows.Scan(&keys.Position, &keys.EncryptedKeyNonce, &keys.EncryptedKey)
			if err != nil {
				return Error.New("failed to scan segments: %w", err)
			}

			result.EncryptedKeysNonces = append(result.EncryptedKeysNonces, keys)
		}

		return nil
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return BeginMoveObjectResult{}, Error.New("unable to fetch object segments: %w", err)
	}

	result.StreamID = object.StreamID
	result.Version = object.Version
	result.EncryptionParameters = object.Encryption
	result.EncryptedMetadata = object.EncryptedMetadata
	result.EncryptedMetadataKey = object.EncryptedMetadataEncryptedKey
	result.EncryptedMetadataKeyNonce = object.EncryptedMetadataNonce

	return result, nil
}

// FinishMoveObject holds all data needed to finish object move.
type FinishMoveObject struct {
	ObjectStream
	NewBucket             string
	NewSegmentKeys        []EncryptedKeyAndNonce
	NewEncryptedObjectKey []byte
	// Optional. Required if object has metadata.
	NewEncryptedMetadataKeyNonce storj.Nonce
	NewEncryptedMetadataKey      []byte
}

// Verify verifies metabase.FinishMoveObject data.
func (finishMove FinishMoveObject) Verify() error {
	if err := finishMove.ObjectStream.Verify(); err != nil {
		return err
	}

	switch {
	case len(finishMove.NewBucket) == 0:
		return ErrInvalidRequest.New("NewBucket is missing")
	case len(finishMove.NewEncryptedObjectKey) == 0:
		return ErrInvalidRequest.New("NewEncryptedObjectKey is missing")
	}

	return nil
}

// FinishMoveObject accepts new encryption keys for moved object and updates the corresponding object ObjectKey and segments EncryptedKey.
func (db *DB) FinishMoveObject(ctx context.Context, opts FinishMoveObject) (err error) {
	defer mon.Task()(&ctx)(&err)

	if err := opts.Verify(); err != nil {
		return err
	}

	err = txutil.WithTx(ctx, db.db, nil, func(ctx context.Context, tx tagsql.Tx) (err error) {
		updateObjectsQuery := `
			UPDATE objects SET
				bucket_name = $1,
				object_key = $2,
				encrypted_metadata_encrypted_key = CASE WHEN objects.encrypted_metadata IS NOT NULL
				THEN $3
				ELSE objects.encrypted_metadata_encrypted_key
				END,
				encrypted_metadata_nonce = CASE WHEN objects.encrypted_metadata IS NOT NULL
				THEN $4
				ELSE objects.encrypted_metadata_nonce
				END
			WHERE
				project_id = $5 AND
				bucket_name = $6 AND
				object_key = $7 AND
				version = $8
			RETURNING
				segment_count, 
				objects.encrypted_metadata IS NOT NULL AND LENGTH(objects.encrypted_metadata) > 0 AS has_metadata,
				stream_id
        `

		var segmentsCount int
		var hasMetadata bool
		var streamID uuid.UUID

		row := tx.QueryRowContext(ctx, updateObjectsQuery, []byte(opts.NewBucket), opts.NewEncryptedObjectKey, opts.NewEncryptedMetadataKey, opts.NewEncryptedMetadataKeyNonce, opts.ProjectID, []byte(opts.BucketName), opts.ObjectKey, opts.Version)
		if err = row.Scan(&segmentsCount, &hasMetadata, &streamID); err != nil {
			if code := pgerrcode.FromError(err); code == pgxerrcode.UniqueViolation {
				return Error.Wrap(ErrObjectAlreadyExists.New(""))
			} else if errors.Is(err, sql.ErrNoRows) {
				return storj.ErrObjectNotFound.New("object not found")
			}
			return Error.New("unable to update object: %w", err)
		}
		if streamID != opts.StreamID {
			return storj.ErrObjectNotFound.New("object was changed during move")
		}
		if segmentsCount != len(opts.NewSegmentKeys) {
			return ErrInvalidRequest.New("wrong number of segments keys received")
		}
		if hasMetadata {
			switch {
			case opts.NewEncryptedMetadataKeyNonce.IsZero() && len(opts.NewEncryptedMetadataKey) != 0:
				return ErrInvalidRequest.New("EncryptedMetadataKeyNonce is missing")
			case len(opts.NewEncryptedMetadataKey) == 0 && !opts.NewEncryptedMetadataKeyNonce.IsZero():
				return ErrInvalidRequest.New("EncryptedMetadataKey is missing")
			}
		}

		var newSegmentKeys struct {
			Positions          []int64
			EncryptedKeys      [][]byte
			EncryptedKeyNonces [][]byte
		}

		for _, u := range opts.NewSegmentKeys {
			newSegmentKeys.EncryptedKeys = append(newSegmentKeys.EncryptedKeys, u.EncryptedKey)
			newSegmentKeys.EncryptedKeyNonces = append(newSegmentKeys.EncryptedKeyNonces, u.EncryptedKeyNonce)
			newSegmentKeys.Positions = append(newSegmentKeys.Positions, int64(u.Position.Encode()))
		}

		updateResult, err := tx.ExecContext(ctx, `
					UPDATE segments SET
						encrypted_key_nonce = P.encrypted_key_nonce,
						encrypted_key = P.encrypted_key
					FROM (SELECT unnest($2::INT8[]), unnest($3::BYTEA[]), unnest($4::BYTEA[])) as P(position, encrypted_key_nonce, encrypted_key)
					WHERE
						stream_id = $1 AND
						segments.position = P.position
			`, opts.StreamID, pgutil.Int8Array(newSegmentKeys.Positions), pgutil.ByteaArray(newSegmentKeys.EncryptedKeyNonces), pgutil.ByteaArray(newSegmentKeys.EncryptedKeys))
		if err != nil {
			return Error.Wrap(err)
		}

		affected, err := updateResult.RowsAffected()
		if err != nil {
			return Error.New("failed to get rows affected: %w", err)
		}

		if affected != int64(len(newSegmentKeys.Positions)) {
			return Error.New("segment is missing")
		}
		return nil
	})
	if err != nil {
		return err
	}

	mon.Meter("finish_move_object").Mark(1)

	return nil
}
