// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package pointerverification

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"sync"

	"storj.io/common/identity"
	"storj.io/common/storj"
	"storj.io/storj/satellite/overlay"
)

// IdentityCache implements caching of *identity.PeerIdentity.
type IdentityCache struct {
	db overlay.PeerIdentities

	mu     sync.RWMutex
	cached map[storj.NodeID]*identity.PeerIdentity
}

// NewIdentityCache returns an IdentityCache.
func NewIdentityCache(db overlay.PeerIdentities) *IdentityCache {
	return &IdentityCache{
		db:     db,
		cached: map[storj.NodeID]*identity.PeerIdentity{},
	}
}

// GetCached returns the peer identity in the cache.
func (cache *IdentityCache) GetCached(ctx context.Context, id storj.NodeID) *identity.PeerIdentity {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	return cache.cached[id]
}

// GetUpdated returns the identity from database and updates the cache.
func (cache *IdentityCache) GetUpdated(ctx context.Context, id storj.NodeID) (_ *identity.PeerIdentity, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	identity, err := cache.db.Get(ctx, id)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.cached[id] = identity

	return identity, nil
}

// EnsureCached loads any missing identity into cache.
func (cache *IdentityCache) EnsureCached(ctx context.Context, nodes []storj.NodeID) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	missing := []storj.NodeID{}

	cache.mu.RLock()
	for _, node := range nodes {
		if _, ok := cache.cached[node]; !ok {
			missing = append(missing, node)
		}
	}
	cache.mu.RUnlock()

	if len(missing) == 0 {
		return nil
	}

	// There might be a race during updating, however we'll "reupdate" later if there's a failure.
	// The common path doesn't end up here.

	identities, err := cache.db.BatchGet(ctx, missing)
	if err != nil {
		return Error.Wrap(err)
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	for _, identity := range identities {
		cache.cached[identity.ID] = identity
	}

	return nil
}
