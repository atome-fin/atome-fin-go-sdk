package repayment

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// RepaymentParam is the POST /repayment-request body. v0.2.
//
// The shape is the partner's "apply to repay" envelope: requestId is
// the partner's idempotency key; externalReferenceUid identifies the
// end user; repaymentAmount is the total in minor units;
// repaymentApplyTime is the partner-side intent timestamp (Unix-ms;
// TZ unspecified per Q11). ExtendInfo is the spec's flexible per-
// channel extension blob — left as map[string]any so partners can pass
// channel-specific fields without an SDK release.
type RepaymentParam struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's stable end-user
	// identifier; matches the value used in upstream /auth + /capture.
	ExternalReferenceUID string `json:"externalReferenceUid"` // max 64
	// RepaymentAmount is the total repayment value in minor units
	// (IDR rupiah).
	RepaymentAmount atomefin.Amount `json:"repaymentAmount"`
	// RepaymentApplyTime is when the partner asked atome-fin to
	// repay, in Unix-ms. TZ unspecified — Q11 carry-over.
	RepaymentApplyTime int64 `json:"repaymentApplyTime"`
	// ExtendInfo carries channel-specific extension fields. Spec
	// declares it as a free-form object; the SDK preserves whatever
	// the partner passes through. Optional.
	ExtendInfo map[string]any `json:"extendInfo,omitempty"`
}

// RepaymentResponse is the POST /repayment-request (and
// /repayment-result) outer envelope. Same Code / Message / Data shape
// as the payment / refund responses; the inner data type is
// RepaymentResult.
//
// The /repayment-callback POST body is interpreted through this same
// type — atome-fin posts the response envelope (with code+message
// wrapping the terminal RepaymentResult) so partners that already
// decode RepaymentResponse for the synchronous query don't learn a
// parallel callback shape. (Mirrors RefundResponse / RefundEvent =
// callback.RefundResponse.)
type RepaymentResponse struct {
	Code    atomefin.Code    `json:"code"`
	Message string           `json:"message"`
	Data    *RepaymentResult `json:"data,omitempty"`
}

// IsTerminal reports whether the response carries a terminal Status.
// Nil-safe.
func (r *RepaymentResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// IsProcessing reports whether the response is the async PROCESSING
// envelope. Nil-safe.
func (r *RepaymentResponse) IsProcessing() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status == atomefin.StatusProcessing
}

// RepaymentResult is the `data` body of RepaymentResponse and the
// schema referenced by /repayment-callback (mirrors the spec's
// RepaymentResult component).
//
// Status uses atomefin.Status (PROCESSING / SUCCESS / FAILED). Event
// uses RepaymentEvent (NORMAL / ATOME_REPAYMENT / OVERPAID_REPAYMENT)
// and identifies the channel that originated the repayment.
//
// AccountChanges is the optional commerce-side credit-delta vector.
// It uses CommerceAccountChanges (defined here), the canonical
// credit-change vector for commerce-domain responses per spec.
type RepaymentResult struct {
	RequestID       string                  `json:"requestId"`
	RepaymentID     string                  `json:"repaymentId"`
	Status          atomefin.Status         `json:"status"`
	Currency        atomefin.Currency       `json:"currency"`
	RepaymentAmount atomefin.Amount         `json:"repaymentAmount,omitempty"`
	RepaymentTime   int64                   `json:"repaymentTime,omitempty"` // Unix-ms
	Event           RepaymentEvent          `json:"event,omitempty"`
	AccountChanges  *CommerceAccountChanges `json:"accountChanges,omitempty"`
	ExtendInfo      *RepaymentExtendInfo    `json:"extendInfo,omitempty"`
}

// RepaymentExtendInfo is the optional `extendInfo` block on
// RepaymentResult. Currently the spec only enumerates a `settlement`
// sub-object; keep the parent typed so future fields land cleanly.
type RepaymentExtendInfo struct {
	Settlement *RepaymentSettlement `json:"settlement,omitempty"`
}

// RepaymentSettlement carries coupon / subsidy bookkeeping consumed
// by the repayment.
type RepaymentSettlement struct {
	// PayableSubsidyAmount is the coupon amount consumed during this
	// repayment, in minor units. Required by spec when settlement is
	// present.
	PayableSubsidyAmount atomefin.Amount `json:"payableSubsidyAmount"`
}

