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
	// the /capture that originated CaptureRequestID.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// CaptureRequestID is the requestId of the original POST /capture
	// call that created the loan being refunded. Per spec: refund is
	// always issued against a captured order.
	//
	// Renamed v0.2.3 from `AuthOrderID` (`json:"authOrderId"`) to
	// match the 2026-05-06 spec snapshot's RefundParam schema. v0.2.0
	// / v0.2.1 / v0.2.2 callers must update to the new name; the
	// underlying value semantics also shift from "the authOrderId
	// returned by /auth" to "the requestId sent on the prior
	// /capture" — partner-side capture-id bookkeeping is required.
	CaptureRequestID string `json:"captureRequestId"`
	// RefundAmount is the total refund value in minor units.
	// Q25 (partner-pending): the SDK's validator currently enforces
	// RefundAmount == Σ SubOrders[].Amount. Relax once the spec
	// clarifies partial-refund semantics — see doc.go.
	RefundAmount atomefin.Amount `json:"refundAmount"`
	// SubOrders enumerates which sub-order lines to refund and how
	// much of each. Required, non-empty. Renamed v0.2.3 from
	// `SubOrderRefunds` to match the 2026-05-06 spec snapshot.
	SubOrders []SubOrderRefundRequest `json:"subOrders"`
}

// SubOrderRefundRequest is one line in RefundParam.SubOrders.
//
// Renamed v0.2.3: per-line `RefundAmount` (`json:"refundAmount"`) →
// `Amount` (`json:"amount"`) to match the 2026-05-06 spec snapshot's
// SubOrderRefundRequest component.
type SubOrderRefundRequest struct {
	// SubOrderID identifies the line — must match a SubOrderID that
	// was on the original /capture.
	SubOrderID string `json:"subOrderId"`
	// Amount is the per-line refund value in minor units.
	Amount atomefin.Amount `json:"amount"`
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
