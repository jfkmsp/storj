// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package storelogger

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"os"

	"runtime"
	"strconv"
	"sync/atomic"

	"go.uber.org/zap"

	"storj.io/storj/storage"
)

var id int64

// Logger implements a zap.Logger for storage.KeyValueStore.
type Logger struct {
	log   *zap.Logger
	store storage.KeyValueStore
}

// New creates a new Logger with log and store.
func New(log *zap.Logger, store storage.KeyValueStore) *Logger {
	loggerid := atomic.AddInt64(&id, 1)
	name := strconv.Itoa(int(loggerid))
	return &Logger{log.Named(name), store}
}

// LookupLimit returns the maximum limit that is allowed.
func (store *Logger) LookupLimit() int { return store.store.LookupLimit() }

// Put adds a value to store.
func (store *Logger) Put(ctx context.Context, key storage.Key, value storage.Value) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("Put", zap.ByteString("key", key), zap.Int("value length", len(value)), zap.Binary("truncated value", truncate(value)))
	return store.store.Put(ctx, key, value)
}

// Get gets a value to store.
func (store *Logger) Get(ctx context.Context, key storage.Key) (_ storage.Value, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("Get", zap.ByteString("key", key))
	return store.store.Get(ctx, key)
}

// GetAll gets all values from the store corresponding to keys.
func (store *Logger) GetAll(ctx context.Context, keys storage.Keys) (_ storage.Values, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("GetAll", zap.Any("keys", keys))
	return store.store.GetAll(ctx, keys)
}

// Delete deletes key and the value.
func (store *Logger) Delete(ctx context.Context, key storage.Key) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("Delete", zap.ByteString("key", key))
	return store.store.Delete(ctx, key)
}

// DeleteMultiple deletes keys ignoring missing keys.
func (store *Logger) DeleteMultiple(ctx context.Context, keys []storage.Key) (_ storage.Items, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name(), trace.WithAttributes(attribute.Int("keys length", len(keys))))
	defer span.End()
	store.log.Debug("DeleteMultiple", zap.Any("keys", keys))
	return store.store.DeleteMultiple(ctx, keys)
}

// List lists all keys starting from first and upto limit items.
func (store *Logger) List(ctx context.Context, first storage.Key, limit int) (_ storage.Keys, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	keys, err := store.store.List(ctx, first, limit)
	store.log.Debug("List", zap.ByteString("first", first), zap.Int("limit", limit), zap.Strings("keys", keys.Strings()))
	return keys, err
}

// Iterate iterates over items based on opts.
func (store *Logger) Iterate(ctx context.Context, opts storage.IterateOptions, fn func(context.Context, storage.Iterator) error) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("Iterate",
		zap.ByteString("prefix", opts.Prefix),
		zap.ByteString("first", opts.First),
		zap.Bool("recurse", opts.Recurse),
	)
	return store.store.Iterate(ctx, opts, func(ctx context.Context, it storage.Iterator) error {
		return fn(ctx, storage.IteratorFunc(func(ctx context.Context, item *storage.ListItem) bool {
			ok := it.Next(ctx, item)
			if ok {
				store.log.Debug("  ",
					zap.ByteString("key", item.Key),
					zap.Int("value length", len(item.Value)),
					zap.Binary("truncated value", truncate(item.Value)),
				)
			}
			return ok
		}))
	})
}

// IterateWithoutLookupLimit calls the callback with an iterator over the keys, but doesn't enforce default limit on opts.
func (store *Logger) IterateWithoutLookupLimit(ctx context.Context, opts storage.IterateOptions, fn func(context.Context, storage.Iterator) error) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("IterateWithoutLookupLimit",
		zap.ByteString("prefix", opts.Prefix),
		zap.ByteString("first", opts.First),
		zap.Bool("recurse", opts.Recurse),
	)
	return store.store.IterateWithoutLookupLimit(ctx, opts, func(ctx context.Context, it storage.Iterator) error {
		return fn(ctx, storage.IteratorFunc(func(ctx context.Context, item *storage.ListItem) bool {
			ok := it.Next(ctx, item)
			if ok {
				store.log.Debug("  ",
					zap.ByteString("key", item.Key),
					zap.Int("value length", len(item.Value)),
					zap.Binary("truncated value", truncate(item.Value)),
				)
			}
			return ok
		}))
	})
}

// Close closes the store.
func (store *Logger) Close() error {
	store.log.Debug("Close")
	return store.store.Close()
}

// CompareAndSwap atomically compares and swaps oldValue with newValue.
func (store *Logger) CompareAndSwap(ctx context.Context, key storage.Key, oldValue, newValue storage.Value) (err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	store.log.Debug("CompareAndSwap", zap.ByteString("key", key),
		zap.Int("old value length", len(oldValue)), zap.Int("new value length", len(newValue)),
		zap.Binary("truncated old value", truncate(oldValue)), zap.Binary("truncated new value", truncate(newValue)))
	return store.store.CompareAndSwap(ctx, key, oldValue, newValue)
}

func truncate(v storage.Value) (t []byte) {
	if len(v)-1 < 10 {
		t = []byte(v)
	} else {
		t = v[:10]
	}
	return t
}
