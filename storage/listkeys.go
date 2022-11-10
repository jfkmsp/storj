// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package storage

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
)

// ListKeys returns keys starting from first and upto limit.
// limit is capped to LookupLimit.
func ListKeys(ctx context.Context, store KeyValueStore, first Key, limit int) (_ Keys, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()
	if limit <= 0 || limit > store.LookupLimit() {
		limit = store.LookupLimit()
	}

	keys := make(Keys, 0, limit)
	err = store.Iterate(ctx, IterateOptions{
		First:   first,
		Recurse: true,
	}, func(ctx context.Context, it Iterator) error {
		var item ListItem
		for ; limit > 0 && it.Next(ctx, &item); limit-- {
			if item.Key == nil {
				panic("nil key")
			}
			keys = append(keys, CloneKey(item.Key))
		}
		return nil
	})

	return keys, err
}
