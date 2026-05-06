package payment

import (
	"context"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// CaptureRequest is the POST /capture body.
//
// Per DESIGN.md §1.4, /capture must mirror the prior /auth: same
// AuthOrderID, same TotalAmount, same PeriodType, byte-equal SubOrders.
// The Service does NOT enforce this — partners typically have only the
// /auth response in hand at capture time, and over-eager validation
// would block legitimate retries. Validation focuses on the same set
// of basic shape rules as AuthRequest.
type CaptureRequest struct {
	// RequestID is partner-generated; max 64 chars. New per capture call
	// (idempotency key).
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's identifier for the user.
	// Required by the spec (both v1-draft and v1.1 list it on
	// /capture); v0.1.0 shipped without it which made our /capture
	// non-compliant. Mirrors AuthRequest.ExternalReferenceUID.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// AuthOrderID is the value returned by /auth.
	AuthOrderID string `json:"authOrderId"`
	// TotalAmount must equal the prior /auth TotalAmount.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// PeriodType must equal the prior /auth PeriodType.
	PeriodType int `json:"periodType"`
	// SubOrders must be byte-equal to the prior /auth SubOrders.
	SubOrders []SubOrder `json:"subOrders"`
	// ExtendInfo carries the optional request-side extendInfo tree.
	ExtendInfo *RequestExtendInfo `json:"extendInfo,omitempty"`
}

// CaptureResponse is the POST /capture envelope. Same shape as
// AuthResponse plus a CaptureResultData (which embeds PaymentResult
// plus AuthOrderID — the spec).
type CaptureResponse struct {
	Code    atomefin.Code      `json:"code"`
	Message string             `json:"message"`
	Data    *CaptureResultData `json:"data,omitempty"`
}

// IsTerminal reports whether the response carries a terminal Status.
// Nil-safe.
func (r *CaptureResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// IsProcessing reports whether the response is the async PROCESSING
// envelope. Nil-safe.
func (r *CaptureResponse) IsProcessing() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status == atomefin.StatusProcessing
}

// PaymentResult is the base shape returned by /capture and the capture
// callback.
//
// CaptureResultData embeds PaymentResult plus AuthOrderID
// . We keep the embedded layout because the wire is
// `allOf(PaymentResult, {authOrderId})` — a flat object on the JSON
// side.
type PaymentResult struct {
	RequestID                string                     `json:"requestId"`
	OrderID                  string                     `json:"orderId"` // max 32; distinct from AuthOrderID (Q21)
	Currency                 atomefin.Currency          `json:"currency"`
	TotalAmount              atomefin.Amount            `json:"totalAmount"`
	Status                   atomefin.Status            `json:"status"`
	FailureCode              atomefin.FailureCode       `json:"failureCode,omitempty"`
	SubOrderInstallmentPlans []SubOrderInstallmentPlans `json:"subOrderInstallmentPlans,omitempty"`
	AccountChanges           *AccountChanges            `json:"accountChanges,omitempty"`
	ExtendInfo               *CaptureExtendInfoResp     `json:"extendInfo,omitempty"`
}

// CaptureResultData is `allOf(PaymentResult, {authOrderId})` per the
// spec — modelled here as PaymentResult embedded plus the AuthOrderID
// field, which is wire-equivalent to the spec's flat object.
type CaptureResultData struct {
	PaymentResult
	AuthOrderID string `json:"authOrderId"`
}

// Capture submits a /capture call.
func (s *Service) Capture(ctx context.Context, req *CaptureRequest) (*CaptureResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil CaptureRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateCaptureRequest(req); err != nil {
		return nil, err
	}
	var resp CaptureResponse
	if err := s.invoke(ctx, "/capture", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CapturePollUntilTerminal mirrors AuthPollUntilTerminal for /capture.
func (s *Service) CapturePollUntilTerminal(ctx context.Context, req *CaptureRequest, opts PollOptions) (*CaptureResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil CaptureRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	return PollUntilTerminal(ctx, opts,
		func(r *CaptureResponse) atomefin.Status {
			if r == nil || r.Data == nil {
				return atomefin.Status("")
			}
			return r.Data.Status
		},
		func(c context.Context) (*CaptureResponse, error) {
			return s.Capture(c, req)
		},
	)
}

func validateCaptureRequest(req *CaptureRequest) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > 64 {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required (must mirror the /auth request that produced authOrderId)"}
	}
	if req.AuthOrderID == "" {
		return &atomefin.ValidationError{Field: "authOrderId", Message: "required (from prior /auth response)"}
	}
	if req.TotalAmount <= 0 {
		return &atomefin.ValidationError{Field: "totalAmount", Message: "must be > 0 (minor units)"}
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
		sum += so.Amount
	}
	if sum != req.TotalAmount {
		return &atomefin.ValidationError{Field: "totalAmount", Message: "must equal sum of subOrders[].amount"}
	}
	if req.ExtendInfo != nil && !IsValidScore(req.ExtendInfo.UserCreditScore) {
		return &atomefin.ValidationError{Field: "extendInfo.userCreditScore", Message: "must be in [0, 1]"}
	}
	return nil
}
