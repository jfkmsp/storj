// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package checker

import (
	"fmt"
	"storj.io/common/uuid"
)

// statsCollector holds a *stats for each redundancy scheme
// seen by the checker. These are chained into the monkit scope for
// monitoring as they are initialized.
type statsCollector struct {
	stats map[string]*stats
}

func newStatsCollector() *statsCollector {
	return &statsCollector{
		stats: make(map[string]*stats),
	}
}

func (collector *statsCollector) getStatsByRS(rs string) *stats {
	stats, ok := collector.stats[rs]
	if !ok {
		stats = newStats(rs)
		collector.stats[rs] = stats
	}
	return stats
}

// collectAggregates transfers the iteration aggregates into the
// respective stats monkit metrics at the end of each checker iteration.
// iterationAggregates is then cleared.
func (collector *statsCollector) collectAggregates() {
	for _, stats := range collector.stats {
		stats.collectAggregates()
		stats.iterationAggregates = new(aggregateStats)
	}
}

// stats is used for collecting and reporting checker metrics.
//
// add any new metrics tagged with rs_scheme to this struct and set them
// in newStats.
type stats struct {
	iterationAggregates *aggregateStats

	objectsChecked                  interface{}
	remoteSegmentsChecked           interface{}
	remoteSegmentsNeedingRepair     interface{}
	newRemoteSegmentsNeedingRepair  interface{}
	remoteSegmentsLost              interface{}
	objectsLost                     interface{}
	remoteSegmentsFailedToCheck     interface{}
	remoteSegmentsHealthyPercentage interface{}

	// remoteSegmentsOverThreshold[0]=# of healthy=rt+1, remoteSegmentsOverThreshold[1]=# of healthy=rt+2, etc...
	remoteSegmentsOverThreshold1 interface{}
	remoteSegmentsOverThreshold2 interface{}
	remoteSegmentsOverThreshold3 interface{}
	remoteSegmentsOverThreshold4 interface{}
	remoteSegmentsOverThreshold5 interface{}

	segmentsBelowMinReq         interface{}
	segmentTotalCount           interface{}
	segmentHealthyCount         interface{}
	segmentAge                  interface{}
	segmentHealth               interface{}
	injuredSegmentHealth        interface{}
	segmentTimeUntilIrreparable interface{}
}

// aggregateStats tallies data over the full checker iteration.
type aggregateStats struct {
	objectsChecked                 int64
	remoteSegmentsChecked          int64
	remoteSegmentsNeedingRepair    int64
	newRemoteSegmentsNeedingRepair int64
	remoteSegmentsLost             int64
	remoteSegmentsFailedToCheck    int64
	objectsLost                    []uuid.UUID

	// remoteSegmentsOverThreshold[0]=# of healthy=rt+1, remoteSegmentsOverThreshold[1]=# of healthy=rt+2, etc...
	remoteSegmentsOverThreshold [5]int64
}

func newStats(rs string) *stats {
	return &stats{
		iterationAggregates:             new(aggregateStats),
		objectsChecked:                  nil,
		remoteSegmentsChecked:           nil,
		remoteSegmentsNeedingRepair:     nil,
		newRemoteSegmentsNeedingRepair:  nil,
		remoteSegmentsLost:              nil,
		objectsLost:                     nil,
		remoteSegmentsFailedToCheck:     nil,
		remoteSegmentsHealthyPercentage: nil,
		remoteSegmentsOverThreshold1:    nil,
		remoteSegmentsOverThreshold2:    nil,
		remoteSegmentsOverThreshold3:    nil,
		remoteSegmentsOverThreshold4:    nil,
		remoteSegmentsOverThreshold5:    nil,
		segmentsBelowMinReq:             nil,
		segmentTotalCount:               nil,
		segmentHealthyCount:             nil,
		segmentAge:                      nil,
		segmentHealth:                   nil,
		injuredSegmentHealth:            nil,
		segmentTimeUntilIrreparable:     nil,
	}
}

func (stats *stats) collectAggregates() {

}

// Stats implements the monkit.StatSource interface.
func (stats *stats) Stats(cb func(key interface{}, field string, val float64)) {

}

func getRSString(min, repair, success, total int) string {
	return fmt.Sprintf("%d/%d/%d/%d", min, repair, success, total)
}
