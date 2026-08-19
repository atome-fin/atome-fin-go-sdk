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
	MainOrders       []BillMainOrder   `json:"mainOrders,omitempty"`
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

// BillMainOrder is one merchant-dimension bill line on /billDetail
// (swagger BillMainOrder). merchantId / subOrderId echo POST /capture
// subOrders[] when those fields were sent (GRAB_MART / GRAB_FOOD);
// TRANSPORT typically omits both. mainOrderId is not used.
type BillMainOrder struct {
	OrderID           string          `json:"orderId"`
	RequestID         string          `json:"requestId"`
	MerchantID        string          `json:"merchantId,omitempty"`
	SubOrderID        string          `json:"subOrderId,omitempty"`
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

// BillDetailParams are the query params for GET /billDetail.
type BillDetailParams struct {
	// BillID identifies the bill month. Format: yyyyMM. Example: 202607.
	BillID               string
	ExternalReferenceUID string
	Start                int
	Count                int
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
	BilledCurrentBillID             string          `json:"billedCurrentBillId"`
	OverdueStatus                   OverdueStatus   `json:"overdueStatus"`
	DaysPastDue                     int             `json:"daysPastDue"`
	BilledCurrentDueDate            string          `json:"billedCurrentDueDate"`
	BilledCurrentBillDate           string          `json:"billedCurrentBillDate"`
	BilledCurrentStartDate          string          `json:"billedCurrentStartDate"`
	BilledCurrentEndDate            string          `json:"billedCurrentEndDate"`
	TotalAmountToBeRepaid           atomefin.Amount `json:"totalAmountToBeRepaid"`
	TotalPrincipalAmountToBeRepaid  atomefin.Amount `json:"totalPrincipalAmountToBeRepaid"`
	TotalInterestAmountToBeRepaid   atomefin.Amount `json:"totalInterestAmountToBeRepaid"`
	TotalLateFeeAmountToBeRepaid    atomefin.Amount `json:"totalLateFeeAmountToBeRepaid"`
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
