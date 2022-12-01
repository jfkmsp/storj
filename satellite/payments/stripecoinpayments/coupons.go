// Copyright (C) 2019 Storj Labs, Inc.
// See LICENSE for copying information.

package stripecoinpayments

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"os"

	"runtime"
	"time"

	"github.com/stripe/stripe-go/v72"

	"storj.io/common/uuid"
	"storj.io/storj/satellite/payments"
)

// ensures that coupons implements payments.Coupons.
var _ payments.Coupons = (*coupons)(nil)

// coupons is an implementation of payments.Coupons.
//
// architecture: Service
type coupons struct {
	service *Service
}

// ApplyCouponCode attempts to apply a coupon code to the user via Stripe.
func (coupons *coupons) ApplyCouponCode(ctx context.Context, userID uuid.UUID, couponCode string) (_ *payments.Coupon, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name(),
		trace.WithAttributes(attribute.String("userID", userID.String())),
		trace.WithAttributes(attribute.String("couponCode", couponCode)))
	defer span.End()

	promoCodeIter := coupons.service.stripeClient.PromoCodes().List(&stripe.PromotionCodeListParams{
		Code: stripe.String(couponCode),
	})
	if !promoCodeIter.Next() {
		return nil, ErrInvalidCoupon.New("Invalid coupon code")
	}
	promoCode := promoCodeIter.PromotionCode()

	customerID, err := coupons.service.db.Customers().GetCustomerID(ctx, userID)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	params := &stripe.CustomerParams{
		PromotionCode: stripe.String(promoCode.ID),
	}
	params.AddExpand("discount.promotion_code")

	customer, err := coupons.service.stripeClient.Customers().Update(customerID, params)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	if customer.Discount == nil || customer.Discount.Coupon == nil {
		return nil, Error.New("invalid discount after coupon code application; user ID:%s, customer ID:%s", userID, customerID)
	}

	return stripeDiscountToPaymentsCoupon(customer.Discount)
}

// GetByUserID returns the coupon applied to the user.
func (coupons *coupons) GetByUserID(ctx context.Context, userID uuid.UUID) (_ *payments.Coupon, err error) {
	pc, _, _, _ := runtime.Caller(0)
	ctx, span := otel.Tracer(os.Getenv("SERVICE_NAME")).Start(ctx, runtime.FuncForPC(pc).Name(),
		trace.WithAttributes(attribute.String("userID", userID.String())))
	defer span.End()

	customerID, err := coupons.service.db.Customers().GetCustomerID(ctx, userID)
	if err != nil {
		return nil, Error.Wrap(err)
	}

	params := &stripe.CustomerParams{}
	params.AddExpand("discount.promotion_code")

	customer, err := coupons.service.stripeClient.Customers().Get(customerID, params)
	if err != nil {
		return nil, err
	}

	if customer.Discount == nil || customer.Discount.Coupon == nil {
		return nil, nil
	}

	return stripeDiscountToPaymentsCoupon(customer.Discount)
}

// stripeDiscountToPaymentsCoupon converts a Stripe discount to a payments.Coupon.
func stripeDiscountToPaymentsCoupon(dc *stripe.Discount) (coupon *payments.Coupon, err error) {
	if dc == nil {
		return nil, Error.New("discount is nil")
	}

	if dc.Coupon == nil {
		return nil, Error.New("discount.Coupon is nil")
	}

	coupon = &payments.Coupon{
		ID:         dc.ID,
		Name:       dc.Coupon.Name,
		AmountOff:  dc.Coupon.AmountOff,
		PercentOff: dc.Coupon.PercentOff,
		AddedAt:    time.Unix(dc.Start, 0),
		ExpiresAt:  time.Unix(dc.End, 0),
		Duration:   payments.CouponDuration(dc.Coupon.Duration),
	}

	if dc.PromotionCode != nil {
		coupon.PromoCode = dc.PromotionCode.Code
	}

	return coupon, nil
}
