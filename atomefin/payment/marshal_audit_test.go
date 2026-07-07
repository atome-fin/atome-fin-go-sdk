// Package payment_test wires the qa/marshal harness against every
// public request/response struct in the payment package. This is the
// single point where the type-parametric harness gets bound to
// concrete types.
//
// Each fixture under qa/testdata/ is exercised by a GoldenRoundTrip
// or StrictDecode call. New fixtures land here as soon as they are
// committed so QA's harness lights up automatically.
package payment_test

import (
	"fmt"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /auth ----------

func TestAuthRequest_Roundtrip_Minimal(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthRequest](t, fixtureRoot+"auth_request.json")
}

func TestAuthRequest_Roundtrip_WithExtend(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthRequest](t, fixtureRoot+"auth_request_with_extend.json")
}

func TestAuthResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"auth_response_success.json")
}

func TestAuthResponse_Roundtrip_Processing(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"auth_response_processing.json")
}

func TestAuthResponse_Roundtrip_FailedCredit(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"auth_response_failed_credit.json")
}

func TestAuthResponse_Roundtrip_FailedRisk(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"auth_response_failed_risk.json")
}

func TestAuthResponse_Roundtrip_AccountChange(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"auth_response_account_change.json")
}

// ---------- /capture ----------

func TestCaptureRequest_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.CaptureRequest](t, fixtureRoot+"capture_request.json")
}

func TestCaptureResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[payment.CaptureResponse](t, fixtureRoot+"capture_response_success.json")
}

func TestCaptureResponse_Roundtrip_Processing(t *testing.T) {
	marshal.GoldenRoundTrip[payment.CaptureResponse](t, fixtureRoot+"capture_response_processing.json")
}

func TestCaptureResponse_Roundtrip_CreditInsufficient(t *testing.T) {
	// Sync 200 with business-code USER_CREDIT_LIMIT_INSUFFICIENT and
	// no `data` envelope.
	marshal.GoldenRoundTrip[payment.CaptureResponse](t, fixtureRoot+"capture_response_credit_insufficient.json")
}

// ---------- /voidAuth ----------

func TestVoidAuthRequest_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.VoidAuthRequest](t, fixtureRoot+"voidauth_request.json")
}

func TestVoidAuthResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[payment.VoidAuthResponse](t, fixtureRoot+"voidauth_response_success.json")
}

func TestVoidAuthResponse_Roundtrip_AuthExpired(t *testing.T) {
	marshal.GoldenRoundTrip[payment.VoidAuthResponse](t, fixtureRoot+"voidauth_response_auth_expired.json")
}

// ---------- Callback bodies (same JSON shape as Auth/Capture responses) ----------

func TestCallback_Auth_Terminal_Success(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"callback_auth_terminal_success.json")
}

func TestCallback_Auth_Terminal_Failed(t *testing.T) {
	marshal.GoldenRoundTrip[payment.AuthResponse](t, fixtureRoot+"callback_auth_terminal_failed.json")
}

func TestCallback_Capture_Terminal_Success(t *testing.T) {
	marshal.GoldenRoundTrip[payment.CaptureResponse](t, fixtureRoot+"callback_capture_terminal_success.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_AuthRequest_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.AuthRequest](t, func(v int64) payment.AuthRequest {
		// SubOrder.amount must equal totalAmount; build a single-sub
		// order whose Amount echoes v. Skip values that the validator
		// would reject — R10 is about the codec, not the validator.
		return payment.AuthRequest{
			RequestID:            "r-1",
			ExternalReferenceUID: "u-1",
			TotalAmount:          v,
			PeriodType:           1,
			SubOrders: []payment.SubOrder{
				specSampleSubOrder(v),
			},
		}
	})
}

func TestR10_SubOrder_OriginalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.SubOrder](t, func(v int64) payment.SubOrder {
		return payment.SubOrder{
			SubOrderID:     "so-1",
			SkuID:          "sku-1",
			CategoryID:     "cat-1",
			CategoryOneName: "Food",
			MerchantID:     "merchant-1",
			Amount:         1,
			Quantity:       1,
			OriginalAmount: v,
		}
	})
}

