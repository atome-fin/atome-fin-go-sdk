package payment

import (
	"context"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// RiplayRequest is the POST /riplay body. Call after /payment-plan
// with the returned sessionId, the same externalReferenceUid, and
// the user-selected tenor (same values as periodType /
// riplayInfoList[].totalTenor).
type RiplayRequest struct {
	// SessionID is the Atome-issued checkout session from
	// POST /payment-plan data.extendInfo.sessionId. Max 64 chars,
	// valid 2 hours.
	SessionID string `json:"sessionId"`
	// ExternalReferenceUID must match the /payment-plan request.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// Tenor is the selected installment tenor: 1, 3, 6, 9, or 12.
	Tenor int `json:"tenor"`
}

// RiplayResponse is the POST /riplay envelope.
type RiplayResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message"`
	Data    *RiplayData   `json:"data,omitempty"`
}

// RiplayData is the `data` body of RiplayResponse.
type RiplayData struct {
	Tenor int    `json:"tenor"`
	URL   string `json:"url"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *RiplayResponse) IsSuccess() bool {
	return r != nil && r.Code == atomefin.CodeSuccess
}

// Riplay submits POST /riplay and returns the RIPLAY contract URL
// for the selected tenor. Session expiry or an unknown session
// surfaces as 400 SESSION_NOT_FOUND; a tenor with no document
// surfaces as 400 NOT_FOUND.
func (s *Service) Riplay(ctx context.Context, req *RiplayRequest) (*RiplayResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil RiplayRequest"}
	}
	if err := validateRiplayRequest(req); err != nil {
		return nil, err
	}
	var resp RiplayResponse
	if err := s.invoke(ctx, "/riplay", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func validateRiplayRequest(req *RiplayRequest) error {
	if req.SessionID == "" {
		return &atomefin.ValidationError{Field: "sessionId", Message: "required (from /payment-plan data.extendInfo.sessionId)"}
	}
	if len(req.SessionID) > 64 {
		return &atomefin.ValidationError{Field: "sessionId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > 64 {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if !isValidRiplayTenor(req.Tenor) {
		return &atomefin.ValidationError{Field: "tenor", Message: "must be one of 1, 3, 6, 9, 12"}
	}
	return nil
}

func isValidRiplayTenor(tenor int) bool {
	switch tenor {
	case 1, 3, 6, 9, 12:
		return true
	}
	return false
}
