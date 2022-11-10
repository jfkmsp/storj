// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package nodestats

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
	"os"

	"runtime"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/pb"
	"storj.io/common/rpc"
	"storj.io/common/storj"
	"storj.io/storj/storagenode/pricing"
	"storj.io/storj/storagenode/reputation"
	"storj.io/storj/storagenode/storageusage"
	"storj.io/storj/storagenode/trust"
)

var (
	// NodeStatsServiceErr defines node stats service error.
	NodeStatsServiceErr = errs.Class("nodestats")
)

// Client encapsulates NodeStatsClient with underlying connection.
//
// architecture: Client
type Client struct {
	conn *rpc.Conn
	pb.DRPCNodeStatsClient
}

// Close closes underlying client connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Service retrieves info from satellites using an rpc client.
//
// architecture: Service
type Service struct {
	log *zap.Logger

	dialer rpc.Dialer
	trust  *trust.Pool
}

// NewService creates new instance of service.
func NewService(log *zap.Logger, dialer rpc.Dialer, trust *trust.Pool) *Service {
	return &Service{
		log:    log,
		dialer: dialer,
		trust:  trust,
	}
}

// GetReputationStats retrieves reputation stats from particular satellite.
func (s *Service) GetReputationStats(ctx context.Context, satelliteID storj.NodeID) (_ *reputation.Stats, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	var meter = global.MeterProvider().Meter(os.Getenv("SERVICE_NAME"))
	defer span.End()

	client, err := s.dial(ctx, satelliteID)
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}
	defer func() { err = errs.Combine(err, client.Close()) }()

	resp, err := client.GetStats(ctx, &pb.GetStatsRequest{})
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}

	audit := resp.GetAuditCheck()

	counter, _ := meter.SyncInt64().Histogram("audit_success_count", instrument.WithDescription("satellite_id: "+satelliteID.String()))
	counter.Record(ctx, audit.GetSuccessCount())
	counter, _ = meter.SyncInt64().Histogram("audit_total_count", instrument.WithDescription("satellite_id: "+satelliteID.String()))
	counter.Record(ctx, audit.GetTotalCount())
	floatCounter, _ := meter.SyncFloat64().Histogram("audit_reputation_score", instrument.WithDescription("satellite_id: "+satelliteID.String()))
	floatCounter.Record(ctx, audit.GetReputationScore())
	floatCounter, _ = meter.SyncFloat64().Histogram("suspension_score", instrument.WithDescription("satellite_id: "+satelliteID.String()))
	floatCounter.Record(ctx, audit.GetUnknownReputationScore())
	floatCounter, _ = meter.SyncFloat64().Histogram("online_score", instrument.WithDescription("satellite_id: "+satelliteID.String()))
	floatCounter.Record(ctx, resp.GetOnlineScore())

	return &reputation.Stats{
		SatelliteID: satelliteID,
		Audit: reputation.Metric{
			TotalCount:   audit.GetTotalCount(),
			SuccessCount: audit.GetSuccessCount(),
			Alpha:        audit.GetReputationAlpha(),
			Beta:         audit.GetReputationBeta(),
			Score:        audit.GetReputationScore(),
			UnknownAlpha: audit.GetUnknownReputationAlpha(),
			UnknownBeta:  audit.GetUnknownReputationBeta(),
			UnknownScore: audit.GetUnknownReputationScore(),
		},
		OnlineScore:          resp.OnlineScore,
		DisqualifiedAt:       resp.GetDisqualified(),
		SuspendedAt:          resp.GetSuspended(),
		OfflineSuspendedAt:   resp.GetOfflineSuspended(),
		OfflineUnderReviewAt: resp.GetOfflineUnderReview(),
		VettedAt:             resp.GetVettedAt(),
		AuditHistory:         resp.GetAuditHistory(),
		UpdatedAt:            time.Now(),
		JoinedAt:             resp.JoinedAt,
	}, nil
}

// GetDailyStorageUsage returns daily storage usage over a period of time for a particular satellite.
func (s *Service) GetDailyStorageUsage(ctx context.Context, satelliteID storj.NodeID, from, to time.Time) (_ []storageusage.Stamp, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	client, err := s.dial(ctx, satelliteID)
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}
	defer func() { err = errs.Combine(err, client.Close()) }()

	resp, err := client.DailyStorageUsage(ctx, &pb.DailyStorageUsageRequest{From: from, To: to})
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}

	return fromSpaceUsageResponse(resp, satelliteID), nil
}

// GetPricingModel returns pricing model of specific satellite.
func (s *Service) GetPricingModel(ctx context.Context, satelliteID storj.NodeID) (_ *pricing.Pricing, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	client, err := s.dial(ctx, satelliteID)
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}
	defer func() { err = errs.Combine(err, client.Close()) }()

	pricingModel, err := client.PricingModel(ctx, &pb.PricingModelRequest{})
	if err != nil {
		return nil, NodeStatsServiceErr.Wrap(err)
	}

	return &pricing.Pricing{
		SatelliteID:     satelliteID,
		EgressBandwidth: pricingModel.EgressBandwidthPrice,
		RepairBandwidth: pricingModel.RepairBandwidthPrice,
		AuditBandwidth:  pricingModel.AuditBandwidthPrice,
		DiskSpace:       pricingModel.DiskSpacePrice,
	}, nil
}

// dial dials the NodeStats client for the satellite by id.
func (s *Service) dial(ctx context.Context, satelliteID storj.NodeID) (_ *Client, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
	defer span.End()

	nodeurl, err := s.trust.GetNodeURL(ctx, satelliteID)
	if err != nil {
		return nil, errs.New("unable to find satellite %s: %w", satelliteID, err)
	}

	conn, err := s.dialer.DialNodeURL(ctx, nodeurl)
	if err != nil {
		return nil, errs.New("unable to connect to the satellite %s: %w", satelliteID, err)
	}

	return &Client{
		conn:                conn,
		DRPCNodeStatsClient: pb.NewDRPCNodeStatsClient(conn),
	}, nil
}

// fromSpaceUsageResponse get DiskSpaceUsage slice from pb.SpaceUsageResponse.
func fromSpaceUsageResponse(resp *pb.DailyStorageUsageResponse, satelliteID storj.NodeID) []storageusage.Stamp {
	var stamps []storageusage.Stamp

	for _, pbUsage := range resp.GetDailyStorageUsage() {
		stamps = append(stamps, storageusage.Stamp{
			SatelliteID:     satelliteID,
			AtRestTotal:     pbUsage.AtRestTotal,
			IntervalStart:   pbUsage.Timestamp,
			IntervalEndTime: pbUsage.IntervalEndTime,
		})
	}

	return stamps
}
