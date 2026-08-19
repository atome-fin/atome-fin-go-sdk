package transaction

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/bill"
)

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

// ---------- Request param types ----------

// TransactionsParams are the query params for GET /transactions.
type TransactionsParams struct {
	ExternalReferenceUID string
	TransactionType      TransactionType
	StartDate            string
	EndDate              string
	Start                int
	Count                int
}

// ---------- Response envelopes ----------

// TransactionsResponse is the GET /transactions outer envelope.
type TransactionsResponse struct {
	Code    atomefin.Code     `json:"code"`
	Message string            `json:"message"`
	Data    *TransactionsData `json:"data,omitempty"`
}

// TransactionsData is the GET /transactions payload.
type TransactionsData struct {
	Currency      atomefin.Currency    `json:"currency,omitempty"`
	PaymentInfo   []TradePaymentInfo   `json:"paymentInfo,omitempty"`
	RefundInfo    []TradeRefundInfo    `json:"refundInfo,omitempty"`
	RepaymentInfo []TradeRepaymentInfo `json:"repaymentInfo,omitempty"`
	Paginator     *Paginator           `json:"paginator,omitempty"`
}

// TransactionDetailResponse is the GET /transactionDetail outer
// envelope.
type TransactionDetailResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *TradeDetail  `json:"data,omitempty"`
}

type Paginator struct {
	Start      int `json:"start,omitempty"`
	Count      int `json:"count,omitempty"`
	TotalCount int `json:"totalCount,omitempty"`
}

type TradePaymentInfo struct {
	CaptureRequestID string          `json:"captureRequestId"`
	OrderID          string          `json:"orderId"`
	SubOrders        []TradeSubOrder `json:"subOrders,omitempty"`
	TotalTenor       int             `json:"totalTenor"`
	CreateTime       int64           `json:"createTime"`
}

type TradeRefundInfo struct {
	RefundRequestID       string                `json:"refundRequestId"`
	CaptureRequestID      string                `json:"captureRequestId"`
	RefundAmount          atomefin.Amount       `json:"refundAmount"`
	RefundDiscountAmount  atomefin.Amount       `json:"refundDiscountAmount,omitempty"`
	WaivedInterestAmount  atomefin.Amount       `json:"waivedInterestAmount,omitempty"`
	OverpaidAmountChange  atomefin.Amount       `json:"overpaidAmountChange,omitempty"`
	AvailableCreditChange atomefin.Amount       `json:"availableCreditChange,omitempty"`
	LateFeeAmountChange   atomefin.Amount       `json:"lateFeeAmountChange,omitempty"`
	CreateTime            int64                 `json:"createTime"`
	SubOrders             []TradeRefundSubOrder `json:"subOrders"`
}

type TradeRepaymentInfo struct {
	RepaymentRequestID   string                 `json:"repaymentRequestId"`
	RepaymentAmount      atomefin.Amount        `json:"repaymentAmount"`
	OverpaidAmountChange atomefin.Amount        `json:"overpaidAmountChange,omitempty"`
	LateFeeAmountChange  atomefin.Amount        `json:"lateFeeAmountChange,omitempty"`
	Event                string                 `json:"event"`
	CreateTime           int64                  `json:"createTime"`
	RepaymentDetails     []TradeRepaymentDetail `json:"repaymentDetails,omitempty"`
}

type TradeSubOrder struct {
	SubOrderID      string          `json:"subOrderId,omitempty"`
	MerchantID      string          `json:"merchantId,omitempty"`
	PrincipalAmount atomefin.Amount `json:"principalAmount"`
	InterestAmount  atomefin.Amount `json:"interestAmount,omitempty"`
	DiscountAmount  atomefin.Amount `json:"discountAmount,omitempty"`
}

type TradeRefundSubOrder struct {
	SubOrderID            string          `json:"subOrderId"`
	RefundStatus          string          `json:"refundStatus"`
	PrincipalAmount       atomefin.Amount `json:"principalAmount"`
	WaivedInterestAmount  atomefin.Amount `json:"waivedInterestAmount,omitempty"`
	AvailableCreditChange atomefin.Amount `json:"availableCreditChange,omitempty"`
	DiscountAmount        atomefin.Amount `json:"discountAmount,omitempty"`
	OverpaidAmountChange  atomefin.Amount `json:"overpaidAmountChange,omitempty"`
}

type TradeRepaymentDetail struct {
	BillID          string          `json:"billId"`
	DueDate         string          `json:"dueDate"`
	BillStartDate   string          `json:"billStartDate"`
	BillEndDate     string          `json:"billEndDate"`
	RepaymentAmount atomefin.Amount `json:"repaymentAmount"`
}

type TradeDetail struct {
	Currency      atomefin.Currency         `json:"currency,omitempty"`
	PaymentInfo   *TradePaymentInfoDetail   `json:"paymentInfo,omitempty"`
	RefundInfo    *RefundInfoDetail         `json:"refundInfo,omitempty"`
	RepaymentInfo *TradeRepaymentInfoDetail `json:"repaymentInfo,omitempty"`
}

