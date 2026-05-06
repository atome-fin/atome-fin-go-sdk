package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Query endpoints (GET) — added in v0.2 for the polling alternative
// to the PROCESSING webhook flow. Each method retrieves the current
// state of a prior outbound submission keyed by the partner-supplied
// `requestId` AND `externalReferenceUid` (both are spec-required).
// Responses use the SAME typed envelopes as the POST counterparts
// (the spec wire shape is identical), so partner code can switch
// between webhook-driven and poll-driven completion detection
// without learning a parallel response schema.
//
// Spec quote (DESIGN.md §1.3 verbatim):
//
//	"GET: Sign the request parameters which parameter names are
//	 sorted in alphabetical natural order"
//
// Wire ≡ canonical: Client.DoSignedGET sets RawQuery to the same
// bytes it signs (sign.CanonicalQuery output, RFC 3986 percent-
// encoded), so server-side reconstruction verifies byte-for-byte.
// Alphabetical sort places `externalReferenceUid` before `requestId`.

// QueryAuth retrieves the current state of a prior /auth submission
// keyed by `requestID` + `externalReferenceUID`. Returns the same
// envelope shape as (*Service).Auth — partners can substitute
// QueryAuth into a polling loop in place of the PROCESSING webhook
// listener.
//
// Spec endpoint: GET /query-auth?externalReferenceUid=<x>&requestId=<y>
func (s *Service) QueryAuth(ctx context.Context, requestID, externalReferenceUID string) (*AuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if err := validateQueryParams(requestID, externalReferenceUID); err != nil {
		return nil, err
	}
	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
	var resp AuthResponse
	if err := s.invokeGET(ctx, "/query-auth", q, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// QueryCapture retrieves the current state of a prior /capture
// submission keyed by `requestID` + `externalReferenceUID`. Same
// envelope as (*Service).Capture.
//
// Spec endpoint: GET /query-capture?externalReferenceUid=<x>&requestId=<y>
func (s *Service) QueryCapture(ctx context.Context, requestID, externalReferenceUID string) (*CaptureResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if err := validateQueryParams(requestID, externalReferenceUID); err != nil {
		return nil, err
	}
	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
	var resp CaptureResponse
	if err := s.invokeGET(ctx, "/query-capture", q, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// QueryVoidAuth retrieves the current state of a prior /voidAuth
// submission keyed by `requestID` + `externalReferenceUID`. Same
// envelope as (*Service).VoidAuth.
//
// Spec endpoint: GET /query-voidAuth?externalReferenceUid=<x>&requestId=<y>
func (s *Service) QueryVoidAuth(ctx context.Context, requestID, externalReferenceUID string) (*VoidAuthResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if err := validateQueryParams(requestID, externalReferenceUID); err != nil {
		return nil, err
	}
	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
	var resp VoidAuthResponse
	if err := s.invokeGET(ctx, "/query-voidAuth", q, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// validateQueryParams enforces the spec's `requestId` (≤64 chars,
// non-empty) and `externalReferenceUid` (non-empty) constraints.
// Both are spec-required on the Query* GETs.
func validateQueryParams(requestID, externalReferenceUID string) error {
	if requestID == "" {
		return &atomefin.ValidationError{
			Field:   "requestId",
			Message: "required (the requestId from the prior outbound submission)",
		}
	}
	if len(requestID) > 64 {
		return &atomefin.ValidationError{
			Field:   "requestId",
			Message: "exceeds spec maxlength 64",
		}
	}
	if externalReferenceUID == "" {
		return &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "required (the partner-side user/account identifier from the prior outbound submission)",
		}
	}
	return nil
}

// invokeGET is the GET counterpart of invoke. Calls
// Client.DoSignedGET (which signs the canonical query, sets the wire
// query to the same bytes, and runs the standard retry/observer
// pipeline) and decodes the 2xx body into out.
func (s *Service) invokeGET(ctx context.Context, op string, q url.Values, out any) error {
	if s == nil || s.c == nil {
		return errors.New("atome-fin/payment: nil Service or Client")
	}
	resp, err := s.c.DoSignedGET(ctx, op, q)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if uerr := json.Unmarshal(resp.Body, out); uerr != nil {
		return &atomefin.TransportError{
			Op:  "unmarshal",
			URL: op,
			Err: fmt.Errorf("decode %s response: %w", op, uerr),
		}
	}
	return nil
}
