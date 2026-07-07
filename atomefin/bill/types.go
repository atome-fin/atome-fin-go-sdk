package bill

import "github.com/atome-fin/atome-fin-go-sdk/atomefin"

// OverdueStatus is the spec's overdue axis for bills.
type OverdueStatus string

// Spec-defined overdue statuses.
const (
	OverdueStatusNotOverdue OverdueStatus = "NOT_OVERDUE"
	OverdueStatusOverdue    OverdueStatus = "OVERDUE"
)

// IsValid reports whether s is a spec-defined overdue status.
// Forward-compat: unknown values round-trip opaquely so future spec
// additions decode cleanly; partners that compare against IsValid
// before branching get strict-enum semantics.
func (s OverdueStatus) IsValid() bool {
	switch s {
	case OverdueStatusNotOverdue, OverdueStatusOverdue:
		return true
	}
	return false
}

// String returns the wire literal verbatim.
func (s OverdueStatus) String() string { return string(s) }

// BillStatus is the spec's billing status axis.
type BillStatus string

const (
	BillStatusBilled   BillStatus = "BILLED"
	BillStatusUnbilled BillStatus = "UNBILLED"
)

// RefundStatus is the spec's refund status axis.
type RefundStatus string

const (
	RefundStatusFullRefunded    RefundStatus = "FULL_REFUNDED"
	RefundStatusPartialRefunded RefundStatus = "PARTIAL_REFUNDED"
	RefundStatusNotRefunded     RefundStatus = "NOT_REFUNDED"
)

// Bill is the schema returned under /bills data[] and BillDetail.bill.
type Bill struct {
	BillID            string          `json:"billId"`
	BillMonth         string          `json:"billMonth"`
	BillStartDate     string          `json:"billStartDate,omitempty"`
	BillEndDate       string          `json:"billEndDate,omitempty"`
	BillTotalAmount   atomefin.Amount `json:"billTotalAmount"`
	OutstandingAmount atomefin.Amount `json:"outstandingAmount"`
	RepaidAmount      atomefin.Amount `json:"repaidAmount"`
	PrincipalAmount   atomefin.Amount `json:"principalAmount"`
	InterestAmount    atomefin.Amount `json:"interestAmount"`
	LateFeeAmount     atomefin.Amount `json:"lateFeeAmount,omitempty"`
	RefundAmount      atomefin.Amount `json:"refundAmount,omitempty"`
	DueDate           string          `json:"dueDate"`
	DaysPastDue       int             `json:"daysPastDue,omitempty"`
	GracePeriod       int             `json:"gracePeriod,omitempty"`
	Status            BillStatus      `json:"status,omitempty"`
	RefundStatus      RefundStatus    `json:"refundStatus,omitempty"`
	RepaymentStatus   string          `json:"repaymentStatus"`
	OverdueStatus     OverdueStatus   `json:"overdueStatus"`
	Discounts         *BillDiscounts  `json:"discounts,omitempty"`
}

// BillsResponseItem is one /bills data row: Bill plus currency.
type BillsResponseItem struct {
	Bill
	Currency atomefin.Currency `json:"currency"`
}

// BillDetail is the full single-bill type returned by /billDetail.
type BillDetail struct {
	Currency         atomefin.Currency `json:"currency,omitempty"`
	Bill             *Bill             `json:"bill,omitempty"`
	RepaymentDetails []RepaymentDetail `json:"repaymentDetails,omitempty"`
	Orders           []BillOrder       `json:"orders,omitempty"`
	Paginator        *Paginator        `json:"paginator,omitempty"`
}

// RepaymentDetail is one repayment write-off row under bill detail.
type RepaymentDetail struct {
	RepaymentRequestID    string          `json:"repaymentRequestId,omitempty"`
	RepaymentAmount       atomefin.Amount `json:"repaymentAmount,omitempty"`
	RepaidPrincipalAmount atomefin.Amount `json:"repaidPrincipalAmount,omitempty"`
	RepaidInterestAmount  atomefin.Amount `json:"repaidInterestAmount,omitempty"`
	RepaidLateFeeAmount   atomefin.Amount `json:"repaidLateFeeAmount,omitempty"`
	RepaymentTime         string          `json:"repaymentTime,omitempty"`
	Event                 string          `json:"event,omitempty"`
}

// Paginator is the spec paginator object used by detail/list aggregate payloads.
type Paginator struct {
	Start      int `json:"start,omitempty"`
	Count      int `json:"count,omitempty"`
	TotalCount int `json:"totalCount,omitempty"`
}

