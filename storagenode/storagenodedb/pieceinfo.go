// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package storagenodedb

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"

	"storj.io/common/pb"
	"storj.io/common/storj"
	"storj.io/storj/storage"
	"storj.io/storj/storage/filestore"
	"storj.io/storj/storagenode/pieces"
)

// ErrPieceInfo represents errors from the piece info database.
var ErrPieceInfo = errs.Class("v0pieceinfodb")

// PieceInfoDBName represents the database name.
const PieceInfoDBName = "pieceinfo"

type v0PieceInfoDB struct {
	dbContainerImpl
}

// Add inserts piece information into the database.
func (db *v0PieceInfoDB) Add(ctx context.Context, info *pieces.Info) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	orderLimit, err := pb.Marshal(info.OrderLimit)
	if err != nil {
		return ErrPieceInfo.Wrap(err)
	}

	uplinkPieceHash, err := pb.Marshal(info.UplinkPieceHash)
	if err != nil {
		return ErrPieceInfo.Wrap(err)
	}

	var pieceExpiration *time.Time
	if !info.PieceExpiration.IsZero() {
		utcExpiration := info.PieceExpiration.UTC()
		pieceExpiration = &utcExpiration
	}

	// TODO remove `uplink_cert_id` from DB
	_, err = db.ExecContext(ctx, `
		INSERT INTO
			pieceinfo_(satellite_id, piece_id, piece_size, piece_creation, piece_expiration, order_limit, uplink_piece_hash, uplink_cert_id)
		VALUES (?,?,?,?,?,?,?,?)
	`, info.SatelliteID, info.PieceID, info.PieceSize, info.PieceCreation.UTC(), pieceExpiration, orderLimit, uplinkPieceHash, 0)

	return ErrPieceInfo.Wrap(err)
}

