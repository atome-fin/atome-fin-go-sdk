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

	// Sessionid is deprecated per the 2026-08-11 spec: Atome now
	// returns data.extendInfo.sessionId on the /payment-plan response;
	// pass that value on POST /auth via the `sessionid` header.
	// When set, the SDK still forwards it as a request header for
	// backward compatibility. Not part of the JSON body (json:"-").
	Sessionid string `json:"-"` // max 64
}

// CheckoutExtendInfo is the extendInfo bag on /payment-plan.
// Per the spec's PaymentPlanExtendInfo, GRAB_MART requires
// mainOrderExtendInfos (each entry: merchantId + skuInfos);
// GRAB_FOOD may send partial entries; TRANSPORT omits the array.
type CheckoutExtendInfo struct {
	OrderType            PaymentOrderType      `json:"orderType"`
	MainOrderExtendInfos []MainOrderExtendInfo `json:"mainOrderExtendInfos,omitempty"`
}

// PlanSubOrder is one merchant-dimension entry on a plan or pre-check
// request (spec's PlanMerchantSubOrder). Only MerchantID and Amount
// are required for GRAB_MART; every field is optional on
// /payment-precheck (PrecheckMerchantSubOrder).
type PlanSubOrder struct {
	SubOrderID         string          `json:"subOrderId,omitempty"`
	MerchantID         string          `json:"merchantId,omitempty"`
	MerchantName       string          `json:"merchantName,omitempty"`
	MerchantCategory   string          `json:"merchantCategory,omitempty"`
	MerchantJoinedDate string          `json:"merchantJoinedDate,omitempty"`
	Amount             atomefin.Amount `json:"amount"`
	PeriodType         *int            `json:"periodType,omitempty"`
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
	SubOrderID       string                    `json:"subOrderId,omitempty"`
	MerchantID       string                    `json:"merchantId,omitempty"`
	OrderAmount      atomefin.Amount           `json:"orderAmount"`
	InstallmentPlans []CommerceInstallmentPlan `json:"installmentPlans"`
}

// PaymentPlanDataExtendInfo carries aggregate order-level installment plans.
type PaymentPlanDataExtendInfo struct {
	// SessionID is the Atome-generated checkout session token. Pass it
	// back on POST /auth via the `sessionid` header (max 64 chars,
	// valid 2 hours).
	SessionID string `json:"sessionId,omitempty"`
	// RiplayInfoList carries per-tenor disclosure URLs (RI play
	// requirement, added 2026-08-07).
	RiplayInfoList           []RiplayInfo              `json:"riplayInfoList,omitempty"`
	SumOrderInstallmentPlans *SumOrderInstallmentPlans `json:"sumOrderInstallmentPlans,omitempty"`
}

// RiplayInfo is one (tenor, URL) disclosure row on the
// /payment-plan response.
type RiplayInfo struct {
	TotalTenor int    `json:"totalTenor"`
	URL        string `json:"url"`
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
	SubOrderID      string          `json:"subOrderId,omitempty"`
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
	if req.ExtendInfo == nil {
		return &atomefin.ValidationError{Field: "extendInfo", Message: "required (carries orderType)"}
	}
	if !req.ExtendInfo.OrderType.IsValid() {
		return &atomefin.ValidationError{Field: "extendInfo.orderType", Message: validOrderTypesMsg}
	}
	if err := validatePlanSubOrders(req.ExtendInfo.OrderType, req.SubOrders); err != nil {
		return err
	}
	if sumPlanSubOrderAmount(req.SubOrders) != req.TotalAmount {
		return &atomefin.ValidationError{
			Field:   "totalAmount",
			Message: "must equal sum of subOrders[].amount",
		}
	}
	if err := validatePlanMainOrderExtendInfos(req.ExtendInfo.OrderType, req.ExtendInfo.MainOrderExtendInfos); err != nil {
		return err
	}
	if len(req.Sessionid) > 64 {
		return &atomefin.ValidationError{Field: "sessionid", Message: "exceeds spec maxlength 64"}
	}
	// Per 2026-08-11 spec: /payment-plan no longer requires the
	// `sessionid` request header — the session is returned on the
	// response (data.extendInfo.sessionId) and consumed by /auth.
	return nil
}