// BillOrder is one order line on a bill detail response.
type BillOrder struct {
	OrderID           string          `json:"orderId"`
	RequestID         string          `json:"requestId"`
	CreateTime        int64           `json:"createTime"`
	PeriodType        string          `json:"periodType"`
	CurrentPeriod     int             `json:"currentPeriod"`
	TotalAmount       atomefin.Amount `json:"totalAmount"`
	OutstandingAmount atomefin.Amount `json:"outstandingAmount"`
	RepaidAmount      atomefin.Amount `json:"repaidAmount"`
	PrincipalAmount   atomefin.Amount `json:"principalAmount"`
	InterestAmount    atomefin.Amount `json:"interestAmount"`
	RefundAmount      atomefin.Amount `json:"refundAmount,omitempty"`
	DueDate           string          `json:"dueDate"`
	Status            BillStatus      `json:"status"`
	RepaymentStatus   string          `json:"repaymentStatus"`
	OverdueStatus     OverdueStatus   `json:"overdueStatus"`
	RefundStatus      RefundStatus    `json:"refundStatus,omitempty"`
	Discounts         *BillDiscounts  `json:"discounts,omitempty"`
}

// BillDiscounts groups the discount lines applied to a bill.
type BillDiscounts struct {
	DiscountAmount               atomefin.Amount `json:"discountAmount,omitempty"`
	InterestAmountAfterDiscount  atomefin.Amount `json:"interestAmountAfterDiscount,omitempty"`
	BillTotalAmountAfterDiscount atomefin.Amount `json:"billTotalAmountAfterDiscount,omitempty"`
	RepaidAmountExcludeDiscount  atomefin.Amount `json:"repaidAmountExcludeDiscount,omitempty"`
}

// ---------- Request param types ----------

// BillsParams are the query params for GET /bills.
type BillsParams struct {
	ExternalReferenceUID string
	StartMonth           string
	EndMonth             string
	Status               []BillStatus
	RepaymentStatus      []string
	OverdueStatus        []OverdueStatus
	RefundStatus         []RefundStatus
}

// BillsUnpaidParams are the query params for GET /billUnpaid.
type BillsUnpaidParams struct {
	ExternalReferenceUID string
}

// ---------- Response envelopes ----------

// BillsResponse is the GET /bills outer envelope.
type BillsResponse struct {
	Code    atomefin.Code       `json:"code"`
	Message string              `json:"message"`
	Data    []BillsResponseItem `json:"data"`
}

// BillDetailResponse is the GET /billDetail outer envelope.
type BillDetailResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *BillDetail   `json:"data,omitempty"`
}

// BillUnpaidResponse is the GET /billUnpaid outer envelope.
type BillUnpaidResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *BillUnpaid   `json:"data,omitempty"`
}

// BillUnpaid is the unpaid summary object returned by /billUnpaid.
type BillUnpaid struct {
	BilledAmountToBeRepaid          atomefin.Amount `json:"billedAmountToBeRepaid"`
	BilledPrincipalAmountToBeRepaid atomefin.Amount `json:"billedPrincipalAmountToBeRepaid"`
	BilledInterestAmountToBeRepaid  atomefin.Amount `json:"billedInterestAmountToBeRepaid"`
	BilledLateFeeAmountToBeRepaid   atomefin.Amount `json:"billedLateFeeAmountToBeRepaid"`
	BilledCurrentBillID             string          `json:"billedCurrentBillId,omitempty"`
	OverdueStatus                   OverdueStatus   `json:"overdueStatus,omitempty"`
	DaysPastDue                     int             `json:"daysPastDue,omitempty"`
	BilledCurrentDueDate            string          `json:"billedCurrentDueDate,omitempty"`
	BilledCurrentBillDate           string          `json:"billedCurrentBillDate,omitempty"`
	BilledCurrentStartDate          string          `json:"billedCurrentStartDate,omitempty"`
	BilledCurrentEndDate            string          `json:"billedCurrentEndDate,omitempty"`
	TotalAmountToBeRepaid           atomefin.Amount `json:"totalAmountToBeRepaid,omitempty"`
	TotalPrincipalAmountToBeRepaid  atomefin.Amount `json:"totalPrincipalAmountToBeRepaid,omitempty"`
	TotalInterestAmountToBeRepaid   atomefin.Amount `json:"totalInterestAmountToBeRepaid,omitempty"`
	TotalLateFeeAmountToBeRepaid    atomefin.Amount `json:"totalLateFeeAmountToBeRepaid,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *BillsResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *BillDetailResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *BillUnpaidResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}
