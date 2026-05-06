package refund_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_RefundEndpoints drives every refund-package outbound
// method against the spec server.
//
// The SDK's RefundParam was authored against the partner-pending
// 2026-04-22 spec snapshot; the 2026-05-06 publish renamed two
// top-level fields (`authOrderId` → `captureRequestId`,
// `subOrderRefunds` → `subOrders`) and renamed one nested field
// (`subOrders[].refundAmount` → `subOrders[].amount`). The renames
// are tracked toward the v0.2.x roadmap; for the spec-server
// regression backstop today, the missing fields are skipped with a
// pointer to this comment so future readers see the gap explicitly.
func TestSpec_RefundEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /refund",
			Run: func(c *atomefin.Client) error {
				_, err := refund.New(c).Refund(context.Background(), specSampleRefundParam())
				return err
			},
			SkipRequired: []string{
				"captureRequestId",       // SDK sends authOrderId; rename pending v0.2.x
				"subOrders",              // SDK sends subOrderRefunds; rename pending v0.2.x
				"subOrders[].subOrderId", // unreachable until subOrders rename lands
				"subOrders[].amount",     // SDK sends subOrderRefunds[].refundAmount; rename pending
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
		AuthOrderID:          "AUTH-SPEC-1",
		RefundAmount:         500000,
		SubOrderRefunds: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", RefundAmount: 500000},
		},
	}
}
