// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package repairer

import (
	"fmt"
)

// statsCollector holds a *stats for each redundancy scheme
// seen by the repairer. These are chained into the monkit scope for
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

// stats is used for collecting and reporting repairer metrics.
//
// add any new metrics tagged with rs_scheme to this struct and set them
// in newStats.
type stats struct {
	repairAttempts              interface{}
	repairSegmentSize           interface{}
	repairerSegmentsBelowMinReq interface{}
	repairerNodesUnavailable    interface{}
	repairUnnecessary           interface{}
	healthyRatioBeforeRepair    interface{}
	repairTooManyNodesFailed    interface{}
	repairFailed                interface{}
	repairPartial               interface{}
	repairSuccess               interface{}
	healthyRatioAfterRepair     interface{}
	segmentTimeUntilRepair      interface{}
	segmentRepairCount          interface{}
}

func newStats(rs string) *stats {
	return &stats{}
}

// Stats implements the monkit.StatSource interface.
func (stats *stats) Stats(cb func(key interface{}, field string, val float64)) {

}

func getRSString(min, repair, success, total int) string {
	return fmt.Sprintf("%d/%d/%d/%d", min, repair, success, total)
}