func TestR10_AccountChanges_Deltas(t *testing.T) {
	// Exercises the full set of *Change deltas including negatives.
	marshal.AssertAmountRoundtrip[payment.AccountChanges](t, func(v int64) payment.AccountChanges {
		return payment.AccountChanges{
			Version:               1746489600000,
			ExternalReferenceUID:  "u-1",
			PreviousStatus:        "NORMAL",
			CurrentStatus:         "NORMAL",
			TotalCreditChange:     v,
			UsedCreditChange:      v,
			FrozenCreditChange:    v,
			AvailableCreditChange: v,
			OverpaidAmountChange:  v,
			LateFeeAmountChange:   v,
			InterestAmountChange:  v,
		}
	})
}

// ---------- R11 — fractional decode of an amount field fails loudly ----------

func TestR11_RejectsFractionalOriginalAmount(t *testing.T) {
	body := []byte(`{"subOrderId":"so-1","amount":1,"quantity":1,"originalAmount":1.5}`)
	marshal.AssertRejectsFractionalAmount[payment.SubOrder](t, body)
}

func TestR11_RejectsFractionalTotalAmount(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","totalAmount":1.5,"periodType":1,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.AuthRequest](t, body)
}

func TestR11_RejectsScientificAmount(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","totalAmount":1e10,"periodType":1,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.AuthRequest](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_AuthRequest_IntegerLiterals(t *testing.T) {
	in := payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders: []payment.SubOrder{
			specSampleSubOrder(1000000),
			func() payment.SubOrder {
				so := specSampleSubOrder(500000)
				so.SubOrderID = "so-2"
				so.Quantity = 2
				so.OriginalAmount = 1100000
				return so
			}(),
		},
	}
	marshal.AssertAmountKeysAreInteger[payment.AuthRequest](t, in,
		"totalAmount", "amount", "originalAmount",
	)
}

func TestR12_AccountChanges_IntegerLiterals(t *testing.T) {
	in := payment.AccountChanges{
		Version:               1746489600000,
		ExternalReferenceUID:  "u",
		PreviousStatus:        "NORMAL",
		CurrentStatus:         "ACCOUNT_CLOSED",
		TotalCreditChange:     1,
		UsedCreditChange:      -1,
		FrozenCreditChange:    0,
		AvailableCreditChange: 999999,
		OverpaidAmountChange:  0,
		LateFeeAmountChange:   0,
		InterestAmountChange:  0,
	}
	marshal.AssertAmountKeysAreInteger[payment.AccountChanges](t, in,
		"totalCreditChange", "usedCreditChange", "frozenCreditChange",
		"availableCreditChange", "overpaidAmountChange",
		"lateFeeAmountChange", "interestAmountChange",
	)
}

// ---------- R3/R4 — omitempty / required-emit on AuthRequest ----------

func TestR3_AuthRequest_OmitsExtendInfoAtZero(t *testing.T) {
	marshal.AssertOmitemptyZero[payment.AuthRequest](t, "extendInfo")
}

func TestR4_AuthRequest_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[payment.AuthRequest](t,
		"requestId", "externalReferenceUid", "totalAmount", "periodType",
	)
}

// ---------- Position-scoped enum ----------

func TestPositionScoped_PreviousStatusRejectsClosed(t *testing.T) {
	if payment.IsValidPreviousStatus("ACCOUNT_CLOSED") {
		t.Error("ACCOUNT_CLOSED must NOT be valid for previousStatus (position-scoped)")
	}
}

func TestPositionScoped_CurrentStatusAcceptsClosed(t *testing.T) {
	if !payment.IsValidCurrentStatus("ACCOUNT_CLOSED") {
		t.Error("ACCOUNT_CLOSED must be valid for currentStatus")
	}
}

// ---------- userCreditScore validity ----------

func TestUserCreditScore_RangeCheck(t *testing.T) {
	in := func(v float64) *float64 { return &v }
	cases := []struct {
		s    *float64
		want bool
	}{
		{nil, true},
		{in(0), true},
		{in(0.5), true},
		{in(1), true},
		{in(-0.01), false},
		{in(1.0001), false},
	}
	for _, c := range cases {
		if got := payment.IsValidScore(c.s); got != c.want {
			label := "<nil>"
			if c.s != nil {
				label = fmt.Sprintf("%g", *c.s)
			}
			t.Errorf("IsValidScore(%s) = %v, want %v", label, got, c.want)
		}
	}
}
