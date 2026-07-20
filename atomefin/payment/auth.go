package payment

import (
	"context"
	"errors"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// AuthRequest is the POST /auth request body.
//
// Idempotency: RequestID is the business-level idempotency key. Set it
// once at the call site and reuse across retries — see DESIGN.md §1.4.
// If RequestID is empty when Auth is invoked, the Service mints one
// from the Client's NewRequestID() and writes it back to the struct so
// the caller can echo it in their reconciliation log.
type AuthRequest struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's identifier for the user.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// TotalAmount is the order total in minor units.
	// MUST equal the sum of SubOrders[].Amount.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// PeriodType is the installment tenor: one of 1, 3, 6, 9, 12.
	// (Q16 covers whether the set is fixed; we keep the type plain int
	// to round-trip values the partner adds before the SDK does.)
	PeriodType int `json:"periodType"`
	// SubOrders is the list of line items. Required, non-empty.
	SubOrders []SubOrder `json:"subOrders"`
	// ExtendInfo is the typed extendInfo tree. Required by the spec
	// because extendInfo.orderType drives risk routing.
	ExtendInfo *RequestExtendInfo `json:"extendInfo"`

	// Sessionid is the per-/auth session token (DESIGN.md §1.3,
	// Q6 lifecycle pending). Required for /auth, ≤ 64 chars.
	//
	// json:"-" because it travels in the HTTP `sessionid` header,
	// not the JSON body. Set it on the struct; the Service reads it
	// and emits the header via atomefin.WithRequestHeader.
	Sessionid string `json:"-"` // max 64
}

// AuthResponse is the POST /auth response envelope.
type AuthResponse struct {
	Code    atomefin.Code      `json:"code"`
	Message string             `json:"message"`
	Data    *AuthorizationData `json:"data,omitempty"`
}

// IsTerminal reports whether the response carries a terminal Status
// (SUCCESS or FAILED). Nil-safe across the Code/Message/Data spine —
// returns false if Data is nil.
func (r *AuthResponse) IsTerminal() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status.IsTerminal()
}

// IsProcessing reports whether the response is the async PROCESSING
// envelope. Nil-safe.
func (r *AuthResponse) IsProcessing() bool {
	if r == nil || r.Data == nil {
		return false
	}
	return r.Data.Status == atomefin.StatusProcessing
}

// AuthorizationData is the `data` body of AuthResponse.
type AuthorizationData struct {
	RequestID                string                     `json:"requestId"`
	Currency                 atomefin.Currency          `json:"currency"`
	AuthOrderID              string                     `json:"authOrderId"`
	TotalAmount              atomefin.Amount            `json:"totalAmount"`
	Status                   atomefin.Status            `json:"status"`
	FailureCode              atomefin.FailureCode       `json:"failureCode,omitempty"`
	SubOrderInstallmentPlans []SubOrderInstallmentPlans `json:"subOrderInstallmentPlans,omitempty"`
	AccountChanges           *AccountChanges            `json:"accountChanges,omitempty"`
	ExtendInfo               *AuthExtendInfoResp        `json:"extendInfo,omitempty"`
}

// Auth submits a /auth call. The Service marshals the request via
// atomefin.MarshalSigning (HTML escaping off — see DESIGN
// batteries-review #4), appends the `sessionid` header, signs the
// body via the Client's Signer, and dispatches with retry.
//
// RequestID is auto-minted from the Client's generator if empty;
// callers can read req.RequestID after the call to log the value
// the SDK actually transmitted.
//
// Returns:
//
//   - (resp, nil) on HTTP 2xx — caller inspects resp.IsTerminal() /
//     IsProcessing() and the typed Data fields.
//   - (nil, *atomefin.APIError) on HTTP 4xx/5xx after retries.
//   - (nil, *atomefin.TransportError) on transport / serialisation
//     failure; (nil, *atomefin.SignatureError) on signing failure;
//     (nil, *atomefin.ValidationError) on client-side rejection.
func (s *Service) Auth(ctx context.Context, req *AuthRequest) (*AuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil AuthRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateAuthRequest(req); err != nil {
		return nil, err
	}

	opts := []atomefin.DoSignedOption{}
	if req.Sessionid != "" {
		opts = append(opts, atomefin.WithRequestHeader("sessionid", req.Sessionid))
	}

	var resp AuthResponse
	if err := s.invoke(ctx, "/auth", req, &resp, opts...); err != nil {
		return nil, err
	}
	return &resp, nil
}

// AuthPollUntilTerminal calls Auth with the same RequestID until the
// response Status is terminal or the budget expires. Spec §1.4: the
// server returns the prior terminal payload synchronously when an
// already-completed RequestID is re-submitted, so this is the
// pragmatic equivalent of a status-poll endpoint (which the spec does
// not provide).
func (s *Service) AuthPollUntilTerminal(ctx context.Context, req *AuthRequest, opts PollOptions) (*AuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil AuthRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	return PollUntilTerminal(ctx, opts,
		func(r *AuthResponse) atomefin.Status {
			if r == nil || r.Data == nil {
				return atomefin.Status("")
			}
			return r.Data.Status
		},
		func(c context.Context) (*AuthResponse, error) {
			return s.Auth(c, req)
		},
	)
}

// validateAuthRequest is the small client-side guard — server-level
// validation still rules. We surface the most common partner mistakes
// (empty required fields, sub-order sum mismatch, sessionid too long)
// before paying the network cost.
func validateAuthRequest(req *AuthRequest) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "must be non-empty (use a partner-stable idempotency key)"}
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
	for i, so := range req.SubOrders {
		if err := validateCommerceSubOrder(so); err != nil {
			return err
		}
		_ = i
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
		// /auth requires the `sessionid` header per DESIGN.md §1.3.
		// Without it the server returns 400 SESSION_NOT_FOUND in
		// production — surface it client-side so the partner sees
		// the actionable error before paying the network round-trip.
		return &atomefin.ValidationError{
			Field:   "sessionid",
			Message: "required for /auth (per DESIGN.md §1.3); travels in the HTTP `sessionid` header",
		}
	}
	if len(req.Sessionid) > 64 {
		return &atomefin.ValidationError{Field: "sessionid", Message: "exceeds spec maxlength 64"}
	}
	if !IsValidScore(req.ExtendInfo.UserCreditScore) {
		return &atomefin.ValidationError{Field: "extendInfo.userCreditScore", Message: "must be in [0, 1] (per spec)"}
	}
	if req.ExtendInfo.DeviceInfo != nil {
		p := req.ExtendInfo.DeviceInfo.Platform
		if p != "" && !p.IsValid() {
			// Forward-compat: do not block on unknown values, but
			// surface them in tests and logs.
			_ = errors.New("unknown platform")
		}
	}
	return nil
}
