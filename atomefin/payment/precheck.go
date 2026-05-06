package payment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// PaymentPreCheckRequest is the POST /payment-precheck body. v0.2
// chunk #7 (pre-checkout). Run this BEFORE /auth to confirm a
// proposed transaction is eligible (sufficient credit, account in
// good standing, risk-engine approval).
//
// Mirrors AuthRequest in shape — same idempotency-keyed
// `requestId`, `externalReferenceUid`, sub-order list — but does
// NOT freeze credit. The server returns an Eligible flag and an
// optional decline reason; callers proceed to /auth on Eligible
// or surface the reason to the user.
type PaymentPreCheckRequest struct {
	// RequestID is partner-generated; max 64 chars.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's user identifier.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// TotalAmount in minor units; Σ(SubOrders[].Amount) must equal.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// Currency for the proposed transaction. v0.1.1 enum-locked to IDR.
	Currency atomefin.Currency `json:"currency"`
	// SubOrders enumerates the cart contents to evaluate.
	SubOrders []PaymentPreCheckSubOrder `json:"subOrders"`
	// ExtendInfo carries the optional request-side extendInfo tree
	// (re-uses the payment-package RequestExtendInfo so partners
	// don't carry per-endpoint shapes).
	ExtendInfo *RequestExtendInfo `json:"extendInfo,omitempty"`
}

// PaymentPreCheckSubOrder is one cart line on a pre-check request.
// Smaller than the full SubOrder used by /auth — pre-check only
// needs the amount-bearing fields.
type PaymentPreCheckSubOrder struct {
	SubOrderID string          `json:"subOrderId"`
	Amount     atomefin.Amount `json:"amount"`
	Quantity   int             `json:"quantity"`
	// SkuName is the display label (optional, useful for risk-
	// engine signals).
	SkuName string `json:"skuName,omitempty"`
}

// PaymentPreCheckResponse is the POST /payment-precheck envelope.
type PaymentPreCheckResponse struct {
	Code    atomefin.Code        `json:"code"`
	Message string               `json:"message"`
	Data    *PaymentPreCheckData `json:"data,omitempty"`
}

// IsEligible reports whether the pre-check approved the proposed
// transaction. Nil-safe across the Code/Message/Data spine.
func (r *PaymentPreCheckResponse) IsEligible() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Eligible
}

// PaymentPreCheckData is the `data` body of PaymentPreCheckResponse.
//
// AvailableCredit reflects the user's available credit at decision
// time — useful for partners that want to surface "you have X
// remaining" to the user. AccountChanges echoes any state change
// the pre-check itself triggered (typically empty since pre-check
// doesn't move money).
type PaymentPreCheckData struct {
	RequestID            string `json:"requestId"`
	ExternalReferenceUID string `json:"externalReferenceUid,omitempty"`
	// Eligible is true when the proposed transaction passes risk +
	// credit checks. False when the transaction must NOT proceed
	// to /auth.
	Eligible bool `json:"eligible"`
	// AvailableCredit is the user's available credit at decision
	// time, in the request's Currency / minor units. Optional —
	// some servers omit this for risk-decline cases.
	AvailableCredit atomefin.Amount `json:"availableCredit,omitempty"`
	// DeniedReason is set when Eligible == false. Free-form
	// per-server (e.g. "credit-limit-insufficient", "user-blocked",
	// "risk-reject"); partners should treat this as informational
	// and rely on the boolean for branching.
	DeniedReason string `json:"deniedReason,omitempty"`
	// AccountChanges echoes any account-state delta the pre-check
	// caused (typically empty since pre-check doesn't move money,
	// but populated if e.g. a risk-engine flagged the account into
	// a temp-block).
	AccountChanges *AccountChanges `json:"accountChanges,omitempty"`
}

// PaymentPreCheck submits POST /payment-precheck.
//
// Auto-mints RequestID via the Client's generator when empty. The
// body is marshalled via atomefin.MarshalSigning (HTML-escape OFF)
// so the bytes signed equal the bytes transmitted byte-for-byte.
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

// validatePaymentPreCheckRequest is the small client-side guard,
// mirroring AuthRequest's shape rules.
func validatePaymentPreCheckRequest(req *PaymentPreCheckRequest) error {
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
