package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// PaymentPlanRequest is the POST /payment-plan body. v0.2 chunk #7
// (pre-checkout). Run this BEFORE /auth to retrieve the available
// installment-plan options (tenors + per-month breakdown) for a
// proposed transaction.
//
// The server returns one CommerceInstallmentPlan per offered
// `periodType`; partners surface the choice to the user, the user
// picks a tenor, the partner submits /auth with that periodType
// (and `subOrders` matching the plan input).
type PaymentPlanRequest struct {
	// RequestID is partner-generated; max 64 chars.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// TotalAmount in minor units; Σ(SubOrders[].Amount) must equal.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// Currency. v0.1.1 enum-locked to IDR.
	Currency atomefin.Currency `json:"currency"`
	// SubOrders enumerates the cart contents to plan against.
	SubOrders []PlanSubOrder `json:"subOrders"`
	// ExtendInfo (re-uses RequestExtendInfo).
	ExtendInfo *RequestExtendInfo `json:"extendInfo,omitempty"`
}

// PlanSubOrder is one cart line on a plan request. Smaller than
// the full SubOrder — plan only needs the amount-bearing fields.
type PlanSubOrder struct {
	SubOrderID string          `json:"subOrderId"`
	Amount     atomefin.Amount `json:"amount"`
	Quantity   int             `json:"quantity"`
}

// PaymentPlanResponse is the POST /payment-plan envelope.
type PaymentPlanResponse struct {
	Code    atomefin.Code    `json:"code"`
	Message string           `json:"message"`
	Data    *PaymentPlanData `json:"data,omitempty"`
}

// PaymentPlanData is the `data` body of PaymentPlanResponse.
//
// Plans is bare `json:"plans"` (no `,omitempty`) per the
// paginated-list pattern codified in v0.2 chunk #3 (bill): an
// empty plan list (e.g., user not eligible) round-trips as `[]`
// rather than disappearing — partner code reading data.plans
// shouldn't need to nil-check on a 0-tenor response.
type PaymentPlanData struct {
	RequestID            string                    `json:"requestId"`
	ExternalReferenceUID string                    `json:"externalReferenceUid,omitempty"`
	Plans                []CommerceInstallmentPlan `json:"plans"`
}

// CommerceInstallmentPlan is one tenor option returned by
// /payment-plan. Each plan describes the per-month cash-flow for a
// given `periodType` (installment count). Partners typically
// surface the list to the user; the user picks one; the partner
// uses the selected `periodType` on the subsequent /auth.
//
// Distinct from `payment.InstallmentPlan` (which is the per-sub-
// order schedule returned by /auth/capture's response data) —
// `CommerceInstallmentPlan` is the pre-checkout option-list shape.
type CommerceInstallmentPlan struct {
	// PeriodType is the installment count (1, 3, 6, 9, 12 per
	// DESIGN.md §1.5).
	PeriodType int `json:"periodType"`
	// Currency for this plan's amount fields. Always matches the
	// request's Currency at v0.1.1 (IDR-only); kept on each plan
	// row for forward-compat with multi-currency v2.
	Currency atomefin.Currency `json:"currency"`
	// TotalAmount is the total the user will pay over the plan
	// (= principal + fees + interest, summed across installments).
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// TotalFee is the partner-facing fee surface (informational —
	// fees may be split into Fee + Interest on Installments[]).
	TotalFee atomefin.Amount `json:"totalFee,omitempty"`
	// Installments is the per-month breakdown. Optional — some
	// servers return only the aggregate. When present, length
	// equals PeriodType.
	Installments []CommerceInstallmentDetail `json:"installments,omitempty"`
}

// CommerceInstallmentDetail is one row of a CommerceInstallmentPlan's
// per-month breakdown. Each row sums to `Amount = Principal + Fee +
// Interest`.
//
// Distinct from `payment.InstallmentDetail` (which is the per-
// installment row inside response.data.subOrderInstallmentPlans) —
// `CommerceInstallmentDetail` is the pre-checkout plan-row shape.
type CommerceInstallmentDetail struct {
	InstallmentID string `json:"installmentId"`
	// DueDate is yyyy-MM-dd. TZ unspecified per DESIGN §13/Q11.
	DueDate   string          `json:"dueDate"`
	Principal atomefin.Amount `json:"principal"`
	Fee       atomefin.Amount `json:"fee"`
	Interest  atomefin.Amount `json:"interest"`
	// Amount is the row total — equal to principal + fee + interest.
	Amount atomefin.Amount `json:"amount"`
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
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/payment-plan", body)
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
	if req.Currency == "" {
		return &atomefin.ValidationError{Field: "currency", Message: "required"}
	}
	if !req.Currency.IsValid() {
		return &atomefin.ValidationError{Field: "currency", Message: "must be IDR (Q10 enum-locked at v0.1.1)"}
	}
	if len(req.SubOrders) == 0 {
		return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
	}
	var sum atomefin.Amount
	for _, so := range req.SubOrders {
		if so.SubOrderID == "" {
			return &atomefin.ValidationError{Field: "subOrders[].subOrderId", Message: "required"}
		}
		if so.Amount <= 0 {
			return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
		}
		if so.Quantity < 1 {
			return &atomefin.ValidationError{Field: "subOrders[].quantity", Message: "must be >= 1"}
		}
		sum += so.Amount
	}
	if sum != req.TotalAmount {
		return &atomefin.ValidationError{
			Field:   "totalAmount",
			Message: "must equal sum of subOrders[].amount",
		}
	}
	if req.ExtendInfo != nil && !IsValidScore(req.ExtendInfo.UserCreditScore) {
		return &atomefin.ValidationError{Field: "extendInfo.userCreditScore", Message: "must be in [0, 1]"}
	}
	return nil
}
