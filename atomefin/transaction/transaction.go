package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Default pagination parameters used when caller passes the zero value.
const (
	DefaultStart = 1
	DefaultCount = 10
	MaxCount     = 50
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
// Spec endpoint: GET /transactions?externalReferenceUid=...&startDate=...&endDate=...&transactionType=...
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

// TransactionsAll walks pages using start/count and returns the final
// concatenated grouped payload.
func (s *Service) TransactionsAll(ctx context.Context, params *TransactionsParams) (*TransactionsData, error) {
	if err := s.checkConfigured(); err != nil {
		return nil, err
	}
	if params == nil {
		params = &TransactionsParams{}
	}
	cur := *params
	if cur.Start <= 0 {
		cur.Start = DefaultStart
	}
	if cur.Count <= 0 {
		cur.Count = DefaultCount
	}

	out := &TransactionsData{}
	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		page, err := s.Transactions(ctx, &cur)
		if err != nil {
			return out, err
		}
		if page == nil || page.Data == nil {
			break
		}
		if out.Currency == "" {
			out.Currency = page.Data.Currency
		}
		out.PaymentInfo = append(out.PaymentInfo, page.Data.PaymentInfo...)
		out.RefundInfo = append(out.RefundInfo, page.Data.RefundInfo...)
		out.RepaymentInfo = append(out.RepaymentInfo, page.Data.RepaymentInfo...)
		out.Paginator = page.Data.Paginator
		got := len(page.Data.PaymentInfo) + len(page.Data.RefundInfo) + len(page.Data.RepaymentInfo)
		if got < cur.Count {
			break
		}
		if page.Data.Paginator != nil && page.Data.Paginator.TotalCount > 0 {
			total := len(out.PaymentInfo) + len(out.RefundInfo) + len(out.RepaymentInfo)
			if total >= page.Data.Paginator.TotalCount {
				break
			}
		}
		if cur.Count <= 0 {
			break
		}
		cur.Start += cur.Count
	}
	return out, nil
}

// ---------- Validation ----------

func validateTransactionsParams(p *TransactionsParams) error {
	if p.ExternalReferenceUID == "" {
		return &atomefin.ValidationError{Field: "externalReferenceUid", Message: "required"}
	}
	if p.StartDate == "" {
		return &atomefin.ValidationError{Field: "startDate", Message: "required"}
	}
	if p.EndDate == "" {
		return &atomefin.ValidationError{Field: "endDate", Message: "required"}
	}
	if p.TransactionType == "" {
		return &atomefin.ValidationError{Field: "transactionType", Message: "required"}
	}
	if p.Start < 0 {
		return &atomefin.ValidationError{Field: "start", Message: "must be >= 0 (0 → default 1)"}
	}
	if p.Count < 0 {
		return &atomefin.ValidationError{Field: "count", Message: "must be >= 0 (0 → default 10)"}
	}
	if p.Count > MaxCount {
		return &atomefin.ValidationError{Field: "count", Message: "must be <= 50"}
	}
	return nil
}

// ---------- Query construction ----------

func buildTransactionsQuery(p *TransactionsParams) url.Values {
	q := url.Values{}
	start := p.Start
	if start <= 0 {
		start = DefaultStart
	}
	count := p.Count
	if count <= 0 {
		count = DefaultCount
	}
	q.Set("start", strconv.Itoa(start))
	q.Set("count", strconv.Itoa(count))
	if p.ExternalReferenceUID != "" {
		q.Set("externalReferenceUid", p.ExternalReferenceUID)
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
