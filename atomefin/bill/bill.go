package bill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

const (
	DefaultStart = 1
	DefaultCount = 10
	MaxCount     = 50
)

// Service is the outbound bill-query client. Construct via
// bill.New(c). Immutable after construction; safe for concurrent
// use across goroutines.
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil
// when passed a nil Client so the caller fails fast at the dial
// site rather than panicking inside Bills / BillDetail.
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

// checkConfigured guards against nil receiver / nil Client.
// Mirrors payment and refund's nil-safety pattern.
func (s *Service) checkConfigured() error {
	if s == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "nil *bill.Service (likely from bill.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*bill.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// Bills retrieves the partner's bill list.
func (s *Service) Bills(ctx context.Context, params *BillsParams) (*BillsResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &BillsParams{}
	}
	if err := validateBillsParams(params); err != nil {
		return nil, err
	}

	q := buildBillsQuery(params)
	resp, err := s.c.DoSignedGET(ctx, "/bills", q)
	if err != nil {
		return nil, err
	}
	var out BillsResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/bills",
			Err: fmt.Errorf("decode /bills response: %w", uerr),
		}
	}
	return &out, nil
}

// BillDetail retrieves the full detail for a single bill, keyed by
// billID (format yyyyMM, e.g. "202607") + externalReferenceUID.
//
// Spec endpoint: GET /billDetail?billId=<id>&externalReferenceUid=<uid>
//
// Signature change in v0.2.3: takes both `billID` and
// `externalReferenceUID` to match the 2026-05-06 spec snapshot —
// both query params are required. v0.2.0 — v0.2.2 callers must add
// the externalReferenceUID argument.
func (s *Service) BillDetail(ctx context.Context, billID, externalReferenceUID string) (*BillDetailResponse, error) {
	return s.BillDetailWithParams(ctx, &BillDetailParams{
		BillID:               billID,
		ExternalReferenceUID: externalReferenceUID,
	})
}

// BillDetailWithParams retrieves the full detail for a single bill and
// exposes the optional start/count query params from the swagger.
func (s *Service) BillDetailWithParams(ctx context.Context, params *BillDetailParams) (*BillDetailResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		return nil, &atomefin.ValidationError{Field: "request", Message: "nil BillDetailParams"}
	}
	if err := validateBillDetailParams(params); err != nil {
		return nil, err
	}
	q := buildBillDetailQuery(params)
	resp, err := s.c.DoSignedGET(ctx, "/billDetail", q)
	if err != nil {
		return nil, err
	}
	var out BillDetailResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/billDetail",
			Err: fmt.Errorf("decode /billDetail response: %w", uerr),
		}
	}
	return &out, nil
}

func validateBillDetailParams(p *BillDetailParams) error {
	if p.BillID == "" {
		return &atomefin.ValidationError{
			Field:   "billId",
			Message: "required (format yyyyMM, e.g. \"202607\")",
		}
	}
	if p.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "required (the partner-side user identifier)",
		}
	}
	if p.Start < 0 {
		return &atomefin.ValidationError{Field: "start", Message: "must be >= 0 (0 omits the param and uses server default 1)"}
	}
	if p.Count < 0 {
		return &atomefin.ValidationError{Field: "count", Message: "must be >= 0 (0 omits the param and uses server default 10)"}
	}
	if p.Count > MaxCount {
		return &atomefin.ValidationError{Field: "count", Message: "must be <= 50"}
	}
	return nil
}

// BillsUnpaid retrieves the partner's unpaid bill summary.
func (s *Service) BillsUnpaid(ctx context.Context, params *BillsUnpaidParams) (*BillUnpaidResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &BillsUnpaidParams{}
	}
	if err := validateBillsUnpaidParams(params); err != nil {
		return nil, err
	}

	q := buildBillsUnpaidQuery(params)
	resp, err := s.c.DoSignedGET(ctx, "/billUnpaid", q)
	if err != nil {
		return nil, err
	}
	var out BillUnpaidResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/billUnpaid",
			Err: fmt.Errorf("decode /billUnpaid response: %w", uerr),
		}
	}
	return &out, nil
}

// BillsAll is retained for compatibility; /bills no longer paginates
// in the 2026-07 spec, so it returns the single response's data rows.
func (s *Service) BillsAll(ctx context.Context, params *BillsParams) ([]BillsResponseItem, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	resp, err := s.Bills(ctx, params)
	if err != nil || resp == nil {
		return nil, err
	}
	return resp.Data, nil
}

// ---------- Validation ----------

func validateBillsParams(p *BillsParams) error {
	if p.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if p.StartMonth == "" {
		return &atomefin.ValidationError{Field: "startMonth", Message: "required"}
	}
	if p.EndMonth == "" {
		return &atomefin.ValidationError{Field: "endMonth", Message: "required"}
	}
	return nil
}

func validateBillsUnpaidParams(p *BillsUnpaidParams) error {
	if p.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	return nil
}

// ---------- Query construction ----------

func buildBillsQuery(p *BillsParams) url.Values {
	q := url.Values{}
	if p.ExternalReferenceUID != "" {
		q.Set("externalReferenceUid", p.ExternalReferenceUID)
	}
	if p.StartMonth != "" {
		q.Set("startMonth", p.StartMonth)
	}
	if p.EndMonth != "" {
		q.Set("endMonth", p.EndMonth)
	}
	for _, v := range p.Status {
		q.Add("status", string(v))
	}
	for _, v := range p.RepaymentStatus {
		q.Add("repaymentStatus", v)
	}
	for _, v := range p.OverdueStatus {
		q.Add("overdueStatus", string(v))
	}
	for _, v := range p.RefundStatus {
		q.Add("refundStatus", string(v))
	}
	return q
}

func buildBillDetailQuery(p *BillDetailParams) url.Values {
	q := url.Values{
		"billId":               []string{p.BillID},
		"externalReferenceUid": []string{p.ExternalReferenceUID},
	}
	if p.Start > 0 {
		q.Set("start", strconv.Itoa(p.Start))
	}
	if p.Count > 0 {
		q.Set("count", strconv.Itoa(p.Count))
	}
	return q
}

func buildBillsUnpaidQuery(p *BillsUnpaidParams) url.Values {
	q := url.Values{}
	if p.ExternalReferenceUID != "" {
		q.Set("externalReferenceUid", p.ExternalReferenceUID)
	}
	return q
}