func (db *v0PieceInfoDB) getAllPiecesOwnedBy(ctx context.Context, blobStore storage.Blobs, satelliteID storj.NodeID) ([]v0StoredPieceAccess, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT piece_id, piece_size, piece_creation, piece_expiration
		FROM pieceinfo_
		WHERE satellite_id = ?
		ORDER BY piece_id
	`, satelliteID)
	if err != nil {
		return nil, ErrPieceInfo.Wrap(err)
	}
	defer func() { err = errs.Combine(err, rows.Close()) }()
	var pieceInfos []v0StoredPieceAccess
	for rows.Next() {
		pieceInfos = append(pieceInfos, v0StoredPieceAccess{
			blobStore: blobStore,
			satellite: satelliteID,
		})
		thisAccess := &pieceInfos[len(pieceInfos)-1]
		err = rows.Scan(&thisAccess.pieceID, &thisAccess.pieceSize, &thisAccess.creationTime, &thisAccess.expirationTime)
		if err != nil {
			return nil, ErrPieceInfo.Wrap(err)
		}
	}
	return pieceInfos, rows.Err()
}

// WalkSatelliteV0Pieces executes walkFunc for each locally stored piece, stored with storage
// format V0 in the namespace of the given satellite. If walkFunc returns a non-nil error,
// WalkSatelliteV0Pieces will stop iterating and return the error immediately. The ctx parameter
// parameter is intended specifically to allow canceling iteration early.
//
// If blobStore is nil, the .Stat() and .FullPath() methods of the provided StoredPieceAccess
// instances will not work, but otherwise everything should be ok.
func (db *v0PieceInfoDB) WalkSatelliteV0Pieces(ctx context.Context, blobStore storage.Blobs, satelliteID storj.NodeID, walkFunc func(pieces.StoredPieceAccess) error) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	// TODO: is it worth paging this query? we hope that SNs will not yet have too many V0 pieces.
	pieceInfos, err := db.getAllPiecesOwnedBy(ctx, blobStore, satelliteID)
	if err != nil {
		return err
	}
	// note we must not keep a transaction open with the db when calling walkFunc; the callback
	// might need to make db calls as well
	for i := range pieceInfos {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := walkFunc(&pieceInfos[i]); err != nil {
			return err
		}
	}
	return nil
}

// Get gets piece information by satellite id and piece id.
func (db *v0PieceInfoDB) Get(ctx context.Context, satelliteID storj.NodeID, pieceID storj.PieceID) (_ *pieces.Info, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	info := &pieces.Info{}
	info.SatelliteID = satelliteID
	info.PieceID = pieceID

	var orderLimit []byte
	var uplinkPieceHash []byte
	var pieceExpiration *time.Time

	err = db.QueryRowContext(ctx, `
		SELECT piece_size, piece_creation, piece_expiration, order_limit, uplink_piece_hash
		FROM pieceinfo_
		WHERE satellite_id = ? AND piece_id = ?
	`, satelliteID, pieceID).Scan(&info.PieceSize, &info.PieceCreation, &pieceExpiration, &orderLimit, &uplinkPieceHash)
	if err != nil {
		return nil, ErrPieceInfo.Wrap(err)
	}

	if pieceExpiration != nil {
		info.PieceExpiration = *pieceExpiration
	}

	info.OrderLimit = &pb.OrderLimit{}
	err = pb.Unmarshal(orderLimit, info.OrderLimit)
	if err != nil {
		return nil, ErrPieceInfo.Wrap(err)
	}

	info.UplinkPieceHash = &pb.PieceHash{}
	err = pb.Unmarshal(uplinkPieceHash, info.UplinkPieceHash)
	if err != nil {
		return nil, ErrPieceInfo.Wrap(err)
	}

	return info, nil
}

// Delete deletes piece information.
func (db *v0PieceInfoDB) Delete(ctx context.Context, satelliteID storj.NodeID, pieceID storj.PieceID) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	_, err = db.ExecContext(ctx, `
		DELETE FROM pieceinfo_
		WHERE satellite_id = ?
		  AND piece_id = ?
	`, satelliteID, pieceID)

	return ErrPieceInfo.Wrap(err)
}

// DeleteFailed marks piece as a failed deletion.
func (db *v0PieceInfoDB) DeleteFailed(ctx context.Context, satelliteID storj.NodeID, pieceID storj.PieceID, now time.Time) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	_, err = db.ExecContext(ctx, `
		UPDATE pieceinfo_
		SET deletion_failed_at = ?
		WHERE satellite_id = ?
		  AND piece_id = ?
	`, now.UTC(), satelliteID, pieceID)

	return ErrPieceInfo.Wrap(err)
}

// GetExpired gets ExpiredInfo records for pieces that are expired.
func (db *v0PieceInfoDB) GetExpired(ctx context.Context, expiredAt time.Time, limit int64) (infos []pieces.ExpiredInfo, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	rows, err := db.QueryContext(ctx, `
		SELECT satellite_id, piece_id
		FROM pieceinfo_
		WHERE piece_expiration IS NOT NULL
		AND piece_expiration < ?
		AND ((deletion_failed_at IS NULL) OR deletion_failed_at <> ?)
		ORDER BY satellite_id
		LIMIT ?
	`, expiredAt.UTC(), expiredAt.UTC(), limit)
	if err != nil {
		return nil, ErrPieceInfo.Wrap(err)
	}
	defer func() { err = errs.Combine(err, rows.Close()) }()
	for rows.Next() {
		info := pieces.ExpiredInfo{InPieceInfo: true}
		err = rows.Scan(&info.SatelliteID, &info.PieceID)
		if err != nil {
			return infos, ErrPieceInfo.Wrap(err)
		}
		infos = append(infos, info)
	}
	return infos, rows.Err()
}

type v0StoredPieceAccess struct {
	blobStore      storage.Blobs
	satellite      storj.NodeID
	pieceID        storj.PieceID
	pieceSize      int64
	creationTime   time.Time
	expirationTime *time.Time
	blobInfo       storage.BlobInfo
}

// PieceID returns the piece ID for the piece.
func (v0Access *v0StoredPieceAccess) PieceID() storj.PieceID {
	return v0Access.pieceID
}

// Satellite returns the satellite ID that owns the piece.
func (v0Access *v0StoredPieceAccess) Satellite() (storj.NodeID, error) {
	return v0Access.satellite, nil
}

// BlobRef returns the relevant storage.BlobRef locator for the piece.
func (v0Access *v0StoredPieceAccess) BlobRef() storage.BlobRef {
	return storage.BlobRef{
		Namespace: v0Access.satellite.Bytes(),
		Key:       v0Access.pieceID.Bytes(),
	}
}

func (v0Access *v0StoredPieceAccess) fillInBlobAccess(ctx context.Context) error {
	if v0Access.blobInfo == nil {
		if v0Access.blobStore == nil {
			return errs.New("this v0StoredPieceAccess instance has no blobStore reference, and cannot look up the relevant blob")
		}
		blobInfo, err := v0Access.blobStore.StatWithStorageFormat(ctx, v0Access.BlobRef(), v0Access.StorageFormatVersion())
		if err != nil {
			return err
		}
		v0Access.blobInfo = blobInfo
	}
	return nil
}

// Size gives the size of the piece, and the piece content size (not including the piece header, if applicable).
func (v0Access *v0StoredPieceAccess) Size(ctx context.Context) (int64, int64, error) {
	return v0Access.pieceSize, v0Access.pieceSize, nil
}

// CreationTime returns the piece creation time as given in the original order (which is not
// necessarily the same as the file mtime).
func (v0Access *v0StoredPieceAccess) CreationTime(ctx context.Context) (time.Time, error) {
	return v0Access.creationTime, nil
}

// ModTime returns the same thing as CreationTime for V0 blobs. The intent is for ModTime to
// be a little faster when CreationTime is too slow and the precision is not needed, but in
// this case we already have the exact creation time from the database.
func (v0Access *v0StoredPieceAccess) ModTime(ctx context.Context) (time.Time, error) {
	return v0Access.creationTime, nil
}

// FullPath gives the full path to the on-disk blob file.
func (v0Access *v0StoredPieceAccess) FullPath(ctx context.Context) (string, error) {
	if err := v0Access.fillInBlobAccess(ctx); err != nil {
		return "", err
	}
	return v0Access.blobInfo.FullPath(ctx)
}

// StorageFormatVersion indicates the storage format version used to store the piece.
func (v0Access *v0StoredPieceAccess) StorageFormatVersion() storage.FormatVersion {
	return filestore.FormatV0
}

// Stat does a stat on the on-disk blob file.
func (v0Access *v0StoredPieceAccess) Stat(ctx context.Context) (os.FileInfo, error) {
	if err := v0Access.fillInBlobAccess(ctx); err != nil {
		return nil, err
	}
	return v0Access.blobInfo.Stat(ctx)
}
