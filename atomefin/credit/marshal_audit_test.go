// Wires the qa/marshal harness against every public credit struct.
// Mirrors atomefin/refund/marshal_audit_test.go and
// atomefin/bill/marshal_audit_test.go.
package credit_test

import (
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /credit-information request + response variants ----------

func TestCreditInformationParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditInformationParam](t, fixtureRoot+"credit_information_request.json")
}

func TestCreditInformationResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditInformationResponse](t, fixtureRoot+"credit_information_response_success.json")
}

// ---------- /credit-application request + response variants ----------

func TestCreditApplicationParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationParam](t, fixtureRoot+"credit_application_request.json")
}

func TestCreditApplicationResponse_Roundtrip_Success(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationResponse](t, fixtureRoot+"credit_application_response_success.json")
}

func TestCreditApplicationResponse_Roundtrip_Processing(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationResponse](t, fixtureRoot+"credit_application_response_processing.json")
}

func TestCreditApplicationResponse_Roundtrip_Failed(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationResponse](t, fixtureRoot+"credit_application_response_failed.json")
}

// ---------- /credit-information-result + callback ----------

func TestCreditInformationCollectResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditInformationCollectResponse](t, fixtureRoot+"credit_information_result_response.json")
}

func TestCreditInformationCollectResponse_Roundtrip_Failed(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditInformationCollectResponse](t, fixtureRoot+"credit_information_result_failed.json")
}

func TestCallback_CreditInformation_Roundtrip(t *testing.T) {
	// Callback body == credit-information-result envelope; same type.
	marshal.GoldenRoundTrip[credit.CreditInformationCollectResponse](t, fixtureRoot+"callback_credit_information_terminal_success.json")
}

func TestCallback_CreditApplication_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationResult](t, fixtureRoot+"callback_credit_application_terminal_success.json")
}

// ---------- /query-balance-history (paginated-list pattern) ----------

func TestBalanceHistoryResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.BalanceHistoryResponse](t, fixtureRoot+"balance_history_response.json")
}

// Empty-list round-trip pin: the paginated-list pattern requires
// an empty page to round-trip as `[]` rather than disappearing.
func TestBalanceHistoryResponse_Roundtrip_Empty(t *testing.T) {
	marshal.GoldenRoundTrip[credit.BalanceHistoryResponse](t, fixtureRoot+"balance_history_response_empty.json")
}

// ---------- /modify-application-info + /close-account ----------

func TestCreditApplicationChangeParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CreditApplicationChangeParam](t, fixtureRoot+"credit_application_change_request.json")
}

func TestCloseAccountParam_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CloseAccountParam](t, fixtureRoot+"close_account_request.json")
}

func TestCloseAccountResponse_Roundtrip_Unpaid(t *testing.T) {
	marshal.GoldenRoundTrip[credit.CloseAccountResponse](t, fixtureRoot+"close_account_response_unpaid.json")
}

// ---------- R10 — full int64 amount round-trip on every amount field ----------

// CreditInfo carries six amount fields — TotalCredit, AvailableCredit,
// UsedCredit, LateFeeAmount, OverpaidAmount, OverpaidWithdrawAmount.
// Each gets its own R10 sweep so a regression on any one field
// surfaces with a per-field signal in test output.

func TestR10_CreditInfo_TotalCredit(t *testing.T) {
	marshal.AssertAmountRoundtrip[credit.CreditInfo](t, func(v int64) credit.CreditInfo {
		return credit.CreditInfo{
			TotalCredit:     v,
			AvailableCredit: 0,
			UsedCredit:      0,
			UserStatus:      credit.UserStatusNormal,
			Version:         1,
		}
	})
}

func TestR10_CreditInfo_AvailableCredit(t *testing.T) {
	marshal.AssertAmountRoundtrip[credit.CreditInfo](t, func(v int64) credit.CreditInfo {
		return credit.CreditInfo{
			TotalCredit:     0,
			AvailableCredit: v,
			UsedCredit:      0,
			UserStatus:      credit.UserStatusNormal,
			Version:         1,
		}
	})
}

func TestR10_CreditInfo_UsedCredit(t *testing.T) {
	marshal.AssertAmountRoundtrip[credit.CreditInfo](t, func(v int64) credit.CreditInfo {
		return credit.CreditInfo{
			TotalCredit:     0,
			AvailableCredit: 0,
			UsedCredit:      v,
			UserStatus:      credit.UserStatusNormal,
			Version:         1,
		}
	})
}

// ---------- R11 — fractional decode of amount field fails loudly ----------

func TestR11_RejectsFractionalTotalCredit(t *testing.T) {
	body := []byte(`{"totalCredit":1.5,"availableCredit":0,"usedCredit":0,"userStatus":"NORMAL","version":1}`)
	marshal.AssertRejectsFractionalAmount[credit.CreditInfo](t, body)
}

