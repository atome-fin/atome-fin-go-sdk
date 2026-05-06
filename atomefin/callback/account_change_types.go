package callback

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// AccountChangeEvent is the JSON shape posted to the partner's
// account-change callback endpoint. v0.2 chunk #8.
//
// Account-change is **inbound-only** — there is no outbound /auth-
// style POST that triggers it; atome-fin pushes these events
// whenever a user's account state mutates (credit-limit change,
// status change, manual ops action, etc.). Partners that don't
// need account-change tracking can leave the handler unmounted.
//
// The data shape reuses payment.AccountChanges for the credit-
// change vector — the per-call delta semantics are identical to
// what auth/capture/void/refund return inside their own response
// data, so partners learn one AccountChanges shape across the SDK.
type AccountChangeEvent struct {
	Code    atomefin.Code      `json:"code"`
	Message string             `json:"message"`
	Data    *AccountChangeData `json:"data,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS.
// Nil-safe.
func (e *AccountChangeEvent) IsSuccess() bool {
	return e != nil && e.Code == atomefin.CodeSuccess
}

// AccountChangeData is the `data` body of AccountChangeEvent.
//
// AccountChanges carries the credit-change vector and the position-
// scoped previous/current AccountStatus enums (Q24): partners that
// validate the enum-position rule should call
// payment.IsValidPreviousStatus / IsValidCurrentStatus on the
// embedded fields.
type AccountChangeData struct {
	// EventID is the partner-side dedupe key. Atome-fin guarantees
	// at-least-once delivery, so partner handlers should keep a
	// short-lived dedupe set keyed on EventID (typical pattern).
	EventID string `json:"eventId"`

	// ExternalReferenceUID is the partner's user identifier — same
	// shape as everywhere else in the SDK.
	ExternalReferenceUID string `json:"externalReferenceUid"`

	// EventTime is Unix-ms (R-time policy — int64 on the wire,
	// never time.Time on the JSON struct).
	EventTime int64 `json:"eventTime"`

	// AccountChanges carries the credit-change vector. Re-uses
	// payment.AccountChanges so the 11-field schema stays
	// canonical across every Service that emits a credit-change
	// (auth/capture/void/refund/account-change). Required.
	AccountChanges *payment.AccountChanges `json:"accountChanges"`

	// ExtendInfo carries optional context about what triggered the
	// account change — kept opt-in so future spec additions don't
	// force a struct break.
	ExtendInfo *AccountChangeExtendInfo `json:"extendInfo,omitempty"`
}

// AccountChangeExtendInfo carries free-form metadata about an
// account-change event. Fields are optional and may evolve with
// the spec; partners should treat unknown fields as informational
// rather than load-bearing.
type AccountChangeExtendInfo struct {
	// Reason is a short human-readable label for the change
	// (e.g., "credit-limit-decrease", "manual-block",
	// "auto-close-after-overdue"). Kept as a free-form string
	// rather than an enum because the spec leaves the value set
	// open.
	Reason string `json:"reason,omitempty"`

	// TriggerSource describes what initiated the change
	// (e.g., "RISK_ENGINE", "OPS_MANUAL",
	// "REPAYMENT_DEFAULT"). Same caveat as Reason — string for
	// forward-compat.
	TriggerSource string `json:"triggerSource,omitempty"`
}
