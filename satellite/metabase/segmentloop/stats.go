// Copyright (C) 2021 Storj Labs, Inc.
// See LICENSE for copying information.

package segmentloop

import (
	"sync"
	"time"
)

var allObserverStatsCollectors = newObserverStatsCollectors()

type observerStatsCollectors struct {
	mu       sync.Mutex
	observer map[string]*observerStats
}

func newObserverStatsCollectors() *observerStatsCollectors {
	return &observerStatsCollectors{
		observer: make(map[string]*observerStats),
	}
}

func (list *observerStatsCollectors) GetStats(name string) *observerStats {
	list.mu.Lock()
	defer list.mu.Unlock()

	stats, ok := list.observer[name]
	if !ok {
		stats = newObserverStats(name)
		list.observer[name] = stats
	}
	return stats
}

// observerStats tracks the most recent observer stats.
type observerStats struct {
	mu sync.Mutex

	key    interface{}
	total  time.Duration
	inline interface{}
	remote interface{}
}

func newObserverStats(name string) *observerStats {
	return &observerStats{
		key:    nil,
		total:  0,
		inline: nil,
		remote: nil,
	}
}

func (stats *observerStats) Observe(observer *observerContext) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	stats.inline = observer.inline
	stats.remote = observer.remote
}

func (stats *observerStats) Stats(cb func(key interface{}, field string, val float64)) {
	stats.mu.Lock()
	defer stats.mu.Unlock()

	cb(stats.key, "sum", stats.total.Seconds())
}
