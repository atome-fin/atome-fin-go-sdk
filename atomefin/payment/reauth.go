package payment

import (
	"context"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// ReAuthRequest is the POST /reAuth body. Submit it after a successful
// /voidAuth to create a new authorization with updated SKU or amount
// while reusing the original rates.
type ReAuthRequest struct {
	// RequestID is partner-generated; max 64 chars. Idempotency key.
	RequestID string `json:"requestId"`
	// ExternalReferenceUID is the partner's identifier for the user.
	ExternalReferenceUID string `json:"externalReferenceUid"`
	// AuthOrderID is the original authorization that was voided.
	AuthOrderID string `json:"authOrderId"`
	// TotalAmount is the new order total in minor units.
	TotalAmount atomefin.Amount `json:"totalAmount"`
	// PeriodType must match the original /auth period type.
	PeriodType int `json:"periodType"`
	// SubOrders is the new merchant-dimension sub-order set. Merchant
	// IDs must match the original authorization.
	SubOrders []SubOrder `json:"subOrders"`
	// ExtendInfo carries the same required request-side tree as /auth.
	ExtendInfo *RequestExtendInfo `json:"extendInfo"`
}

// ReAuthResponse is the POST /reAuth envelope.
type ReAuthResponse struct {
	Code    atomefin.Code     `json:"code"`
	Message string            `json:"message"`
	Data    *ReAuthResultData `json:"data,omitempty"`
}

// IsTerminal reports whether the synchronous response carries a
// terminal business status. /reAuth never returns PROCESSING.
func (r *ReAuthResponse) IsTerminal() bool {
	return r != nil && r.Data != nil && r.Data.Status.IsTerminal()
}

// IsSuccess reports whether the terminal business status is SUCCESS.
func (r *ReAuthResponse) IsSuccess() bool {
	return r != nil && r.Data != nil && r.Data.Status == atomefin.StatusSuccess
}

// ReAuthResultData is the /reAuth response data. AuthOrderID is the new
// authorization to pass to /capture; OriginalAuthOrderID is the voided
// authorization supplied in the request.
type ReAuthResultData struct {
	RequestID                string                     `json:"requestId"`
	Currency                 atomefin.Currency          `json:"currency"`
	AuthOrderID              string                     `json:"authOrderId"`
	OriginalAuthOrderID      string                     `json:"originalAuthOrderId"`
	TotalAmount              atomefin.Amount            `json:"totalAmount"`
	Status                   atomefin.Status            `json:"status"`
	FailureCode              atomefin.FailureCode       `json:"failureCode,omitempty"`
	SubOrderInstallmentPlans []SubOrderInstallmentPlans `json:"subOrderInstallmentPlans,omitempty"`
	AccountChanges           *AccountChanges            `json:"accountChanges,omitempty"`
	ExtendInfo               *AuthExtendInfoResp        `json:"extendInfo,omitempty"`
}

// ReAuth submits POST /reAuth. Unlike Auth, it never sends a sessionid
// header and returns a terminal business status synchronously.
func (s *Service) ReAuth(ctx context.Context, req *ReAuthRequest) (*ReAuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil ReAuthRequest"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateReAuthRequest(req); err != nil {
		return nil, err
	}
	var resp ReAuthResponse
	if err := s.invoke(ctx, "/reAuth", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func validateReAuthRequest(req *ReAuthRequest) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > 64 {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if len(req.ExternalReferenceUID) > 64 {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "exceeds spec maxlength 64"}
	}
	if req.AuthOrderID == "" {
		return &atomefin.ValidationError{Field: "authOrderId", Message: "required (the voided original authorization)"}
	}
	if req.TotalAmount <= 0 {
		return &atomefin.ValidationError{Field: "totalAmount", Message: "must be > 0 (minor units)"}
	}
	if err := validateCheckoutAmounts(req.PeriodType, req.TotalAmount, subOrderAmounts(req.SubOrders)); err != nil {
		return err
	}
	if req.ExtendInfo == nil {
		return &atomefin.ValidationError{Field: "extendInfo", Message: "required (carries orderType)"}
	}
	if !req.ExtendInfo.OrderType.IsValid() {
		return &atomefin.ValidationError{Field: "extendInfo.orderType", Message: validOrderTypesMsg}
	}
	if err := validateAuthCaptureSubOrders(req.ExtendInfo.OrderType, req.SubOrders); err != nil {
		return err
	}
	if sumSubOrderAmount(req.SubOrders) != req.TotalAmount {
		return &atomefin.ValidationError{
			Field:   "totalAmount",
			Message: "must equal sum of subOrders[].amount",
		}
	}
	if err := validateAuthCaptureMainOrderExtendInfos(req.ExtendInfo.OrderType, req.ExtendInfo.MainOrderExtendInfos); err != nil {
		return err
	}
	return nil
}
