package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// PaymentPreCheckRequest is the POST /payment-precheck body.
// Run this BEFORE /auth to confirm a proposed transaction is eligible.
type PaymentPreCheckRequest struct {
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// TotalAmount in minor units; Σ(SubOrders[].Amount) must equal.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// SubOrders enumerates the cart contents to evaluate (PlanSubOrder shape).
	SubOrders []PlanSubOrder `json:"subOrders"`
	// ExtendInfo carries checkout extension fields. Required by the
	// spec because extendInfo.orderType drives risk routing.
	ExtendInfo *PreCheckExtendInfo `json:"extendInfo"`

	// RequestID is client-side only for idempotency logging; not in the
	// spec body. Populated by the SDK when empty before the network call.
	RequestID string `json:"-"`
}

// PreCheckExtendInfo is the extendInfo bag on /payment-precheck.
type PreCheckExtendInfo struct {
	OrderType PaymentOrderType `json:"orderType"`
}

// PaymentPreCheckSubOrder is a backward-compatible alias for PlanSubOrder.
type PaymentPreCheckSubOrder = PlanSubOrder

// PaymentPreCheckResponse is the POST /payment-precheck envelope.
type PaymentPreCheckResponse struct {
	Code    atomefin.Code        `json:"code"`
	Message string               `json:"message"`
	Data    *PaymentPreCheckData `json:"data,omitempty"`
}

// IsEligible reports whether the pre-check approved the proposed
// transaction. Eligibility is indicated by top-level code SUCCESS.
func (r *PaymentPreCheckResponse) IsEligible() bool {
	if r == nil {
		return false
	}
	return r.Code == atomefin.CodeSuccess
}

// PaymentPreCheckData is the `data` body of PaymentPreCheckResponse.
type PaymentPreCheckData struct {
	// AvailableCredit is the user's available credit in IDR minor units.
	AvailableCredit atomefin.Amount `json:"availableCredit"`
	// ExtendInfo carries optional fields such as reapplyTime on RISK_REJECT.
	ExtendInfo *PaymentPreCheckDataExtendInfo `json:"extendInfo,omitempty"`
}

// PaymentPreCheckDataExtendInfo is the data.extendInfo bag.
type PaymentPreCheckDataExtendInfo struct {
	// ReapplyTime is Unix-ms when the user may retry after RISK_REJECT.
	ReapplyTime int64 `json:"reapplyTime,omitempty"`
}

// PaymentPreCheck submits POST /payment-precheck.
func (s *Service) PaymentPreCheck(ctx context.Context, req *PaymentPreCheckRequest) (*PaymentPreCheckResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil PaymentPreCheckRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validatePaymentPreCheckRequest(req); err != nil {
		return nil, err
	}

	body, err := atomefin.MarshalSigning(req)
	if err != nil {
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/payment-precheck", body)
	if err != nil {
		return nil, err
	}
	var out PaymentPreCheckResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/payment-precheck",
			Err: fmt.Errorf("decode /payment-precheck response: %w", uerr),
		}
	}
	return &out, nil
}

func validatePaymentPreCheckRequest(req *PaymentPreCheckRequest) error {
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
	return nil
}
