// Package repayment's Service surface — POST /repayment-request +
// GET /repayment-result — plus the polling helper that mirrors
// payment.AuthPollUntilTerminal for the PROCESSING flow.
package repayment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// Service is the outbound repayment client. Construct via
// repayment.New(c). Immutable after construction; safe for concurrent
// use across goroutines (the underlying *atomefin.Client and its
// Signer are concurrent-safe per their package docs).
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil when
// passed a nil Client so the caller fails fast at the dial site
// rather than panicking inside Repayment / QueryRepayment.
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
			Message: "nil *repayment.Service (likely from repayment.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*repayment.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// Repayment submits POST /repayment-request. Auto-mints the
// RequestID via the Client's generator when empty; partners that want
// their own idempotency-key prefix should set req.RequestID
// explicitly.
func (s *Service) Repayment(ctx context.Context, req *RepaymentParam) (*RepaymentResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil RepaymentParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateRepayment(req); err != nil {
		return nil, err
	}

	body, err := atomefin.MarshalSigning(req)
	if err != nil {
		// Marshal failure is non-temporary — classify as signature-
		// class so callers errors.As(*SignatureError) consistently.
		return nil, &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, "/repayment-request", body)
	if err != nil {
		return nil, err
	}
	var out RepaymentResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/repayment-request",
			Err: fmt.Errorf("decode /repayment-request response: %w", uerr),
		}
	}
	return &out, nil
}

// QueryRepayment retrieves the current state of a prior /repayment-
// request keyed by `requestID` + `externalReferenceUID`. Returns the
// same envelope shape as Repayment — partners can substitute
// QueryRepayment into a polling loop in place of the PROCESSING
// webhook listener.
//
// Spec endpoint: GET /repayment-result?requestId=<id>&externalReferenceUid=<uid>
//
// Both query parameters are spec-required; passing an empty string
// for either rejects locally without a network round-trip.
func (s *Service) QueryRepayment(ctx context.Context, requestID, externalReferenceUID string) (*RepaymentResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if requestID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "required (the requestId from the prior /repayment-request submission)",
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
			Message: "required",
		}
	}
	if len(externalReferenceUID) > 64 {
		return nil, &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "exceeds spec maxlength 64",
		}
	}

	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
	resp, err := s.c.DoSignedGET(ctx, "/repayment-result", q)
	if err != nil {
		return nil, err
	}
	var out RepaymentResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/repayment-result",
			Err: fmt.Errorf("decode /repayment-result response: %w", uerr),
		}
	}
	return &out, nil
}

// RepaymentPollUntilTerminal calls Repayment with the same RequestID
// until the response Status is terminal (SUCCESS / FAILED), the
// parent ctx expires, or PollOptions.MaxWait elapses.
//
// Reuses payment.PollUntilTerminal so the backoff semantics are
// identical to AuthPollUntilTerminal / CapturePollUntilTerminal /
// RefundPollUntilTerminal — partners using more than one Service get
// one consistent polling shape.
//
// NOTE: this re-POSTs /repayment-request (idempotent on RequestID).
// For GET-based polling, drive QueryRepayment from your own loop —
// partners that prefer GET-only polling can wrap QueryRepayment with
// payment.PollUntilTerminal directly.
func (s *Service) RepaymentPollUntilTerminal(ctx context.Context, req *RepaymentParam, opts payment.PollOptions) (*RepaymentResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil RepaymentParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	return payment.PollUntilTerminal(ctx, opts,
		func(r *RepaymentResponse) atomefin.Status {
			if r == nil || r.Data == nil {
				return atomefin.Status("")
			}
			return r.Data.Status
		},
		func(c context.Context) (*RepaymentResponse, error) {
			return s.Repayment(c, req)
		},
	)
}

// validateRepayment is the small client-side guard. Server-level
// validation still rules; this lets partners surface common mistakes
// locally (empty required fields, requestId length) without a
// network round-trip.
func validateRepayment(req *RepaymentParam) error {
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
	if req.RepaymentAmount <= 0 {
		return &atomefin.ValidationError{Field: "repaymentAmount", Message: "must be > 0 (minor units)"}
	}
	if req.RepaymentApplyTime <= 0 {
		return &atomefin.ValidationError{
			Field:   "repaymentApplyTime",
			Message: "must be > 0 (Unix timestamp in milliseconds; TZ unspecified — Q11)",
		}
	}
	return nil
}
