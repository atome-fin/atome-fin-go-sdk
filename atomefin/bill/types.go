package bill

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

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

// RepaymentStatus is the spec's repayment status axis for bills and
// bill lines.
type RepaymentStatus string

const (
	RepaymentStatusRepaid        RepaymentStatus = "REPAID"
	RepaymentStatusUnpaid        RepaymentStatus = "UNPAID"
	RepaymentStatusPartialRepaid RepaymentStatus = "PARTIAL_REPAID"
)

// IsValid reports whether s is a spec-defined repayment status.
// Forward-compat: unknown values round-trip opaquely.
func (s RepaymentStatus) IsValid() bool {
	switch s {
	case RepaymentStatusRepaid, RepaymentStatusUnpaid, RepaymentStatusPartialRepaid:
		return true
	}
	return false
}

// RepaymentEvent is the spec's repayment trigger axis on
// repaymentDetail rows (NORMAL / ATOME_REPAYMENT /
// OVERPAID_REPAYMENT).
type RepaymentEvent string

const (
	RepaymentEventNormal          RepaymentEvent = "NORMAL"
	RepaymentEventAtomeRepayment  RepaymentEvent = "ATOME_REPAYMENT"
	RepaymentEventOverpaidRepaymt RepaymentEvent = "OVERPAID_REPAYMENT"
)

// Bill is the schema returned under /bills data[] and BillDetail.bill.
type Bill struct {
	BillID            string            `json:"billId"`
	BillMonth         string            `json:"billMonth"`
	BillStartDate     string            `json:"billStartDate"`
	BillEndDate       string            `json:"billEndDate"`
	BillTotalAmount   atomefin.Amount   `json:"billTotalAmount"`
	OutstandingAmount atomefin.Amount   `json:"outstandingAmount"`
	RepaidAmount      atomefin.Amount   `json:"repaidAmount"`
	PrincipalAmount   atomefin.Amount   `json:"principalAmount"`
	InterestAmount    atomefin.Amount   `json:"interestAmount"`
	LateFeeAmount     atomefin.Amount   `json:"lateFeeAmount"`
	RefundAmount      atomefin.Amount   `json:"refundAmount"`
	DueDate           string            `json:"dueDate"`
	DaysPastDue       int               `json:"daysPastDue"`
	GracePeriod       int               `json:"gracePeriod"`
	Status            BillStatus        `json:"status"`
	RefundStatus      RefundStatus      `json:"refundStatus"`
	Currency          atomefin.Currency `json:"currency"`
	RepaymentStatus   RepaymentStatus   `json:"repaymentStatus"`
	OverdueStatus     OverdueStatus     `json:"overdueStatus"`
	Discounts         *BillDiscounts    `json:"discounts,omitempty"`
}

// BillsResponseItem is one /bills data row: the spec's BillsResponse
// (allOf Bill + required currency). Bill already carries the
// `currency` JSON field, so the embed alone covers the full shape —
// a previous duplicate outer Currency field shadowed Bill.Currency
// and left it empty after decoding.
type BillsResponseItem struct {
	Bill
}

// BillDetail is the full single-bill type returned by /billDetail.
// currency, bill, mainOrders and paginator are required per spec
// (omitempty removed accordingly); orders is the legacy SKU-dimension
// line array, `type: object` in the spec with no schema — Grab
// returns null.
type BillDetail struct {
	Currency         atomefin.Currency `json:"currency"`
	Bill             *Bill             `json:"bill"`
	RepaymentDetails []RepaymentDetail `json:"repaymentDetails,omitempty"`
	MainOrders       []BillMainOrder   `json:"mainOrders"`
	Paginator        *Paginator        `json:"paginator"`
	Orders           []map[string]any  `json:"orders,omitempty"`
}

// RepaymentDetail is one repayment write-off row under bill detail.
type RepaymentDetail struct {
	RepaymentRequestID    string          `json:"repaymentRequestId,omitempty"`
	RepaymentAmount       atomefin.Amount `json:"repaymentAmount,omitempty"`
	RepaidPrincipalAmount atomefin.Amount `json:"repaidPrincipalAmount,omitempty"`
	RepaidInterestAmount  atomefin.Amount `json:"repaidInterestAmount,omitempty"`
	RepaidLateFeeAmount   atomefin.Amount `json:"repaidLateFeeAmount,omitempty"`
	RepaymentTime         int64           `json:"repaymentTime,omitempty"`
	Event                 RepaymentEvent  `json:"event,omitempty"`
}

// Paginator is the spec paginator object used by detail/list aggregate
// payloads. start/count/totalCount are all required per spec.
type Paginator struct {
	Start      int `json:"start"`
	Count      int `json:"count"`
	TotalCount int `json:"totalCount"`
}

// BillMainOrder is one merchant-dimension bill line on /billDetail
// (swagger BillMainOrder). merchantId / subOrderId echo POST /capture
// subOrders[] when those fields were sent (GRAB_MART / GRAB_FOOD);
// TRANSPORT typically omits both. mainOrderId is not used.
type BillMainOrder struct {
	OrderID           string                   `json:"orderId"`
	RequestID         string                   `json:"requestId"`
	MerchantID        string                   `json:"merchantId,omitempty"`
	MerchantName      string                   `json:"merchantName,omitempty"`
	OrderType         payment.PaymentOrderType `json:"orderType"`
	SubOrderID        string                   `json:"subOrderId,omitempty"`
	CreateTime        int64                    `json:"createTime"`
	PeriodType        int                      `json:"periodType"`
	CurrentPeriod     int                      `json:"currentPeriod"`
	TotalAmount       atomefin.Amount          `json:"totalAmount"`
	OutstandingAmount atomefin.Amount          `json:"outstandingAmount"`
	RepaidAmount      atomefin.Amount          `json:"repaidAmount"`
	PrincipalAmount   atomefin.Amount          `json:"principalAmount"`
	InterestAmount    atomefin.Amount          `json:"interestAmount"`
	RefundAmount      atomefin.Amount          `json:"refundAmount"`
	GracePeriod       int                      `json:"gracePeriod"`
	DueDate           string                   `json:"dueDate"`
	Status            BillStatus               `json:"status"`
	RepaymentStatus   RepaymentStatus          `json:"repaymentStatus"`
	OverdueStatus     OverdueStatus            `json:"overdueStatus"`
	RefundStatus      RefundStatus             `json:"refundStatus"`
	Discounts         *BillDiscounts           `json:"discounts,omitempty"`
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
	RepaymentStatus      []RepaymentStatus
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
