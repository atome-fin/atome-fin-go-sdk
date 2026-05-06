package bill

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// OverdueStatus is the lifecycle state of a bill — whether the
// partner has paid in time, is in the grace window, or is past due.
// Closed enum per the spec convention; unknown values round-trip
// opaquely (forward-compat).
type OverdueStatus string

// Spec-defined overdue statuses.
const (
	// OverdueStatusOnTime — bill has been paid by the due date or is
	// not yet due.
	OverdueStatusOnTime OverdueStatus = "ON_TIME"
	// OverdueStatusGracePeriod — past due date but within the
	// per-partner grace window.
	OverdueStatusGracePeriod OverdueStatus = "GRACE_PERIOD"
	// OverdueStatusOverdue — past grace; subject to late-fee
	// accrual.
	OverdueStatusOverdue OverdueStatus = "OVERDUE"
)

// IsValid reports whether s is a spec-defined overdue status.
// Forward-compat: unknown values round-trip opaquely so future spec
// additions decode cleanly; partners that compare against IsValid
// before branching get strict-enum semantics.
func (s OverdueStatus) IsValid() bool {
	switch s {
	case OverdueStatusOnTime, OverdueStatusGracePeriod, OverdueStatusOverdue:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (s OverdueStatus) String() string { return string(s) }

// Bill is the list-row type returned by /bills and /billUnpaid. The
// shape is also embedded into BillDetail for the single-bill view.
//
// Money fields follow the project-wide int64-minor-unit policy.
// Date fields are passed through as strings (yyyyMM for billId,
// yyyy-MM-dd for date fields — DESIGN.md §13/Q11 keeps the TZ
// question open, so the SDK does NOT silently parse these).
type Bill struct {
	// BillID is yyyyMM (e.g. "202605"). Acts as the unique id when
	// querying /billDetail.
	BillID string `json:"billId"` // yyyyMM
	// BillDate / DueDate are yyyy-MM-dd (TZ open per Q11).
	BillDate string `json:"billDate,omitempty"`
	DueDate  string `json:"dueDate,omitempty"`

	// ExternalReferenceUID is the partner's user identifier — echoed
	// so partners can correlate without an extra round-trip.
	ExternalReferenceUID string `json:"externalReferenceUid,omitempty"`

	Currency     atomefin.Currency `json:"currency"`
	TotalAmount  atomefin.Amount   `json:"totalAmount"`
	PaidAmount   atomefin.Amount   `json:"paidAmount,omitempty"`
	UnpaidAmount atomefin.Amount   `json:"unpaidAmount,omitempty"`

	OverdueStatus OverdueStatus `json:"overdueStatus,omitempty"`
}

// BillDetail is the full single-bill type returned by /billDetail.
// Embeds Bill so the list-row fields are accessible verbatim, plus
// adds the per-order breakdown and any discount summary.
type BillDetail struct {
	Bill
	Orders    []BillOrder    `json:"orders,omitempty"`
	Discounts *BillDiscounts `json:"discounts,omitempty"`
}

// BillOrder is one order line on a bill — typically corresponds to
// a prior /capture's CaptureResultData.
type BillOrder struct {
	AuthOrderID string          `json:"authOrderId"`
	OrderID     string          `json:"orderId,omitempty"`
	Amount      atomefin.Amount `json:"amount"`
	Status      atomefin.Status `json:"status,omitempty"`
}

// BillDiscounts groups the discount lines applied to a bill.
type BillDiscounts struct {
	TotalDiscount atomefin.Amount `json:"totalDiscount"`
	Items         []Discount      `json:"items,omitempty"`
}

// Discount is one discount line.
type Discount struct {
	DiscountID string          `json:"discountId"`
	Amount     atomefin.Amount `json:"amount"`
	Detail     *DiscountDetail `json:"detail,omitempty"`
}

// DiscountDetail carries free-form metadata about a discount
// (description, source code, partner-specific tags). Fields kept
// minimal until the spec stabilises; partners that need richer
// detail should track DESIGN.md §13 for spec evolution.
type DiscountDetail struct {
	Description string `json:"description,omitempty"`
	SourceCode  string `json:"sourceCode,omitempty"`
}

// ---------- Request param types ----------

// BillsParams are the query params for GET /bills (and BillsAll).
// All fields are optional; the zero value queries page 1 of 20
// items with no filters.
type BillsParams struct {
	// PageNumber is 1-indexed. Zero defaults to 1.
	PageNumber int
	// PageSize is the maximum rows per page. Zero defaults to 20.
	PageSize int

	// ExternalReferenceUID filters to a single user.
	ExternalReferenceUID string
	// BillID filters to a specific yyyyMM bill.
	BillID string
	// StartDate / EndDate filter the issue window (yyyy-MM-dd, TZ
	// open per DESIGN §13/Q11).
	StartDate string
	EndDate   string
}

// BillsUnpaidParams are the query params for GET /billUnpaid. The
// endpoint pre-filters to unpaid bills, so the param surface is
// smaller than BillsParams.
type BillsUnpaidParams struct {
	// PageNumber is 1-indexed. Zero defaults to 1.
	PageNumber int
	// PageSize is the maximum rows per page. Zero defaults to 20.
	PageSize int
	// ExternalReferenceUID filters to a single user.
	ExternalReferenceUID string
}

// ---------- Response envelopes ----------

// BillsResponse is the GET /bills (and /billUnpaid) outer envelope.
type BillsResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *BillsData    `json:"data,omitempty"`
}

// BillsData is the paginated list payload.
//
// Bills is bare `json:"bills"` (no `,omitempty`) so an empty page
// round-trips as `[]` rather than disappearing — partner code that
// reads `data.bills` shouldn't need to nil-check on a 0-row page.
type BillsData struct {
	PageNumber int    `json:"pageNumber"`
	PageSize   int    `json:"pageSize"`
	Total      int    `json:"total"`
	Bills      []Bill `json:"bills"`
}

// BillDetailResponse is the GET /billDetail outer envelope.
type BillDetailResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *BillDetail   `json:"data,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *BillsResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *BillDetailResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}
