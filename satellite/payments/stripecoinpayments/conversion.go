// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package stripecoinpayments

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"

	"runtime"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/currency"
	"storj.io/common/sync2"
)

// convertToCents convert amount to USD cents with given rate.
func convertToCents(rate decimal.Decimal, amount currency.Amount) int64 {
	amountDecimal := amount.AsDecimal()
	usd := amountDecimal.Mul(rate)
	usdCents := usd.Shift(2)
	return usdCents.Round(0).IntPart()
}

// ErrConversion defines version service error.
var ErrConversion = errs.Class("conversion service")

// ConversionService updates conversion rates in a loop.
//
// architecture: Service
type ConversionService struct {
	log     *zap.Logger
	service *Service
	Cycle   sync2.Cycle
}

// NewConversionService creates new instance of ConversionService.
func NewConversionService(log *zap.Logger, service *Service, interval time.Duration) *ConversionService {
	return &ConversionService{
		log:     log,
		service: service,
		Cycle:   *sync2.NewCycle(interval),
	}
}

// Run runs loop which updates conversion rates for service.
func (conversion *ConversionService) Run(ctx context.Context) (err error) {
	return ErrConversion.Wrap(conversion.Cycle.Run(ctx,
		func(ctx context.Context) error {
			pc, _, _, _ := runtime.Caller(0)
			ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
			defer span.End()
			conversion.log.Debug("running conversion rates update cycle")

			if err := conversion.service.UpdateRates(ctx); err != nil {
				conversion.log.Error("conversion rates update cycle failed", zap.Error(ErrChore.Wrap(err)))
			}

			return nil
		},
	))
}

// Close closes underlying cycle.
func (conversion *ConversionService) Close() (err error) {
	pc, _, _, _ := runtime.Caller(0)
	_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), runtime.FuncForPC(pc).Name())
	defer span.End()

	conversion.Cycle.Close()
	return nil
}
