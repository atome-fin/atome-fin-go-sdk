package atomefin

import "testing"

func TestStatusIsTerminal(t *testing.T) {
	tests := []struct {
		s    Status
		want bool
	}{
		{StatusProcessing, false},
		{StatusSuccess, true},
		{StatusFailed, true},
		{Status(""), false},
		{Status("unknown"), false},
	}
	for _, tt := range tests {
		if got := tt.s.IsTerminal(); got != tt.want {
			t.Errorf("Status(%q).IsTerminal() = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestCodeIsSuccess(t *testing.T) {
	if !CodeSuccess.IsSuccess() {
		t.Error("CodeSuccess.IsSuccess() = false, want true")
	}
	if CodeServerError.IsSuccess() {
		t.Error("CodeServerError.IsSuccess() = true, want false")
	}
	if Code("").IsSuccess() {
		t.Error("empty code IsSuccess() = true, want false")
	}
}

// Spec-fidelity check: every constant must equal its spec literal byte for
// byte. This is the minimum bar to keep the spec field tables truthful.
func TestStringLiteralsMatchSpec(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{string(StatusProcessing), "PROCESSING"},
		{string(StatusSuccess), "SUCCESS"},
		{string(StatusFailed), "FAILED"},

		{string(CodeSuccess), "SUCCESS"},
		{string(CodeInvalidOrderAmount), "INVALID_ORDER_AMOUNT"},
		{string(CodeCreditApplicationNotApproved), "CREDIT_APPLICATION_NOT_APPROVED"},
		{string(CodeAccountBlockedOverdue), "ACCOUNT_BLOCKED_OVERDUE"},
		{string(CodeAccountBlocked), "ACCOUNT_BLOCKED"},
		{string(CodeAccountTempBlocked), "ACCOUNT_TEMP_BLOCKED"},
		{string(CodeFeeChange), "FEE_CHANGE"},
		{string(CodeUserCreditLimitInsufficient), "USER_CREDIT_LIMIT_INSUFFICIENT"},
		{string(CodeParamsMissing), "PARAMS_MISSING"},
		{string(CodeWrongParamsFormat), "WRONG_PARAMS_FORMAT"},
		{string(CodeParamsWrong), "PARAMS_WRONG"},
		{string(CodeNotFound), "NOT_FOUND"},
		{string(CodeSessionNotFound), "SESSION_NOT_FOUND"},
		{string(CodeCaptureAmountExceed), "CAPTURE_AMOUNT_EXCEED"},
		{string(CodeAuthExpired), "AUTH_EXPIRED"},
		{string(CodeInvalidSignature), "INVALID_SIGNATURE"},
		{string(CodeServerError), "SERVER_ERROR"},

		{string(FailureUserCreditLimitInsufficient), "USER_CREDIT_LIMIT_INSUFFICIENT"},
		{string(FailureRiskReject), "RISK_REJECT"},

		{string(AccountStatusNormal), "NORMAL"},
		{string(AccountStatusBlockedOverdue), "ACCOUNT_BLOCKED_OVERDUE"},
		{string(AccountStatusBlocked), "ACCOUNT_BLOCKED"},
		{string(AccountStatusClosed), "ACCOUNT_CLOSED"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("constant value = %q, want %q (spec literal)", c.got, c.want)
		}
	}
}