// CommerceAccountChanges is the canonical credit-change vector for
// commerce-domain responses (repayment, etc.) per spec. It carries
// the field set defined by the spec for commerce-domain endpoints;
// the auth / capture / voidAuth / refund family uses
// payment.AccountChanges, which is its own canonical vector.
//
// All *Change fields are SIGNED int64 deltas — a repayment reduces
// UsedCredit (negative) and increases AvailableCredit (positive).
type CommerceAccountChanges struct {
	ExternalReferenceUID  string                 `json:"externalReferenceUid"`
	PreviousStatus        atomefin.AccountStatus `json:"previousStatus,omitempty"`
	CurrentStatus         atomefin.AccountStatus `json:"currentStatus,omitempty"`
	TotalCreditChange     atomefin.Amount        `json:"totalCreditChange,omitempty"`
	UsedCreditChange      atomefin.Amount        `json:"usedCreditChange,omitempty"`
	AvailableCreditChange atomefin.Amount        `json:"availableCreditChange,omitempty"`
	OverpaidAmountChange  atomefin.Amount        `json:"overpaidAmountChange,omitempty"`
	LateFeeAmountChange   atomefin.Amount        `json:"lateFeeAmountChange,omitempty"`
	InterestAmountChange  atomefin.Amount        `json:"interestAmountChange,omitempty"`
	// Version is a Unix-ms timestamp identifying the revision of the
	// account snapshot this delta refers to.
	Version int64 `json:"version"`
}

// ---------- Enums ----------

// RepaymentEvent is the spec's `event` discriminator on
// RepaymentResult — identifies the channel that originated the
// repayment. The set is closed; unknown values still round-trip
// opaquely (Currency forward-compat pattern).
type RepaymentEvent string

// Spec-defined repayment events.
const (
	// RepaymentEventNormal — partner-initiated through the SDK.
	RepaymentEventNormal RepaymentEvent = "NORMAL"
	// RepaymentEventAtomeRepayment — user repaid through the ATOME
	// channel directly; partner sees it via callback.
	RepaymentEventAtomeRepayment RepaymentEvent = "ATOME_REPAYMENT"
	// RepaymentEventOverpaidRepayment — repayment auto-applied from
	// overpaid balance; triggered by ATOME side.
	RepaymentEventOverpaidRepayment RepaymentEvent = "OVERPAID_REPAYMENT"
)

// IsValid reports whether e is a spec-defined repayment event.
// Decoding NEVER goes through IsValid (forward-compat). Use at the
// validator layer or in business-logic switches.
func (e RepaymentEvent) IsValid() bool {
	switch e {
	case RepaymentEventNormal,
		RepaymentEventAtomeRepayment,
		RepaymentEventOverpaidRepayment:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (e RepaymentEvent) String() string { return string(e) }

// RepaymentStatus is the spec's bill-level repayment lifecycle enum
// (REPAID / UNPAID / PARTIAL_REPAID). It is conceptually the
// "how much of this bill has been repaid" axis, distinct from
// RepaymentResult.Status (which is the per-/repayment-request
// outcome PROCESSING / SUCCESS / FAILED).
//
// The type lives in repayment/ as the primary domain. bill/,
// transaction/, and any future package referencing the bill-level
// status import this without creating a cycle (repayment imports
// nothing from bill/transaction).
type RepaymentStatus string

// Spec-defined bill-level repayment statuses.
const (
	// StatusRepaid — bill has been paid in full.
	StatusRepaid RepaymentStatus = "REPAID"
	// StatusUnpaid — bill is still outstanding.
	StatusUnpaid RepaymentStatus = "UNPAID"
	// StatusPartialRepaid — part of the bill has been paid back.
	StatusPartialRepaid RepaymentStatus = "PARTIAL_REPAID"
)

// IsValid reports whether s is a spec-defined repayment status.
func (s RepaymentStatus) IsValid() bool {
	switch s {
	case StatusRepaid, StatusUnpaid, StatusPartialRepaid:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (s RepaymentStatus) String() string { return string(s) }
