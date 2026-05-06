package bill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Default pagination parameters used when caller passes the zero
// value. Tuned to match the spec's recommended page size; partners
// can override per call.
const (
	DefaultPageNumber = 1
	DefaultPageSize   = 20
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

// Bills retrieves one page of the partner's bill list.
//
// Spec endpoint: GET /bills?pageNumber=N&pageSize=M&...
//
// Pass nil for the default first page (PageNumber=1, PageSize=20,
// no filters); pass &BillsParams{...} for explicit control.
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
// billID (yyyyMM, e.g. "202605") + externalReferenceUID.
//
// Spec endpoint: GET /billDetail?billId=<id>&externalReferenceUid=<uid>
//
// Signature change in v0.2.3: takes both `billID` and
// `externalReferenceUID` to match the 2026-05-06 spec snapshot —
// both query params are required. v0.2.0 — v0.2.2 callers must add
// the externalReferenceUID argument.
func (s *Service) BillDetail(ctx context.Context, billID, externalReferenceUID string) (*BillDetailResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if billID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "billId",
			Message: "required (yyyyMM, e.g. \"202605\")",
		}
	}
	if externalReferenceUID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "required (the partner-side user identifier)",
		}
	}
	q := url.Values{
		"billId":               []string{billID},
		"externalReferenceUid": []string{externalReferenceUID},
	}
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

// BillsUnpaid retrieves one page of the partner's unpaid bills. The
// endpoint pre-filters to OverdueStatus != ON_TIME (or to bills
// where UnpaidAmount > 0 — exact server-side semantics may evolve).
//
// Spec endpoint: GET /billUnpaid?pageNumber=N&pageSize=M&...
func (s *Service) BillsUnpaid(ctx context.Context, params *BillsUnpaidParams) (*BillsResponse, error) {
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
	var out BillsResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/billUnpaid",
			Err: fmt.Errorf("decode /billUnpaid response: %w", uerr),
		}
	}
	return &out, nil
}

// BillsAll walks every page of GET /bills and returns the
// concatenated rows. Convenience wrapper — partners that need
// per-page control should call Bills directly.
//
// The loop honours params (filters carry through every page) and
// terminates when the cumulative count reaches Data.Total OR the
// server returns fewer rows than PageSize. The starting PageNumber
// can be set on params; if zero, the loop begins at PageNumber=1.
//
// ctx cancellation is honoured between pages; each individual
// Bills call is itself cancellable.
func (s *Service) BillsAll(ctx context.Context, params *BillsParams) ([]Bill, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &BillsParams{}
	}
	// Make a copy so we can mutate PageNumber without touching
	// the caller's struct.
	cur := *params
	if cur.PageNumber <= 0 {
		cur.PageNumber = DefaultPageNumber
	}
	if cur.PageSize <= 0 {
		cur.PageSize = DefaultPageSize
	}

	var all []Bill
	for {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		page, err := s.Bills(ctx, &cur)
		if err != nil {
			return all, err
		}
		if page == nil || page.Data == nil {
			break
		}
		all = append(all, page.Data.Bills...)
		// Termination: short page OR we've reached Total.
		if len(page.Data.Bills) < cur.PageSize {
			break
		}
		if page.Data.Total > 0 && len(all) >= page.Data.Total {
			break
		}
		cur.PageNumber++
	}
	return all, nil
}

// ---------- Validation ----------

func validateBillsParams(p *BillsParams) error {
	if p.PageNumber < 0 {
		return &atomefin.ValidationError{Field: "pageNumber", Message: "must be >= 0 (0 → default 1)"}
	}
	if p.PageSize < 0 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be >= 0 (0 → default 20)"}
	}
	if p.PageSize > 1000 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be <= 1000 (sanity cap)"}
	}
	return nil
}

func validateBillsUnpaidParams(p *BillsUnpaidParams) error {
	if p.PageNumber < 0 {
		return &atomefin.ValidationError{Field: "pageNumber", Message: "must be >= 0 (0 → default 1)"}
	}
	if p.PageSize < 0 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be >= 0 (0 → default 20)"}
	}
	if p.PageSize > 1000 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be <= 1000 (sanity cap)"}
	}
	return nil
}

// ---------- Query construction ----------

// buildBillsQuery materialises a BillsParams into a url.Values whose
// CanonicalQuery output is the signing canonical. Defaults are
// applied here so the wire query always carries pageNumber +
// pageSize.
func buildBillsQuery(p *BillsParams) url.Values {
	q := url.Values{}
	pn := p.PageNumber
	if pn <= 0 {
		pn = DefaultPageNumber
	}
	ps := p.PageSize
	if ps <= 0 {
		ps = DefaultPageSize
	}
	q.Set("pageNumber", strconv.Itoa(pn))
	q.Set("pageSize", strconv.Itoa(ps))
	if p.ExternalReferenceUID != "" {
		q.Set("externalReferenceUid", p.ExternalReferenceUID)
	}
	if p.BillID != "" {
		q.Set("billId", p.BillID)
	}
	if p.StartMonth != "" {
		q.Set("startMonth", p.StartMonth)
	}
	if p.EndMonth != "" {
		q.Set("endMonth", p.EndMonth)
	}
	return q
}

func buildBillsUnpaidQuery(p *BillsUnpaidParams) url.Values {
	q := url.Values{}
	pn := p.PageNumber
	if pn <= 0 {
		pn = DefaultPageNumber
	}
	ps := p.PageSize
	if ps <= 0 {
		ps = DefaultPageSize
	}
	q.Set("pageNumber", strconv.Itoa(pn))
	q.Set("pageSize", strconv.Itoa(ps))
	if p.ExternalReferenceUID != "" {
		q.Set("externalReferenceUid", p.ExternalReferenceUID)
	}
	return q
}
