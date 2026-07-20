package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// PaymentPlanRequest is the POST /payment-plan body. Run this BEFORE
// /auth to retrieve the available installment-plan options for a
// proposed transaction.
type PaymentPlanRequest struct {
	// RequestID is client-side only for idempotency logging; not in the
	// spec body. Populated by the SDK when empty before the network call.
	RequestID string `json:"-"`
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// TotalAmount in minor units; Σ(SubOrders[].Amount) must equal.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// SubOrders enumerates the cart contents to plan against.
	SubOrders []PlanSubOrder `json:"subOrders"`
	// ExtendInfo carries checkout extension fields. Required by the
	// spec because extendInfo.orderType drives risk routing.
	ExtendInfo *CheckoutExtendInfo `json:"extendInfo"`

	// Sessionid is the per-checkout session token. Required for
	// /payment-plan, ≤ 64 chars. Travels in the HTTP header, not
	// the JSON body (json:"-").
	Sessionid string `json:"-"` // max 64
}

// CheckoutExtendInfo is the extendInfo bag on /payment-plan (and
// optionally other checkout endpoints).
type CheckoutExtendInfo struct {
	OrderType PaymentOrderType `json:"orderType"`
}

// PlanSubOrder is one cart line on a plan or pre-check request.
// Field set aligns with the spec's PlanSubOrder schema (one SKU = one
// sub-order) and mirrors the commerce-domain required fields on /auth.
type PlanSubOrder struct {
	SubOrderID      string              `json:"subOrderId"`
	SkuID           string              `json:"skuId"`
	SkuName         string              `json:"skuName,omitempty"`
	CategoryID      string              `json:"categoryId"`
	CategoryOneName string              `json:"categoryOneName"`
	CategoryCodes   []string            `json:"categoryCodes,omitempty"`
	Quantity        int                 `json:"quantity"`
	Amount          atomefin.Amount     `json:"amount"`
	MerchantID      string              `json:"merchantId"`
	ExtendInfo      *SubOrderExtendInfo `json:"extendInfo,omitempty"`
}

// PaymentPlanResponse is the POST /payment-plan envelope.
type PaymentPlanResponse struct {
	Code    atomefin.Code    `json:"code"`
	Message string           `json:"message"`
	Data    *PaymentPlanData `json:"data,omitempty"`
}

// PaymentPlanData is the `data` body of PaymentPlanResponse.
type PaymentPlanData struct {
	Currency                 atomefin.Currency                  `json:"currency,omitempty"`
	SubOrderInstallmentPlans []CommerceSubOrderInstallmentPlans `json:"subOrderInstallmentPlans"`
	ExtendInfo               *PaymentPlanDataExtendInfo         `json:"extendInfo,omitempty"`
}

// CommerceSubOrderInstallmentPlans groups plan options for one sub-order.
type CommerceSubOrderInstallmentPlans struct {
	SubOrderID       string                    `json:"subOrderId"`
	OrderAmount      atomefin.Amount           `json:"orderAmount"`
	InstallmentPlans []CommerceInstallmentPlan `json:"installmentPlans"`
}

// PaymentPlanDataExtendInfo carries aggregate order-level installment plans.
type PaymentPlanDataExtendInfo struct {
	SumOrderInstallmentPlans *SumOrderInstallmentPlans `json:"sumOrderInstallmentPlans,omitempty"`
}

// SumOrderInstallmentPlans groups aggregate order-level plan options.
type SumOrderInstallmentPlans struct {
	SumOrderAmount   atomefin.Amount           `json:"sumOrderAmount,omitempty"`
	InstallmentPlans []SumOrderInstallmentPlan `json:"installmentPlans,omitempty"`
}

// SumOrderInstallmentPlan is one aggregate order-level tenor option.
type SumOrderInstallmentPlan struct {
	TotalTenor             int                         `json:"totalTenor"`
	SumOrderRepayAmount    atomefin.Amount             `json:"sumOrderRepayAmount"`
	SumOrderInterestAmount atomefin.Amount             `json:"sumOrderInterestAmount"`
	SumOrderDiscountAmount atomefin.Amount             `json:"sumOrderDiscountAmount"`
	InstallmentDetails     []SumOrderInstallmentDetail `json:"installmentDetails"`
}

