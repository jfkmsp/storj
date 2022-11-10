// Copyright (C) 2022 Storj Labs, Inc.
// See LICENSE for copying information.

package billing

import (
	"context"
	"go.opentelemetry.io/otel"
	"os"
	"runtime"
	"time"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/sync2"
)

// ChoreErr is billing chore err class.
var ChoreErr = errs.Class("billing chore")

// Chore periodically queries for new billing transactions from payment type.
//
// architecture: Chore
type Chore struct {
	log              *zap.Logger
	paymentTypes     []PaymentType
	transactionsDB   TransactionsDB
	TransactionCycle *sync2.Cycle

	disableLoop bool
}

// NewChore creates new chore.
func NewChore(log *zap.Logger, paymentTypes []PaymentType, transactionsDB TransactionsDB, interval time.Duration, disableLoop bool) *Chore {
	return &Chore{
		log:              log,
		paymentTypes:     paymentTypes,
		transactionsDB:   transactionsDB,
		TransactionCycle: sync2.NewCycle(interval),
		disableLoop:      disableLoop,
	}
}

// Run runs billing transaction loop.
func (chore *Chore) Run(ctx context.Context) (err error) {
	return chore.TransactionCycle.Run(ctx, func(ctx context.Context) error {
		pc, _, _, _ := runtime.Caller(0)
		ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name())
		defer span.End()
		if chore.disableLoop {
			chore.log.Debug("Skipping chore iteration as loop is disabled", zap.Bool("disableLoop", chore.disableLoop))
			return nil
		}

		for _, paymentType := range chore.paymentTypes {
			lastTransactionTime, lastTransactionMetadata, err := chore.transactionsDB.LastTransaction(ctx, paymentType.Source(), paymentType.Type())
			if err != nil && !errs.Is(err, ErrNoTransactions) {
				chore.log.Error("unable to determine timestamp of last transaction", zap.Error(ChoreErr.Wrap(err)))
				continue
			}
			transactions, err := paymentType.GetNewTransactions(ctx, lastTransactionTime, lastTransactionMetadata)
			if err != nil {
				chore.log.Error("unable to get new billing transactions", zap.Error(ChoreErr.Wrap(err)))
				continue
			}
			for _, transaction := range transactions {
				_, err = chore.transactionsDB.Insert(ctx, transaction)
				if err != nil {
					chore.log.Error("error storing transaction to db", zap.Error(ChoreErr.Wrap(err)))
					// we need to halt storing transactions if one fails, so that it can be tried again on the next loop.
					break
				}
			}
		}
		return nil
	})
}

// Close closes all underlying resources.
func (chore *Chore) Close() (err error) {
	pc, _, _, _ := runtime.Caller(0)
	_, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(context.Background(), runtime.FuncForPC(pc).Name())
	defer span.End()
	chore.TransactionCycle.Close()
	return nil
}
