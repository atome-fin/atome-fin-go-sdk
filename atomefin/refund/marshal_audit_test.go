// Wires the qa/marshal harness against every public refund struct.
// Mirrors atomefin/payment/marshal_audit_test.go.
package refund_test

import (
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /refund + /query-refund + callback ----------

func TestRefundParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundParam](t, fixtureRoot+"refund_request.json")
}

func TestRefundResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundResponse](t, fixtureRoot+"refund_response_success.json")
}

func TestRefundResponse_Roundtrip_Processing(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundResponse](t, fixtureRoot+"refund_response_processing.json")
}

func TestRefundResponse_Roundtrip_Failed(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundResponse](t, fixtureRoot+"refund_response_failed.json")
}

func TestQueryRefundResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundResponse](t, fixtureRoot+"query_refund_response_success.json")
}

func TestCallback_Refund_Terminal_Success(t *testing.T) {
	marshal.GoldenRoundTrip[refund.RefundResult](t, fixtureRoot+"callback_refund_terminal_success.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_RefundParam_RefundAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[refund.RefundParam](t, func(v int64) refund.RefundParam {
		// Sub-order sum must equal RefundAmount; build a single-sub
		// case whose RefundAmount echoes v.
		return refund.RefundParam{
			RequestID:            "r-1",
			ExternalReferenceUID: "u-1",
			CaptureRequestID:     "CAP-1",
			RefundAmount:         v,
			SubOrders: []refund.SubOrderRefundRequest{
				{SubOrderID: "so-1", Amount: v},
			},
		}
	})
}

func TestR10_SubOrderRefundRequest_RefundAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[refund.SubOrderRefundRequest](t, func(v int64) refund.SubOrderRefundRequest {
		return refund.SubOrderRefundRequest{SubOrderID: "so-1", Amount: v}
	})
}

// ---------- R11 — fractional decode of an amount field fails loudly ----------

func TestR11_RejectsFractionalRefundAmount(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","captureRequestId":"C","refundAmount":1.5,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[refund.RefundParam](t, body)
}

func TestR11_RejectsFractionalSubOrderAmount(t *testing.T) {
	body := []byte(`{"subOrderId":"so-1","amount":1.5}`)
	marshal.AssertRejectsFractionalAmount[refund.SubOrderRefundRequest](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_RefundParam_IntegerLiterals(t *testing.T) {
	in := refund.RefundParam{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		CaptureRequestID:     "C",
		RefundAmount:         1500000,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 1000000},
			{SubOrderID: "so-2", Amount: 500000},
		},
	}
	marshal.AssertAmountKeysAreInteger[refund.RefundParam](t, in,
		"refundAmount",
	)
}

// ---------- R3/R4 — omitempty / required-emit on RefundParam ----------

func TestR3_RefundParam_OmitsNothingExtra(t *testing.T) {
	// RefundParam currently has no optional fields, so this is a
	// boundary check — ensure the zero value doesn't inadvertently
	// emit a stray key from a future addition.
	marshal.AssertOmitemptyZero[refund.RefundParam](t)
}

func TestR4_RefundParam_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[refund.RefundParam](t,
		"requestId", "externalReferenceUid", "captureRequestId", "refundAmount", "subOrders",
	)
}
