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
		},
		{
			Op: "POST /payment-plan",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).PaymentPlan(context.Background(), specSamplePaymentPlanRequest())
				return err
			},
		},
		{
			Op: "POST /riplay",
			Run: func(c *atomefin.Client) error {
				_, err := payment.New(c).Riplay(context.Background(), specSampleRiplayRequest())
				return err
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
			specSampleSubOrder(1500000),
		},
		ExtendInfo: specSampleRequestExtendInfo(),
		Sessionid:  "session-spec",
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
			specSampleSubOrder(1500000),
		},
		ExtendInfo: specSampleRequestExtendInfo(),
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
		ExternalReferenceUID: "u-spec-1",
		TotalAmount:          1500000,
	}
}

func specSamplePaymentPlanRequest() *payment.PaymentPlanRequest {
	return &payment.PaymentPlanRequest{
		RequestID:            "r-spec-plan",
		ExternalReferenceUID: "u-spec-1",
		TotalAmount:          1500000,
		SubOrders: []payment.PlanSubOrder{
			specSamplePlanSubOrder(1500000),
		},
		ExtendInfo: &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
		Sessionid:  "session-spec",
	}
}

func specSampleRiplayRequest() *payment.RiplayRequest {
	return &payment.RiplayRequest{
		SessionID:            "SES-spec-1",
		ExternalReferenceUID: "u-spec-1",
		Tenor:                3,
	}
}
