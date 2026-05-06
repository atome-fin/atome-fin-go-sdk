package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Default pagination parameters used when caller passes the zero
// value. Match bill's defaults so partners using both Service
// families get one consistent paging shape.
const (
	DefaultPageNumber = 1
	DefaultPageSize   = 20
)

// Service is the outbound transaction-query client. Construct via
// transaction.New(c). Immutable after construction; safe for
// concurrent use across goroutines.
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil
// when passed a nil Client so the caller fails fast at the dial
// site rather than panicking inside Transactions / TransactionDetail.
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
// Mirrors payment / refund / bill nil-safety.
func (s *Service) checkConfigured() error {
	if s == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "nil *transaction.Service (likely from transaction.New(nil))",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*transaction.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// Transactions retrieves one page of the partner's transaction
// list.
//
// Spec endpoint: GET /transactions?pageNumber=N&pageSize=M&...
//
// Pass nil for the default first page (PageNumber=1, PageSize=20,
// no filters); pass &TransactionsParams{...} for explicit control.
func (s *Service) Transactions(ctx context.Context, params *TransactionsParams) (*TransactionsResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &TransactionsParams{}
	}
	if err := validateTransactionsParams(params); err != nil {
		return nil, err
	}

	q := buildTransactionsQuery(params)
	resp, err := s.c.DoSignedGET(ctx, "/transactions", q)
	if err != nil {
		return nil, err
	}
	var out TransactionsResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/transactions",
			Err: fmt.Errorf("decode /transactions response: %w", uerr),
		}
	}
	return &out, nil
}

// TransactionDetail retrieves the full detail for a single
// transaction, keyed by the originating partner-side `requestID` +
// `externalReferenceUID` + `transactionType`. The
// `transactionType` discriminates which originating endpoint the
// requestId came from: per spec, alias of PAYMENT, REFUND, or
// REPAYMENT.
//
// Spec endpoint:
//
//	GET /transactionDetail?requestId=<r>&externalReferenceUid=<u>&transactionType=<t>
//
// Signature change in v0.2.3: previously took only `tradeID`
// (which the SDK encoded as `tradeId` per the partner-pending
// 2026-04-22 spec). The 2026-05-06 publish renamed the lookup to
// require all three of `requestId`, `externalReferenceUid`, and
// `transactionType` — `tradeID` no longer exists on the wire.
// v0.2.0 — v0.2.2 callers must update both the signature and their
// call-site bookkeeping (substitute the original payment / refund
// / repayment requestId for the discarded tradeID).
func (s *Service) TransactionDetail(ctx context.Context, requestID, externalReferenceUID string, transactionType TransactionType) (*TransactionDetailResponse, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if requestID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "requestId",
			Message: "required (the original payment / refund / repayment requestId)",
		}
	}
	if externalReferenceUID == "" {
		return nil, &atomefin.ValidationError{
			Field:   "externalReferenceUid",
			Message: "required (the partner-side user identifier)",
		}
	}
	if transactionType == "" {
		return nil, &atomefin.ValidationError{
			Field:   "transactionType",
			Message: "required (PAYMENT / REFUND / REPAYMENT)",
		}
	}
	q := url.Values{
		"requestId":            []string{requestID},
		"externalReferenceUid": []string{externalReferenceUID},
		"transactionType":      []string{string(transactionType)},
	}
	resp, err := s.c.DoSignedGET(ctx, "/transactionDetail", q)
	if err != nil {
		return nil, err
	}
	var out TransactionDetailResponse
	if uerr := json.Unmarshal(resp.Body, &out); uerr != nil {
		return nil, &atomefin.TransportError{
			Op:  "unmarshal",
			URL: "/transactionDetail",
			Err: fmt.Errorf("decode /transactionDetail response: %w", uerr),
		}
	}
	return &out, nil
}

// TransactionsAll walks every page of GET /transactions and returns
// the concatenated rows. Convenience wrapper — partners that need
// per-page control should call Transactions directly.
//
// Mirrors bill.BillsAll's termination logic: short page OR
// cumulative count reaches Total. ctx cancellation honoured between
// pages.
func (s *Service) TransactionsAll(ctx context.Context, params *TransactionsParams) ([]Transaction, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &TransactionsParams{}
	}
	cur := *params
	if cur.PageNumber <= 0 {
		cur.PageNumber = DefaultPageNumber
	}
	if cur.PageSize <= 0 {
		cur.PageSize = DefaultPageSize
	}

	var all []Transaction
	for {
		if err := ctx.Err(); err != nil {
			return all, err
		}
		page, err := s.Transactions(ctx, &cur)
		if err != nil {
			return all, err
		}
		if page == nil || page.Data == nil {
			break
		}
		all = append(all, page.Data.Items...)
		if len(page.Data.Items) < cur.PageSize {
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

func validateTransactionsParams(p *TransactionsParams) error {
	if p.PageNumber < 0 {
		return &atomefin.ValidationError{Field: "pageNumber", Message: "must be >= 0 (0 → default 1)"}
	}
	if p.PageSize < 0 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be >= 0 (0 → default 20)"}
	}
	if p.PageSize > 1000 {
		return &atomefin.ValidationError{Field: "pageSize", Message: "must be <= 1000 (sanity cap)"}
	}
	// TransactionType is intentionally NOT strict-validated here —
	// partners passing an unknown literal hit the server's
	// NOT_FOUND-style envelope and forward-compat works (matches the
	// bill enum pattern).
	return nil
}

// ---------- Query construction ----------

func buildTransactionsQuery(p *TransactionsParams) url.Values {
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
	if p.AuthOrderID != "" {
		q.Set("authOrderId", p.AuthOrderID)
	}
	if p.TransactionType != "" {
		q.Set("transactionType", string(p.TransactionType))
	}
	if p.StartDate != "" {
		q.Set("startDate", p.StartDate)
	}
	if p.EndDate != "" {
		q.Set("endDate", p.EndDate)
	}
	return q
}
