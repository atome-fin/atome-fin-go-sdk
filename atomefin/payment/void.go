package payment

import (
	"context"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// VoidAuthRequest is the POST /voidAuth body.
//
// Per spec, the void request
// carries EXACTLY three fields — there is no `extendInfo` here.
type VoidAuthRequest struct {
	// RequestID is partner-generated; max 64 chars. New per void call.
	RequestID string `json:"requestId"` // max 64
	// ExternalReferenceUID is the partner's user identifier; matches
	// the /auth that originated AuthOrderID.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// AuthOrderID identifies the void target.
	AuthOrderID string `json:"authOrderId"`
}

// VoidAuthResponse is the POST /voidAuth envelope.
type VoidAuthResponse struct {
	Code    atomefin.Code   `json:"code"`
	Message string          `json:"message"`
	Data    *VoidResultData `json:"data,omitempty"`
}

// IsTerminal reports whether the response is a successful void
// acknowledgement. /voidAuth data no longer carries a status field.
func (r *VoidAuthResponse) IsTerminal() bool {
	return r != nil && r.Code == atomefin.CodeSuccess && r.Data != nil
}

// VoidResultData is the `data` body of VoidAuthResponse.
//
// Per spec re-read: VoidResultData carries no extendInfo. There is no
// VoidExtendInfoResp type.
type VoidResultData struct {
	RequestID            string          `json:"requestId"`
	ExternalReferenceUID string          `json:"externalReferenceUid"`
	AuthOrderID          string          `json:"authOrderId"`
	AccountChanges       *AccountChanges `json:"accountChanges,omitempty"`
}

// VoidAuth submits a /voidAuth call.
func (s *Service) VoidAuth(ctx context.Context, req *VoidAuthRequest) (*VoidAuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil VoidAuthRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateVoidAuthRequest(req); err != nil {
		return nil, err
	}
	var resp VoidAuthResponse
	if err := s.invoke(ctx, "/voidAuth", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func validateVoidAuthRequest(req *VoidAuthRequest) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > 64 {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if req.AuthOrderID == "" {
		return &atomefin.ValidationError{Field: "authOrderId", Message: "required"}
	}
	return nil
}
