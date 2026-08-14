// Package refund's Service surface — POST /refund + GET
// /query-refund — plus the polling helper that mirrors
// payment.AuthPollUntilTerminal for the PROCESSING flow.
package refund

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// Service is the outbound refund client. Construct via refund.New(c).
// Immutable after construction; safe for concurrent use across
// goroutines (the underlying *atomefin.Client and its Signer are
// concurrent-safe per their package docs).
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil when
// passed a nil Client so the caller fails fast at the dial site
// rather than panicking inside Refund / QueryRefund.
func New(c *atomefin.Client) *Service {
	if c == nil {
		return nil
	}
	return &Service{c: c}
}

// Client exposes the underlying *atomefin.Client. Nil-safe.
func (s *Service) Client() *atomefin.Client {
	if s == nil {
		return nil
	}
	return s.c
}

// checkConfigured guards against nil receiver / nil Client. Mirrors
// payment.Service.checkConfigured. Returns *atomefin.ValidationError
// so callers can errors.As uniformly.
func (s *Service) checkConfigured() error {
	if s == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "nil *refund.Service (likely from refund.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*refund.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// Refund submits POST /refund. Auto-mints the RequestID via the
// Client's generator when empty; partners that want their own
// idempotency-key prefix should set req.RequestID explicitly.
func (s *Service) Refund(ctx context.Context, req *RefundParam) (*RefundResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil RefundParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateRefund(req); err != nil {
		return nil, err
	}

	body, err := atomefin.MarshalSigning(req)
	if err != nil {
		// Marshal failure is non-temporary — classify as signature-
		// class so callers errors.As(*SignatureError) consistently.
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/refund", body)
	if err != nil {
		return nil, err
	}
	var out RefundResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/refund",
			Err: fmt.Errorf("decode /refund response: %w", uerr),
		}
	}
	return &out, nil
}

// QueryRefund retrieves the current state of a prior /refund keyed
// by `requestID` + `externalReferenceUID` (both spec-required).
// Returns the same envelope shape as Refund — partners can
// substitute QueryRefund into a polling loop in place of the
// PROCESSING webhook listener.
//
// Spec endpoint: GET /query-refund?externalReferenceUid=<x>&requestId=<y>
// (alphabetical sort places externalReferenceUid before requestId in
// the canonical query.)
func (s *Service) QueryRefund(ctx context.Context, requestID, externalReferenceUID string) (*RefundResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if requestID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "required (the requestId from the prior /refund submission)",
		}
	}
	if len(requestID) > 64 {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "exceeds spec maxlength 64",
		}
	}
	if externalReferenceUID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "required (the partner-side user/account identifier from the prior /refund submission)",
		}
	}

	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
	resp, err := s.c.DoSignedGET(ctx, "/query-refund", q)
	if err != nil {
		return nil, err
	}
	var out RefundResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/query-refund",
			Err: fmt.Errorf("decode /query-refund response: %w", uerr),
		}
	}
	return &out, nil
}

// RefundPollUntilTerminal calls Refund with the same RequestID until
// the response Status is terminal (SUCCESS / FAILED), the parent
// ctx expires, or PollOptions.MaxWait elapses.
//
// Reuses payment.PollUntilTerminal so the backoff semantics are
// identical to AuthPollUntilTerminal / CapturePollUntilTerminal —
// partners using more than one Service get one consistent polling
// shape.
func (s *Service) RefundPollUntilTerminal(ctx context.Context, req *RefundParam, opts payment.PollOptions) (*RefundResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil RefundParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	return payment.PollUntilTerminal(ctx, opts,
		func(r *RefundResponse) atomefin.Status {
			if r == nil || r.Data == nil {
				return atomefin.Status("")
			}
			return r.Data.Status
		},
		func(c context.Context) (*RefundResponse, error) {
			return s.Refund(c, req)
		},
	)
}

// validateRefund is the small client-side guard. Server-level
// validation still rules; this lets partners surface common
// mistakes locally (empty required fields, sub-order sum mismatch,
// requestId length) without a network round-trip.
//
// Q25 (partner-pending): the SDK requires
// `refundAmount == Σ subOrderRefunds[].refundAmount`, mirroring the
// capture sum-rule. The 2026-05-06 spec snapshot is ambiguous on
// whether partial refunds (refundAmount < authAmount) are
// permitted; the conservative validator avoids silent data loss
// when the spec relaxes. See refund/doc.go.
func validateRefund(req *RefundParam) error {
	if req.RequestID == "" {
		return &atomefin.ValidationError{Field: "requestId", Message: "required"}
	}
	if len(req.RequestID) > 64 {
		return &atomefin.ValidationError{Field: "requestId", Message: "exceeds spec maxlength 64"}
	}
	if req.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if req.CaptureRequestID == "" {
		return &atomefin.ValidationError{Field: "captureRequestId", Message: "required (the requestId of the prior /capture call)"}
	}
	if req.RefundAmount <= 0 {
		return &atomefin.ValidationError{Field: "refundAmount", Message: "must be > 0 (minor units)"}
	}
	if len(req.SubOrders) == 0 {
		return &atomefin.ValidationError{Field: "subOrders", Message: "must be non-empty"}
	}
	var sum atomefin.Amount
	for _, so := range req.SubOrders {
		if so.Amount <= 0 {
			return &atomefin.ValidationError{Field: "subOrders[].amount", Message: "must be > 0 (minor units)"}
		}
		sum += so.Amount
	}
	if sum != req.RefundAmount {
		return &atomefin.ValidationError{
			Field:   "refundAmount",
			Message: "must equal sum of subOrders[].amount (Q25 conservative — partner-pending)",
		}
	}
	if req.ExtendInfo == nil || req.ExtendInfo.OrderType == "" {
		return &atomefin.ValidationError{Field: "extendInfo.orderType", Message: "required"}
	}
	switch req.ExtendInfo.OrderType {
	case "TRANSPORT", "GRAB_FOOD":
		if len(req.SubOrders) != 1 {
			return &atomefin.ValidationError{
				Field:   "subOrders",
				Message: "must contain exactly one entry for " + req.ExtendInfo.OrderType,
			}
		}
	case "GRAB_MART":
		for _, so := range req.SubOrders {
			if so.MerchantID == "" {
				return &atomefin.ValidationError{Field: "subOrders[].merchantId", Message: "required for GRAB_MART"}
			}
		}
	default:
		return &atomefin.ValidationError{
			Field:   "extendInfo.orderType",
			Message: "must be one of TRANSPORT | GRAB_FOOD | GRAB_MART",
		}
	}
	return nil
}
