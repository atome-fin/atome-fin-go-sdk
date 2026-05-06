// Wires the qa/marshal harness against every public repayment struct.
// Mirrors atomefin/refund/marshal_audit_test.go.
package repayment_test

import (
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /repayment-request + /repayment-result + callback ----------

func TestRepaymentParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[repayment.RepaymentParam](t, fixtureRoot+"repayment_request.json")
}

func TestRepaymentResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[repayment.RepaymentResponse](t, fixtureRoot+"repayment_response_success.json")
}

func TestRepaymentResponse_Roundtrip_Processing(t *testing.T) {
	marshal.GoldenRoundTrip[repayment.RepaymentResponse](t, fixtureRoot+"repayment_response_processing.json")
}

func TestQueryRepaymentResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[repayment.RepaymentResponse](t, fixtureRoot+"query_repayment_response.json")
}

func TestCallback_Repayment_Terminal_Success(t *testing.T) {
	// Callback body shape == /repayment-request response; same RepaymentResponse type.
	marshal.GoldenRoundTrip[repayment.RepaymentResponse](t, fixtureRoot+"callback_repayment_terminal_success.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_RepaymentParam_RepaymentAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.RepaymentParam](t, func(v int64) repayment.RepaymentParam {
		return repayment.RepaymentParam{
			RequestID:            "r-1",
			ExternalReferenceUID: "u-1",
			RepaymentAmount:      v,
			RepaymentApplyTime:   1746662400000,
		}
	})
}

func TestR10_RepaymentResult_RepaymentAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.RepaymentResult](t, func(v int64) repayment.RepaymentResult {
		return repayment.RepaymentResult{
			RequestID:       "r-1",
			RepaymentID:     "RPM-1",
			Status:          "SUCCESS",
			Currency:        "IDR",
			RepaymentAmount: v,
		}
	})
}

func TestR10_CommerceAccountChanges_TotalCreditChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID: "u-1",
			TotalCreditChange:    v,
			Version:              1746662400000,
		}
	})
}

func TestR10_CommerceAccountChanges_UsedCreditChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID: "u-1",
			UsedCreditChange:     v,
			Version:              1746662400000,
		}
	})
}

func TestR10_CommerceAccountChanges_AvailableCreditChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID:  "u-1",
			AvailableCreditChange: v,
			Version:               1746662400000,
		}
	})
}

func TestR10_CommerceAccountChanges_OverpaidAmountChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID: "u-1",
			OverpaidAmountChange: v,
			Version:              1746662400000,
		}
	})
}

func TestR10_CommerceAccountChanges_LateFeeAmountChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID: "u-1",
			LateFeeAmountChange:  v,
			Version:              1746662400000,
		}
	})
}

func TestR10_CommerceAccountChanges_InterestAmountChange(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.CommerceAccountChanges](t, func(v int64) repayment.CommerceAccountChanges {
		return repayment.CommerceAccountChanges{
			ExternalReferenceUID: "u-1",
			InterestAmountChange: v,
			Version:              1746662400000,
		}
	})
}

func TestR10_RepaymentSettlement_PayableSubsidyAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[repayment.RepaymentSettlement](t, func(v int64) repayment.RepaymentSettlement {
		return repayment.RepaymentSettlement{PayableSubsidyAmount: v}
	})
}

// ---------- R11 — fractional decode of an amount field fails loudly ----------

func TestR11_RejectsFractionalRepaymentAmount(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","repaymentAmount":1.5,"repaymentApplyTime":1746662400000}`)
	marshal.AssertRejectsFractionalAmount[repayment.RepaymentParam](t, body)
}

func TestR11_RejectsFractionalCommerceAccountChange(t *testing.T) {
	body := []byte(`{"externalReferenceUid":"u","totalCreditChange":1.5,"version":1746662400000}`)
	marshal.AssertRejectsFractionalAmount[repayment.CommerceAccountChanges](t, body)
}

func TestR11_RejectsFractionalSettlement(t *testing.T) {
	body := []byte(`{"payableSubsidyAmount":1.5}`)
	marshal.AssertRejectsFractionalAmount[repayment.RepaymentSettlement](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_RepaymentParam_IntegerLiterals(t *testing.T) {
	in := repayment.RepaymentParam{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		RepaymentAmount:      1500000,
		RepaymentApplyTime:   1746662400000,
	}
	marshal.AssertAmountKeysAreInteger[repayment.RepaymentParam](t, in,
		"repaymentAmount",
	)
}

func TestR12_RepaymentResult_IntegerLiterals(t *testing.T) {
	in := repayment.RepaymentResult{
		RequestID:       "r",
		RepaymentID:     "RPM-1",
		Status:          "SUCCESS",
		Currency:        "IDR",
		RepaymentAmount: 1500000,
		AccountChanges: &repayment.CommerceAccountChanges{
			ExternalReferenceUID:  "u",
			TotalCreditChange:     0,
			UsedCreditChange:      -1500000,
			AvailableCreditChange: 1500000,
			OverpaidAmountChange:  0,
			LateFeeAmountChange:   0,
			InterestAmountChange:  0,
			Version:               1746662400000,
		},
		ExtendInfo: &repayment.RepaymentExtendInfo{
			Settlement: &repayment.RepaymentSettlement{
				PayableSubsidyAmount: 0,
			},
		},
	}
	marshal.AssertAmountKeysAreInteger[repayment.RepaymentResult](t, in,
		"repaymentAmount",
		"totalCreditChange", "usedCreditChange",
		"availableCreditChange", "overpaidAmountChange",
		"lateFeeAmountChange", "interestAmountChange",
		"payableSubsidyAmount",
	)
}

// ---------- R3/R4 — omitempty / required-emit on RepaymentParam ----------

func TestR3_RepaymentParam_OmitsExtendInfoWhenZero(t *testing.T) {
	marshal.AssertOmitemptyZero[repayment.RepaymentParam](t, "extendInfo")
}

func TestR4_RepaymentParam_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[repayment.RepaymentParam](t,
		"requestId", "externalReferenceUid",
		"repaymentAmount", "repaymentApplyTime",
	)
}

func TestR4_RepaymentResult_RequiredEmitsAtZero(t *testing.T) {
	// Required by spec: requestId, repaymentId, status, currency.
	marshal.AssertRequiredEmits[repayment.RepaymentResult](t,
		"requestId", "repaymentId", "status", "currency",
	)
}

func TestR3_RepaymentResult_OmitsOptionalsWhenZero(t *testing.T) {
	marshal.AssertOmitemptyZero[repayment.RepaymentResult](t,
		"repaymentAmount", "repaymentTime", "event",
		"accountChanges", "extendInfo",
	)
}