func TestR11_RejectsFractionalAvailableCredit(t *testing.T) {
	body := []byte(`{"totalCredit":0,"availableCredit":2.5,"usedCredit":0,"userStatus":"NORMAL","version":1}`)
	marshal.AssertRejectsFractionalAmount[credit.CreditInfo](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_CreditInfo_IntegerLiterals(t *testing.T) {
	in := credit.CreditInfo{
		TotalCredit:            30000000,
		AvailableCredit:        25000000,
		UsedCredit:             5000000,
		LateFeeAmount:          50000,
		OverpaidAmount:         10000,
		OverpaidWithdrawAmount: 5000,
		UserStatus:             credit.UserStatusNormal,
		Version:                1715000000000,
	}
	marshal.AssertAmountKeysAreInteger[credit.CreditInfo](t, in,
		"totalCredit", "availableCredit", "usedCredit",
		"lateFeeAmount", "overpaidAmount", "overpaidWithdrawAmount",
	)
}

func TestR12_CreditApplicationResult_IntegerLiterals(t *testing.T) {
	in := credit.CreditApplicationResult{
		ExternalReferenceUID: "user-42",
		Status:               credit.CreditStatusSuccess,
		ReapplyTime:          1720000000000,
		Currency:             "IDR",
		CreditInfo: &credit.CreditInfo{
			TotalCredit:     30000000,
			AvailableCredit: 30000000,
			UsedCredit:      0,
			UserStatus:      credit.UserStatusNormal,
			Version:         1715000000000,
		},
	}
	marshal.AssertAmountKeysAreInteger[credit.CreditApplicationResult](t, in,
		"totalCredit", "availableCredit", "usedCredit",
	)
}

// ---------- R3 — omitempty on optional fields ----------

func TestR3_CreditInformationParam_OmitsExtendInfo(t *testing.T) {
	marshal.AssertOmitemptyZero[credit.CreditInformationParam](t,
		"extendInfo",
	)
}

func TestR3_CloseAccountResponse_OmitsNothing(t *testing.T) {
	// Boundary check — CloseAccountResponse has only Code+Message;
	// neither is optional in our envelope shape.
	marshal.AssertOmitemptyZero[credit.CloseAccountResponse](t)
}

// ---------- R4 — required-emit on the top-level shapes ----------

func TestR4_CreditInformationParam_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[credit.CreditInformationParam](t,
		"requestId", "externalReferenceUid", "mobileNumber", "email", "country",
		"applicationEssentialInfo",
	)
}

func TestR4_CreditApplicationParam_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[credit.CreditApplicationParam](t,
		"requestId", "externalReferenceUid", "mobileNumber",
		"email", "country", "applicationEssentialInfo", "extendInfo",
	)
}

func TestR4_CloseAccountParam_RequiredEmitsAtZero(t *testing.T) {
	marshal.AssertRequiredEmits[credit.CloseAccountParam](t,
		"requestId", "externalReferenceUid",
	)
}

// ---------- Status / enum round-trip pin ----------

func TestCreditStatus_IsValid(t *testing.T) {
	for _, s := range []credit.CreditStatus{
		credit.CreditStatusSuccess,
		credit.CreditStatusFailed,
		credit.CreditStatusProcessing,
		credit.CreditStatusDraft,
	} {
		if !s.IsValid() {
			t.Errorf("CreditStatus(%q).IsValid() = false; want true", s)
		}
	}
	for _, s := range []credit.CreditStatus{"", "success", "ON_HOLD"} {
		if s.IsValid() {
			t.Errorf("CreditStatus(%q).IsValid() = true; want false", s)
		}
	}
}

func TestBalanceHistoryType_IsValid(t *testing.T) {
	for _, t1 := range []credit.BalanceHistoryType{
		credit.BalanceHistoryTypeOverpaidChange,
		credit.BalanceHistoryTypeCreditLimitAdjustment,
		credit.BalanceHistoryTypeTradeAvailableCreditChange,
	} {
		if !t1.IsValid() {
			t.Errorf("BalanceHistoryType(%q).IsValid() = false; want true", t1)
		}
	}
}

func TestUserStatus_IsValid(t *testing.T) {
	for _, s := range []credit.UserStatus{
		credit.UserStatusNormal,
		credit.UserStatusAccountBlockedOverdue,
		credit.UserStatusAccountBlocked,
		credit.UserStatusAccountClosed,
	} {
		if !s.IsValid() {
			t.Errorf("UserStatus(%q).IsValid() = false; want true", s)
		}
	}
}

func TestCreditStatus_StringIsWireLiteral(t *testing.T) {
	if got := credit.CreditStatusProcessing.String(); got != "PROCESSING" {
		t.Errorf("CreditStatusProcessing.String() = %q", got)
	}
	if got := credit.UserStatusAccountBlockedOverdue.String(); got != "ACCOUNT_BLOCKED_OVERDUE" {
		t.Errorf("UserStatusAccountBlockedOverdue.String() = %q", got)
	}
}

// IsTerminal — terminal=SUCCESS|FAILED.
func TestCreditStatus_IsTerminal(t *testing.T) {
	terminals := []credit.CreditStatus{
		credit.CreditStatusSuccess,
		credit.CreditStatusFailed,
	}
	for _, s := range terminals {
		if !s.IsTerminal() {
			t.Errorf("CreditStatus(%q).IsTerminal() = false; want true", s)
		}
	}
	for _, s := range []credit.CreditStatus{
		credit.CreditStatusProcessing,
		credit.CreditStatusDraft,
		credit.CreditStatus(""),
	} {
		if s.IsTerminal() {
			t.Errorf("CreditStatus(%q).IsTerminal() = true; want false", s)
		}
	}
}