type TradePaymentInfoDetail struct {
	CaptureRequestID string                `json:"captureRequestId"`
	OrderID          string                `json:"orderId"`
	TotalTenor       int                   `json:"totalTenor"`
	CreateTime       int64                 `json:"createTime"`
	PrincipalAmount  atomefin.Amount       `json:"principalAmount"`
	InterestAmount   atomefin.Amount       `json:"interestAmount,omitempty"`
	DiscountAmount   atomefin.Amount       `json:"discountAmount,omitempty"`
	SubOrders        []TradeSubOrderDetail `json:"subOrders,omitempty"`
}

type TradeSubOrderDetail struct {
	SubOrderID       string                  `json:"subOrderId,omitempty"`
	MerchantID       string                  `json:"merchantId,omitempty"`
	PrincipalAmount  atomefin.Amount         `json:"principalAmount"`
	InterestAmount   atomefin.Amount         `json:"interestAmount,omitempty"`
	DiscountAmount   atomefin.Amount         `json:"discountAmount,omitempty"`
	BillOrderDetails []TradeBillDetail       `json:"billOrderDetails,omitempty"`
	RefundInfo       []TradeRefundInfoDetail `json:"refundInfo,omitempty"`
}

type TradeBillDetail struct {
	BillID            string              `json:"billId"`
	BillDate          string              `json:"billDate"`
	DueDate           string              `json:"dueDate"`
	BillStartDate     string              `json:"billStartDate,omitempty"`
	BillEndDate       string              `json:"billEndDate,omitempty"`
	TotalAmount       atomefin.Amount     `json:"totalAmount"`
	OutstandingAmount atomefin.Amount     `json:"outstandingAmount,omitempty"`
	RepaidAmount      atomefin.Amount     `json:"repaidAmount"`
	PrincipalAmount   atomefin.Amount     `json:"principalAmount"`
	InterestAmount    atomefin.Amount     `json:"interestAmount"`
	GracePeriod       int                 `json:"gracePeriod,omitempty"`
	Status            bill.BillStatus     `json:"status,omitempty"`
	RepaymentStatus   string              `json:"repaymentStatus,omitempty"`
	OverdueStatus     bill.OverdueStatus  `json:"overdueStatus,omitempty"`
	Discounts         *bill.BillDiscounts `json:"discounts,omitempty"`
}

type TradeRefundInfoDetail struct {
	RefundRequestID       string          `json:"refundRequestId"`
	PrincipalAmount       atomefin.Amount `json:"principalAmount"`
	DiscountAmount        atomefin.Amount `json:"discountAmount,omitempty"`
	WaivedInterestAmount  atomefin.Amount `json:"waivedInterestAmount,omitempty"`
	AvailableCreditChange atomefin.Amount `json:"availableCreditChange,omitempty"`
	LateFeeAmountChange   atomefin.Amount `json:"lateFeeAmountChange,omitempty"`
	RefundStatus          string          `json:"refundStatus,omitempty"`
	OverpaidAmountChange  atomefin.Amount `json:"overpaidAmountChange,omitempty"`
	CreateTime            int64           `json:"createTime"`
}

type RefundInfoDetail struct {
	RefundRequestID       string                `json:"refundRequestId"`
	CaptureRequestID      string                `json:"captureRequestId"`
	RefundAmount          atomefin.Amount       `json:"refundAmount"`
	RefundDiscountAmount  atomefin.Amount       `json:"refundDiscountAmount,omitempty"`
	WaivedInterestAmount  atomefin.Amount       `json:"waivedInterestAmount,omitempty"`
	LateFeeAmountChange   atomefin.Amount       `json:"lateFeeAmountChange,omitempty"`
	AvailableCreditChange atomefin.Amount       `json:"availableCreditChange,omitempty"`
	OverpaidAmountChange  atomefin.Amount       `json:"overpaidAmountChange,omitempty"`
	CreateTime            int64                 `json:"createTime,omitempty"`
	SubOrders             []TradeRefundSubOrder `json:"subOrders,omitempty"`
}

type TradeRepaymentInfoDetail struct {
	RepaymentRequestID   string                       `json:"repaymentRequestId"`
	RepaymentAmount      atomefin.Amount              `json:"repaymentAmount"`
	OverpaidAmountChange atomefin.Amount              `json:"overpaidAmountChange,omitempty"`
	LateFeeAmountChange  atomefin.Amount              `json:"lateFeeAmountChange,omitempty"`
	Event                string                       `json:"event"`
	CreateTime           int64                        `json:"createTime"`
	RepaymentDetails     []TradeDetailRepaymentDetail `json:"repaymentDetails,omitempty"`
}

type TradeDetailRepaymentDetail struct {
	BillID                string          `json:"billId"`
	DueDate               string          `json:"dueDate"`
	BillStartDate         string          `json:"billStartDate"`
	BillEndDate           string          `json:"billEndDate"`
	RepaymentAmount       atomefin.Amount `json:"repaymentAmount"`
	RepaidPrincipalAmount atomefin.Amount `json:"repaidPrincipalAmount,omitempty"`
	RepaidInterestAmount  atomefin.Amount `json:"repaidInterestAmount,omitempty"`
	RepaidLateFeeAmount   atomefin.Amount `json:"repaidLateFeeAmount,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *TransactionsResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *TransactionDetailResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}
