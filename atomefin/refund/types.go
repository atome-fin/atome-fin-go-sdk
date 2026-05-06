package refund

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// RefundParam is the POST /refund request body. v0.2.
//
// The shape mirrors capture's idempotency + sub-order-list pattern:
// `requestId` is the partner's idempotency key; `authOrderId`
// identifies the prior /auth that produced the credit being
// refunded; `subOrderRefunds` enumerates which lines (and how much
// of each) to refund.
type RefundParam struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's user identifier; matches
	// the /auth that originated AuthOrderID.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// AuthOrderID is the value returned by the prior /auth.
	AuthOrderID string `json:"authOrderId"`
	// RefundAmount is the total refund value in minor units.
	// Q25 (partner-pending): the SDK's validator currently enforces
	// RefundAmount == Σ SubOrderRefunds[].RefundAmount. Relax once
	// the spec clarifies partial-refund semantics — see doc.go.
	RefundAmount atomefin.Amount `json:"refundAmount"`
	// SubOrderRefunds enumerates which sub-order lines to refund and
	// how much of each. Required, non-empty.
	SubOrderRefunds []SubOrderRefundRequest `json:"subOrderRefunds"`
}

// SubOrderRefundRequest is one line in RefundParam.SubOrderRefunds.
type SubOrderRefundRequest struct {
	// SubOrderID identifies the line — must match a SubOrderID that
	// was on the original /auth.
	SubOrderID string `json:"subOrderId"`
	// RefundAmount is the per-line refund value in minor units.
	RefundAmount atomefin.Amount `json:"refundAmount"`
}

// RefundResponse is the POST /refund (and /query-refund) outer
// envelope. Same Code / Message / Data shape as the payment
// responses; the inner data type is RefundResult.
type RefundResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *RefundResult `json:"data,omitempty"`
}

// IsTerminal reports whether the response carries a terminal Status.
// Nil-safe.
func (r *RefundResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// IsProcessing reports whether the response is the async PROCESSING
// envelope. Nil-safe.
func (r *RefundResponse) IsProcessing() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status == atomefin.StatusProcessing
}

// RefundResult is the `data` body of RefundResponse.
//
// AccountChanges re-uses payment.AccountChanges (the credit-change
// vector is identical across auth/capture/void/refund). refund
// imports payment for this — there's no cycle because payment does
// not import refund.
type RefundResult struct {
	RequestID            string `json:"requestId"`
	ExternalReferenceUID string `json:"externalReferenceUid"`
	AuthOrderID          string `json:"authOrderId"`
	// RefundOrderID is the atome-fin-side identifier for this
	// refund (analog of CaptureResultData.OrderID); max 32.
	RefundOrderID       string                  `json:"refundOrderId"` // max 32
	Currency            atomefin.Currency       `json:"currency"`
	RefundAmount        atomefin.Amount         `json:"refundAmount"`
	Status              atomefin.Status         `json:"status"`
	FailureCode         atomefin.FailureCode    `json:"failureCode,omitempty"`
	SubOrderRefundInfos []SubOrderRefundInfo    `json:"subOrderRefundInfos,omitempty"`
	AccountChanges      *payment.AccountChanges `json:"accountChanges,omitempty"`
}

// SubOrderRefundInfo is one line in RefundResult.SubOrderRefundInfos.
// Mirrors SubOrderRefundRequest in shape; the response carries the
// same fields the request did, optionally annotated by the server
// with terminal-state metadata.
type SubOrderRefundInfo struct {
	SubOrderID   string          `json:"subOrderId"`
	RefundAmount atomefin.Amount `json:"refundAmount"`
}
