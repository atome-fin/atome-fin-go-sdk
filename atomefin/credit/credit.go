// Package credit's Service surface — POST /credit-information,
// POST /credit-application, GET /credit-result, GET
// /credit-information-result, GET /query-balance-history, POST
// /modify-application-info, POST /close-account.
package credit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Default pagination parameters used when caller passes the zero
// value to BalanceHistory. Tuned to match the spec defaults
// (start=1, count=10).
const (
	DefaultStart = 1
	DefaultCount = 10
	// MaxCount is the spec-imposed server cap on page size.
	MaxCount = 50
)

// Service is the outbound credit-lifecycle client. Construct via
// credit.New(c). Immutable after construction; safe for concurrent
// use across goroutines (the underlying *atomefin.Client and its
// Signer are concurrent-safe per their package docs).
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil
// when passed a nil Client so the caller fails fast at the dial
// site rather than panicking inside SubmitInformation /
// SubmitApplication / etc.
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
			Message: "nil *credit.Service (likely from credit.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*credit.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// invokePost is the shared POST helper: marshal via
// atomefin.MarshalSigning (HTML-escape OFF), DoSigned, decode into
// out. Mirrors payment.Service.invoke.
func (s *Service) invokePost(ctx context.Context, op string, in any, out any) error {
	body, err := atomefin.MarshalSigning(in)
	if err != nil {
		return &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, op, body)
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

// invokeGet is the shared GET helper: build the canonical query via
// the caller-supplied url.Values, DoSignedGET, decode into out.
func (s *Service) invokeGet(ctx context.Context, op string, q url.Values, out any) error {
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

// ---------- POST /credit-information ----------

// SubmitInformation submits POST /credit-information — the
// lightweight first step of the credit flow. Returns a jumpUrl into
// Atome's KYC web flow plus the requestId the partner echoes on the
// subsequent POST /credit-application.
//
// BLOCKED in v0.2.x. The 2026-05-06 spec snapshot requires the
// /credit-information request body and `Encrypt` header to use an
// AES-ECB-PKCS5 envelope sealed with an RSA-encrypted session key
// (the partner-pending hybrid-encryption tag in DESIGN §13). v0.2.x
// does not implement that envelope, so an unblocked call would be
// rejected upstream with `400 INVALID_ENCRYPTION` on the first
// production hit. Returning a typed *ValidationError surfaces the
// gap locally with a clear migration message; the underlying
// request struct, validators, and round-trip fixtures all stay in
// place so v0.3 can re-enable the path with one edit.
func (s *Service) SubmitInformation(ctx context.Context, req *CreditInformationParam) (*CreditInformationResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	return nil, &atomefin.ValidationError{
		Field:   "/credit-information",
		Message: "requires AES+RSA hybrid encryption (Encrypt header + AES-ECB-PKCS5 body); not yet implemented in v0.2.x — lands in v0.3. See CHANGELOG.",
	}
}

// ---------- POST /credit-application ----------

// SubmitApplication submits POST /credit-application — the full KYC
// payload. Must reference a prior successful /credit-information's
// requestId via ExtendInfo.CreditInformationRequestID.
//
// BLOCKED in v0.2.x. Same hybrid-encryption requirement as
// SubmitInformation above; see that method's doc-comment for the
// rationale. Returns a typed *ValidationError; v0.3 re-enables the
// network path once the encryption envelope ships.
func (s *Service) SubmitApplication(ctx context.Context, req *CreditApplicationParam) (*CreditApplicationResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	return nil, &atomefin.ValidationError{
		Field:   "/credit-application",
		Message: "requires AES+RSA hybrid encryption (Encrypt header + AES-ECB-PKCS5 body); not yet implemented in v0.2.x — lands in v0.3. See CHANGELOG.",
	}
}

// ---------- GET /credit-result ----------

// QueryResult retrieves the current state of a credit application
// for the given user. Returns the same envelope as
// SubmitApplication; partners can substitute QueryResult into a
// polling loop in place of the credit-application callback.
//
// Spec endpoint: GET /credit-result?externalReferenceUid=<uid>
func (s *Service) QueryResult(ctx context.Context, externalReferenceUID string) (*CreditApplicationResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
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
	q := url.Values{"externalReferenceUid": []string{externalReferenceUID}}
	var out CreditApplicationResponse
	if err := s.invokeGet(ctx, "/credit-result", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- GET /credit-information-result ----------

// QueryInformationResult retrieves the current state of a
// credit-information collection for the given user + requestId.
// Returns the same envelope as the credit-information callback.
//
// Spec endpoint:
// GET /credit-information-result?externalReferenceUid=<uid>&requestId=<id>
func (s *Service) QueryInformationResult(ctx context.Context, externalReferenceUID, requestID string) (*CreditInformationCollectResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
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
	if requestID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "required (the requestId from the prior /credit-information submission)",
		}
	}
	if len(requestID) > 64 {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "exceeds spec maxlength 64",
		}
	}
	q := url.Values{
		"externalReferenceUid": []string{externalReferenceUID},
		"requestId":            []string{requestID},
	}
	var out CreditInformationCollectResponse
	if err := s.invokeGet(ctx, "/credit-information-result", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- GET /query-balance-history ----------

// BalanceHistory retrieves one page of the credit-balance change
// history. Pass nil for the default first page (Start=1, Count=10,
// no requestId filter) — but ExternalReferenceUID and Type are
// required regardless.
//
// Spec endpoint:
// GET /query-balance-history?externalReferenceUid=<uid>&type=<t>&...
func (s *Service) BalanceHistory(ctx context.Context, params *BalanceHistoryParams) (*BalanceHistoryResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil BalanceHistoryParams"}
	}
	if err := validateBalanceHistoryParams(params); err != nil {
		return nil, err
	}
	q := buildBalanceHistoryQuery(params)
	var out BalanceHistoryResponse
	if err := s.invokeGet(ctx, "/query-balance-history", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// buildBalanceHistoryQuery materialises a BalanceHistoryParams into
// a url.Values whose CanonicalQuery output is the signing canonical.
// Defaults are applied here so the wire query always carries
// start + count alongside the required externalReferenceUid + type.
func buildBalanceHistoryQuery(p *BalanceHistoryParams) url.Values {
	q := url.Values{}
	q.Set("externalReferenceUid", p.ExternalReferenceUID)
	q.Set("type", string(p.Type))
	if p.RequestID != "" {
		q.Set("requestId", p.RequestID)
	}
	start := p.Start
	if start <= 0 {
		start = DefaultStart
	}
	q.Set("start", strconv.Itoa(start))
	count := p.Count
	if count <= 0 {
		count = DefaultCount
	}
	q.Set("count", strconv.Itoa(count))
	return q
}

// ---------- POST /modify-application-info ----------

// ModifyApplicationInfo submits POST /modify-application-info to
// update the partner-side mobileNumber / email on an approved
// application. Server returns code/message only; no data body.
//
// RequestID is auto-minted from the Client's generator when empty.
func (s *Service) ModifyApplicationInfo(ctx context.Context, req *CreditApplicationChangeParam) (*ModifyApplicationInfoResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil CreditApplicationChangeParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateCreditApplicationChange(req); err != nil {
		return nil, err
	}
	var out ModifyApplicationInfoResponse
	if err := s.invokePost(ctx, "/modify-application-info", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------- POST /close-account ----------

// CloseAccount submits POST /close-account. Returns the spec's
// closure outcome (SUCCESS / UNPAID_DEBT / OVERPAID_UNRETURNED /
// ACTIVE_ACCOUNT / FAILED / USER_ACCOUNT_NOT_EXIST /
// ONGOING_WITHDRAWAL).
//
// RequestID is auto-minted from the Client's generator when empty.
func (s *Service) CloseAccount(ctx context.Context, req *CloseAccountParam) (*CloseAccountResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil CloseAccountParam"}
	}
	if req.RequestID == "" {
		req.RequestID = s.c.NewRequestID()
	}
	if err := validateCloseAccount(req); err != nil {
		return nil, err
	}
	var out CloseAccountResponse
	if err := s.invokePost(ctx, "/close-account", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
