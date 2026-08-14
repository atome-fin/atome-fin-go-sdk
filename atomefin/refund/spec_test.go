package refund_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_RefundEndpoints drives every refund-package outbound
// method against the spec server. v0.2.3 closes the rename gap
// surfaced by milestone 2 of v0.2.2 — the SDK now sends
// `captureRequestId` / `subOrders` / `subOrders[].amount` matching
// the 2026-05-06 spec snapshot. No SkipRequired entries needed.
func TestSpec_RefundEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /refund",
			Run: func(c *atomefin.Client) error {
				_, err := refund.New(c).Refund(context.Background(), specSampleRefundParam())
				return err
			},
		},
		{
			Op: "GET /query-refund",
			Run: func(c *atomefin.Client) error {
				_, err := refund.New(c).QueryRefund(context.Background(), "r-spec-1", "u-spec-1")
				return err
			},
		},
	})
}

func specSampleRefundParam() *refund.RefundParam {
	return &refund.RefundParam{
		RequestID:            "r-spec-refund",
		ExternalReferenceUID: "u-spec-1",
		CaptureRequestID:     "CAP-SPEC-1",
		RefundAmount:         500000,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 500000},
		},
		ExtendInfo: &refund.RefundExtendInfo{OrderType: "GRAB_FOOD"},
	}
}