// SumOrderInstallmentDetail is one aggregate installment row.
type SumOrderInstallmentDetail struct {
	TotalTenor      int             `json:"totalTenor"`
	CurrentTenor    int             `json:"currentTenor"`
	RepayAmount     atomefin.Amount `json:"repayAmount"`
	PrincipalAmount atomefin.Amount `json:"principalAmount"`
	InterestAmount  atomefin.Amount `json:"interestAmount"`
	DiscountAmount  atomefin.Amount `json:"discountAmount,omitempty"`
	BillID          string          `json:"billId"`
	BillDate        string          `json:"billDate"`
	DueDate         string          `json:"dueDate"`
}

// CommerceInstallmentPlan is one sub-order tenor option returned by
// /payment-plan.
type CommerceInstallmentPlan struct {
	TotalTenor          int                         `json:"totalTenor"`
	OrderRepayAmount    atomefin.Amount             `json:"orderRepayAmount"`
	OrderInterestAmount atomefin.Amount             `json:"orderInterestAmount"`
	OrderDiscountAmount atomefin.Amount             `json:"orderDiscountAmount"`
	InstallmentDetails  []CommerceInstallmentDetail `json:"installmentDetails"`
}

// CommerceInstallmentDetail is one row of a sub-order installment plan.
type CommerceInstallmentDetail struct {
	SubOrderID      string          `json:"subOrderId"`
	TotalTenor      int             `json:"totalTenor"`
	CurrentTenor    int             `json:"currentTenor"`
	RepayAmount     atomefin.Amount `json:"repayAmount"`
	PrincipalAmount atomefin.Amount `json:"principalAmount"`
	InterestAmount  atomefin.Amount `json:"interestAmount"`
	DiscountAmount  atomefin.Amount `json:"discountAmount,omitempty"`
	BillID          string          `json:"billId"`
	BillDate        string          `json:"billDate"`
	DueDate         string          `json:"dueDate"`
}

// PaymentPlan submits POST /payment-plan. Auto-mints RequestID
// when empty; signed body via atomefin.MarshalSigning.
func (s *Service) PaymentPlan(ctx context.Context, req *PaymentPlanRequest) (*PaymentPlanResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil PaymentPlanRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validatePaymentPlanRequest(req); err != nil {
		return nil, err
	}

	body, err := atomefin.MarshalSigning(req)
	if err != nil {
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	opts := []atomefin.DoSignedOption{}
	if req.Sessionid != "" {
		opts = append(opts, atomefin.WithRequestHeader("sessionid", req.Sessionid))
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/payment-plan", body, opts...)
	if err != nil {
		return nil, err
	}
	var out PaymentPlanResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/payment-plan",
			Err: fmt.Errorf("decode /payment-plan response: %w", uerr),
		}
	}
	return &out, nil
}

// validatePaymentPlanRequest mirrors validatePaymentPreCheckRequest
// — same shape rules, sub-order sum check.
func validatePaymentPlanRequest(req *PaymentPlanRequest) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > 64 {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if req.TotalAmount <= 0 {
		return &atomefin.ValidationError{Field: "totalAmount", Message: "must be > 0 (minor units)"}
	}
	if len(req.SubOrders) == 0 {
		return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
	}
	var sum atomefin.Amount
	for _, so := range req.SubOrders {
		if err := validatePlanSubOrder(so); err != nil {
			return err
		}
		sum += so.Amount
	}
	if sum != req.TotalAmount {
		return &atomefin.ValidationError{
			Field:   "totalAmount",
			Message: "must equal sum of subOrders[].amount",
		}
	}
	if req.ExtendInfo == nil {
		return &atomefin.ValidationError{Field: "extendInfo", Message: "required (carries orderType)"}
	}
	if !req.ExtendInfo.OrderType.IsValid() {
		return &atomefin.ValidationError{Field: "extendInfo.orderType", Message: "must be one of TRANSPORT | GRAB_FOOD | GRAB_MART | SPECIALIZED_DELIVERY"}
	}
	if req.Sessionid == "" {
		return &atomefin.ValidationError{Field: "sessionid", Message: "required (HTTP header on /payment-plan)"}
	}
	if len(req.Sessionid) > 64 {
		return &atomefin.ValidationError{Field: "sessionid", Message: "exceeds spec maxlength 64"}
	}
	return nil
}
