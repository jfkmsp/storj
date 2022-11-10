// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package metainfo

import (
	"context"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"os"
	"sort"
	"strings"

	"github.com/blang/semver"
	"go.uber.org/zap"

	"storj.io/common/useragent"
)

const uplinkProduct = "uplink"

type transfer string

const upload = transfer("upload")
const download = transfer("download")

var knownUserAgents = []string{
	"rclone", "gateway-st", "gateway-mt", "linksharing", "uplink-cli", "transfer-sh", "filezilla", "duplicati",
	"comet", "orbiter", "uplink-php", "nextcloud", "aws-cli", "ipfs-go-ds-storj", "storj-downloader",
}

type versionOccurrence struct {
	Product string
	Version string
	Method  string
}

type versionCollector struct {
	log *zap.Logger
}

func newVersionCollector(log *zap.Logger) *versionCollector {
	return &versionCollector{
		log: log,
	}
}

func (vc *versionCollector) collect(useragentRaw []byte, method string) {
	if len(useragentRaw) == 0 {
		return
	}

	entries, err := useragent.ParseEntries(useragentRaw)
	if err != nil {
		vc.log.Warn("unable to collect uplink version", zap.Error(err))
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents")
		span.AddEvent("user_agents", trace.WithAttributes(attribute.String("user_agent", "unparseable")))
		span.End()
		return
	}

	// foundProducts tracks potentially multiple noteworthy products names from the user-agent
	var foundProducts []string
	for _, entry := range entries {
		product := strings.ToLower(entry.Product)
		if product == uplinkProduct {
			vo := versionOccurrence{Product: product, Version: entry.Version, Method: method}
			vc.sendUplinkMetric(vo)
		} else if contains(knownUserAgents, product) && !contains(foundProducts, product) {
			foundProducts = append(foundProducts, product)
		}
	}

	if len(foundProducts) > 0 {
		sort.Strings(foundProducts)
		// concatenate all known products for this metric, EG "gateway-mt + rclone"
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents")
		span.AddEvent("user_agents", trace.WithAttributes(attribute.String("user_agent", strings.Join(foundProducts, " + "))))
		span.End()
	} else { // lets keep also general value for user agents with no known product
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents")
		span.AddEvent("user_agents", trace.WithAttributes(attribute.String("user_agent", "other")))
		span.End()
	}
}

func (vc *versionCollector) sendUplinkMetric(vo versionOccurrence) {
	if vo.Version == "" {
		vo.Version = "unknown"
	} else {
		// use only minor to avoid using too many resources and
		// minimize risk of abusing by sending lots of different versions
		semVer, err := semver.ParseTolerant(vo.Version)
		if err != nil {
			vc.log.Warn("invalid uplink library user agent version", zap.String("version", vo.Version), zap.Error(err))
			return
		}

		// keep number of possible versions very limited
		if semVer.Major != 1 || semVer.Minor > 30 {
			vc.log.Warn("invalid uplink library user agent version", zap.String("version", vo.Version), zap.Error(err))
			return
		}

		vo.Version = fmt.Sprintf("v%d.%d", 1, semVer.Minor)
	}

	_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "uplink_versions")
	span.AddEvent("uplink_versions", trace.WithAttributes(attribute.String("version", vo.Version)), trace.WithAttributes(attribute.String("method", vo.Method)))
	span.End()
}

func (vc *versionCollector) collectTransferStats(useragentRaw []byte, transfer transfer, transferSize int) {
	entries, err := useragent.ParseEntries(useragentRaw)
	if err != nil {
		vc.log.Warn("unable to collect transfer statistics", zap.Error(err))
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents_transfer_stats")
		span.AddEvent("user_agents_transfer_stats", trace.WithAttributes(attribute.String("user_agent", "unparseable")), trace.WithAttributes(attribute.String("type", string(transfer))), trace.WithAttributes(attribute.Int("transferSize", transferSize)))
		span.End()
		return
	}

	// foundProducts tracks potentially multiple noteworthy products names from the user-agent
	var foundProducts []string
	for _, entry := range entries {
		product := strings.ToLower(entry.Product)
		if contains(knownUserAgents, product) && !contains(foundProducts, product) {
			foundProducts = append(foundProducts, product)
		}
	}

	if len(foundProducts) > 0 {
		sort.Strings(foundProducts)
		// concatenate all known products for this metric, EG "gateway-mt + rclone"
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents_transfer_stats")
		span.AddEvent("user_agents_transfer_stats", trace.WithAttributes(attribute.String("user_agent", strings.Join(foundProducts, " + "))), trace.WithAttributes(attribute.String("type", string(transfer))), trace.WithAttributes(attribute.Int("transferSize", transferSize)))
		span.End()
	} else { // lets keep also general value for user agents with no known product
		_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), "user_agents_transfer_stats")
		span.AddEvent("user_agents_transfer_stats", trace.WithAttributes(attribute.String("user_agent", "other")), trace.WithAttributes(attribute.String("type", string(transfer))), trace.WithAttributes(attribute.Int("transferSize", transferSize)))
		span.End()
	}
}

// contains returns true if the given string is contained in the given slice.
func contains(slice []string, testValue string) bool {
	for _, sliceValue := range slice {
		if sliceValue == testValue {
			return true
		}
	}
	return false
}
