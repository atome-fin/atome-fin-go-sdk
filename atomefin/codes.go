package atomefin

// This file hosts the enum-like string types shared across the outbound
// payment service (atomefin/payment) and the inbound callback receivers
// (atomefin/callback). Both directions exchange the same envelope shapes
// (`*AckResponse` and the various error envelopes — DESIGN.md §1.6, §5, §8),
// so the typed enums live one level above both.
//
// All values are taken verbatim from the Partner Order Auth-Capture API v1
// (Draft) spec at https://doc.apaylater.net/white-label/G/. The string
// literals are wire-stable; do not lower-case them or rename the constants
// without bumping the SDK major version.

// Status is the `data.status` field on every successful 200 response and on
// every callback body. It is the only field that distinguishes "still
// processing" from a terminal outcome.
type Status string

// Status values defined by the spec. PROCESSING never appears in callbacks —
// callbacks are fired only at terminal states (DESIGN.md §1.4).
const (
	StatusProcessing Status = "PROCESSING"
	StatusSuccess    Status = "SUCCESS"
	StatusFailed     Status = "FAILED"
)

// IsTerminal reports whether the status is SUCCESS or FAILED. PROCESSING
// callers can keep polling (see payment.PollUntilTerminal in T3).
func (s Status) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed
}

// Code is the business-level outcome code echoed inside the 200 envelope's
// top-level `code` field. Note that `USER_CREDIT_LIMIT_INSUFFICIENT` may
// surface BOTH here (capture-time pre-flight rejection) and as a FailureCode
// nested in `data.failureCode` (terminal FAILED outcome) — see DESIGN.md
// §1.6. The string literal is shared; the typed Go constants are not.
type Code string

// Spec-defined business codes that may surface in HTTP 200 envelopes.
const (
	CodeSuccess                      Code = "SUCCESS"
	CodeInvalidOrderAmount           Code = "INVALID_ORDER_AMOUNT"
	CodeCreditApplicationNotApproved Code = "CREDIT_APPLICATION_NOT_APPROVED"
	CodeAccountBlockedOverdue        Code = "ACCOUNT_BLOCKED_OVERDUE"
	CodeAccountBlocked               Code = "ACCOUNT_BLOCKED"
	CodeAccountTempBlocked           Code = "ACCOUNT_TEMP_BLOCKED"
	CodeFeeChange                    Code = "FEE_CHANGE"
	CodeUserCreditLimitInsufficient  Code = "USER_CREDIT_LIMIT_INSUFFICIENT"
	CodeRiskReject                   Code = "RISK_REJECT"

	// Codes that surface in non-200 envelopes; included here so a caller can
	// type-switch a single APIError.Code field without juggling multiple
	// enum types. See DESIGN.md §1.6.
	CodeParamsMissing       Code = "PARAMS_MISSING"
	CodeWrongParamsFormat   Code = "WRONG_PARAMS_FORMAT"
	CodeParamsWrong         Code = "PARAMS_WRONG"
	CodeNotFound            Code = "NOT_FOUND"
	CodeSessionNotFound     Code = "SESSION_NOT_FOUND"
	CodeCaptureAmountExceed Code = "CAPTURE_AMOUNT_EXCEED"
	CodeAuthExpired         Code = "AUTH_EXPIRED"
	CodeInvalidSignature    Code = "INVALID_SIGNATURE"
	CodeServerError         Code = "SERVER_ERROR"
)

// IsSuccess reports whether the code is the canonical 200/SUCCESS value.
// Useful as a quick guard around envelope decoding without touching status.
func (c Code) IsSuccess() bool { return c == CodeSuccess }

// FailureCode is the optional `data.failureCode` field set when `Status` is
// FAILED. Distinct from Code despite sharing string literals with some Code
// values — semantics differ (FailureCode describes *why* a terminal-FAILED
// outcome failed; Code describes *what kind* of envelope this is).
type FailureCode string

// Spec-defined failure codes (DESIGN.md §1.6).
const (
	FailureUserCreditLimitInsufficient FailureCode = "USER_CREDIT_LIMIT_INSUFFICIENT"
	FailureRiskReject                  FailureCode = "RISK_REJECT"
)

// AccountStatus is the value of `accountChanges.previousStatus` /
// `accountChanges.currentStatus` inside auth/capture response data. Its
// enum is closed in the spec.
type AccountStatus string

// Spec-defined account statuses.
const (
	AccountStatusNormal         AccountStatus = "NORMAL"
	AccountStatusBlockedOverdue AccountStatus = "ACCOUNT_BLOCKED_OVERDUE"
	AccountStatusBlocked        AccountStatus = "ACCOUNT_BLOCKED"
	AccountStatusClosed         AccountStatus = "ACCOUNT_CLOSED"
)
