package payment_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_PaymentEndpoints drives every payment-package outbound
// method against the spec server. Each row is a thin closure that
// constructs a minimal valid request and invokes the SDK method;
// the spec server validates against the pinned swagger.yaml's
// required-set and reports any missing required field by path.
//
// This is the regression backstop for COMPLETENESS_REVIEW_V0.2 §2.1
// (the four GET methods that v0.2.0 shipped without
// `externalReferenceUid`). Today's signatures send both keys and
// the spec server confirms it on every CI run.
func TestSpec_PaymentEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /auth",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).Auth(context.Background(), specSampleAuthRequest())
				return err
			},
		},
		{
			Op: "POST /capture",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).Capture(context.Background(), specSampleCaptureRequest())
				return err
			},
		},
		{
			Op: "POST /voidAuth",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).VoidAuth(context.Background(), specSampleVoidAuthRequest())
				return err
			},
		},
		{
			Op: "GET /query-auth",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).QueryAuth(context.Background(), "r-spec-1", "u-spec-1")
				return err
			},
		},
		{
			Op: "GET /query-capture",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).QueryCapture(context.Background(), "r-spec-1", "u-spec-1")
				return err
			},
		},
		{
			Op: "GET /query-voidAuth",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).QueryVoidAuth(context.Background(), "r-spec-1", "u-spec-1")
				return err
			},
		},
		{
			Op: "POST /payment-precheck",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).PaymentPreCheck(context.Background(), specSamplePreCheckRequest())
				return err
			},
			// SDK's PaymentPreCheckSubOrder carries `subOrderId` (used by
			// /auth and /capture) but the spec's pre-check schema declares
			// commerce-domain fields (`categoryId`, `categoryOneName`,
			// `merchantId`, `skuId`) plus a top-level `event`. The
			// upstream is "Initial draft — version 1, finalised
			// case-by-case via bilateral integration agreement"; the
			// fields are tracked toward partner clarification before the
			// SDK shape grows. Skipping for now keeps the spec server
			// honest about the gap without forcing a speculative API
			// extension. Drift the pinned spec or close the partner Q
			// → re-evaluate.
			SkipRequired: []string{
				"event",
				"subOrders[].categoryId",
				"subOrders[].categoryOneName",
				"subOrders[].merchantId",
				"subOrders[].skuId",
			},
		},
		{
			Op: "POST /payment-plan",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).PaymentPlan(context.Background(), specSamplePaymentPlanRequest())
				return err
			},
			// Same partner-pending rationale as /payment-precheck above;
			// /payment-plan layers on a deeper extendInfo.ecommerceOrder
			// tree that the SDK does not yet model. The `sessionid`
			// header gap is also partner-pending — /auth carries it via
			// AuthRequest.Sessionid; PaymentPlanRequest does not yet
			// expose an equivalent. All entries here trace back to the
			// spec's "Initial draft" disclaimer and will close as
			// partner integration agreements solidify the shape.
			SkipRequired: []string{
				"sessionid",
				"extendInfo.ecommerceOrder.ecommerceSubOrders",
				"extendInfo.ecommerceOrder.orderAmount",
				"extendInfo.paymentType",
				"subOrders[].categoryId",
				"subOrders[].categoryOneName",
				"subOrders[].discounts[].discountDetails",
				"subOrders[].discounts[].discountDetails[].discountType",
				"subOrders[].discounts[].funder",
				"subOrders[].discounts[].totalTenor",
				"subOrders[].merchantId",
				"subOrders[].skuId",
			},
		},
	})
}

// ---------- minimal request constructors (one per endpoint) ----------

func specSampleAuthRequest() *payment.AuthRequest {
	return &payment.AuthRequest{
		RequestID:            "r-spec-auth",
		ExternalReferenceUID: "u-spec-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders: []payment.SubOrder{
			{SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
		},
		Sessionid: "session-spec",
	}
}

func specSampleCaptureRequest() *payment.CaptureRequest {
	return &payment.CaptureRequest{
		RequestID:            "r-spec-capture",
		ExternalReferenceUID: "u-spec-1",
		AuthOrderID:          "AUTH-SPEC-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders: []payment.SubOrder{
			{SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
		},
	}
}

func specSampleVoidAuthRequest() *payment.VoidAuthRequest {
	return &payment.VoidAuthRequest{
		RequestID:            "r-spec-void",
		ExternalReferenceUID: "u-spec-1",
		AuthOrderID:          "AUTH-SPEC-1",
	}
}

func specSamplePreCheckRequest() *payment.PaymentPreCheckRequest {
	return &payment.PaymentPreCheckRequest{
		RequestID:            "r-spec-precheck",
		ExternalReferenceUID: "u-spec-1",
		TotalAmount:          1500000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
		},
	}
}

func specSamplePaymentPlanRequest() *payment.PaymentPlanRequest {
	return &payment.PaymentPlanRequest{
		RequestID:            "r-spec-plan",
		ExternalReferenceUID: "u-spec-1",
		TotalAmount:          1500000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PlanSubOrder{
			{SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
		},
	}
}
