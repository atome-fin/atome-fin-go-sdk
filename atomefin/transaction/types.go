package transaction

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// TransactionType enumerates the kinds of originating transaction
// the /transactions and /transactionDetail endpoints address. Per
// the 2026-05-06 spec snapshot, this is an alias of PAYMENT,
// REFUND, or REPAYMENT — discriminating which originating
// `requestId` the lookup is rooted in. Forward-compat: unknown
// values round-trip opaquely.
//
// Renamed v0.2.3 from `TradeType` (which previously enumerated
// AUTH/CAPTURE/VOID/REFUND) — the partner-pending 2026-04-22
// snapshot used a different wire shape. v0.2.0 — v0.2.2 callers
// must update both the type name and the literal values.
type TransactionType string

// Spec-defined transaction types (alias PAYMENT, REFUND, REPAYMENT).
const (
	// TransactionTypePayment — capture-side outcomes.
	TransactionTypePayment TransactionType = "PAYMENT"
	// TransactionTypeRefund — refund outcomes.
	TransactionTypeRefund TransactionType = "REFUND"
	// TransactionTypeRepayment — repayment outcomes.
	TransactionTypeRepayment TransactionType = "REPAYMENT"
)

// IsValid reports whether t is a spec-defined transaction type.
// Forward-compat: unknown values round-trip opaquely.
func (t TransactionType) IsValid() bool {
	switch t {
	case TransactionTypePayment, TransactionTypeRefund, TransactionTypeRepayment:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (t TransactionType) String() string { return string(t) }

// Transaction is the list-row type returned by /transactions. The
// shape is also embedded into TransactionDetail for the single-
// transaction view.
//
// Money fields follow the project-wide int64-minor-unit policy.
// TradeTime is int64 ms-since-epoch (R-time policy — never time.Time
// on the JSON struct). TradeStatus reuses atomefin.Status — the
// terminal-state lifecycle is identical to auth/capture/void.
type Transaction struct {
	// TradeID is the atome-fin-side unique identifier for this
	// transaction. Acts as the param for /transactionDetail.
	TradeID string `json:"tradeId"`

	// TransactionType discriminates the originating call. See the
	// TransactionType enum (PAYMENT / REFUND / REPAYMENT).
	TransactionType TransactionType `json:"transactionType"`

	// AuthOrderID links to the originating /auth (always present;
	// every transaction roots in an auth).
	AuthOrderID string `json:"authOrderId"`

	// OrderID is the per-capture / per-refund identifier, present
	// only on TradeTypeCapture / TradeTypeRefund rows.
	OrderID string `json:"orderId,omitempty"`

	// ExternalReferenceUID is the partner's user identifier — echoed
	// so partners can correlate without an extra round-trip.
	ExternalReferenceUID string `json:"externalReferenceUid,omitempty"`

	Currency atomefin.Currency `json:"currency"`
	Amount   atomefin.Amount   `json:"amount"`

	TradeStatus atomefin.Status `json:"tradeStatus"`

	// TradeTime is Unix-ms (DESIGN.md §1.5 time policy).
	TradeTime int64 `json:"tradeTime"`
}

// TransactionDetail is the full single-transaction type returned by
// /transactionDetail. Embeds Transaction so the list-row fields are
// accessible verbatim, plus optional context fields the row view
// elides.
type TransactionDetail struct {
	Transaction

	// BillID associates this transaction with a bill (yyyyMM) when
	// applicable — populated for transactions that contributed to a
	// bill (e.g. a CAPTURE on the month it was billed).
	BillID string `json:"billId,omitempty"`

	// FailureCode is set when TradeStatus == FAILED.
	FailureCode atomefin.FailureCode `json:"failureCode,omitempty"`

	// Notes is free-form metadata the server may attach (settlement
	// reference, partner-side trace ids, etc.). Optional.
	Notes string `json:"notes,omitempty"`
}

// ---------- Request param types ----------

// TransactionsParams are the query params for GET /transactions
// (and TransactionsAll). All fields optional; the zero value
// queries page 1 of 20 items with no filters.
type TransactionsParams struct {
	// PageNumber is 1-indexed. Zero defaults to 1.
	PageNumber int
	// PageSize is the maximum rows per page. Zero defaults to 20.
	PageSize int

	// ExternalReferenceUID filters to a single user.
	ExternalReferenceUID string
	// AuthOrderID filters to all transactions rooted in a single
	// /auth.
	AuthOrderID string
	// TransactionType discriminates the originating endpoint
	// (PAYMENT / REFUND / REPAYMENT). REQUIRED on the wire per the
	// 2026-05-06 spec snapshot. Renamed v0.2.3 from `TradeType`.
	TransactionType TransactionType
	// StartDate / EndDate filter the trade-time window. The spec
	// declares these as yyyyMMdd strings (REQUIRED on the wire).
	// TZ open per DESIGN §13/Q11.
	StartDate string
	EndDate   string
}

// ---------- Response envelopes ----------

// TransactionsResponse is the GET /transactions outer envelope.
type TransactionsResponse struct {
	Code    atomefin.Code     `json:"code"`
	Message string            `json:"message"`
	Data    *TransactionsData `json:"data,omitempty"`
}

// TransactionsData is the paginated list payload.
//
// Items is bare `json:"items"` (no `,omitempty`) per the
// paginated-list pattern codified in v0.2 chunk #3 (bill): empty
// pages round-trip as `[]` rather than disappearing — partner code
// reading data.items shouldn't need to nil-check on a 0-row page.
type TransactionsData struct {
	PageNumber int           `json:"pageNumber"`
	PageSize   int           `json:"pageSize"`
	Total      int           `json:"total"`
	Items      []Transaction `json:"items"`
}

// TransactionDetailResponse is the GET /transactionDetail outer
// envelope.
type TransactionDetailResponse struct {
	Code    atomefin.Code      `json:"code"`
	Message string             `json:"message"`
	Data    *TransactionDetail `json:"data,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *TransactionsResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *TransactionDetailResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}
