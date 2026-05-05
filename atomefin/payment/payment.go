// Package payment implements the outbound POST /auth, POST /capture and
// POST /voidAuth calls of the atomefin white-label "G" API.
//
// See doc.go for the package overview and the constructor pattern. This
// file hosts the Service struct, its constructor, and the small
// PollUntilTerminal helper that wraps the spec's "submit then poll
// idempotently with the same requestId" flow when partners want a
// synchronous return.
package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// Service is the outbound payment client. Construct with payment.New(c)
// where c is an *atomefin.Client. The struct is immutable after
// construction and safe for concurrent use across goroutines.
type Service struct {
	c *atomefin.Client
}

// New returns a *Service bound to the given Client. Returns nil when
// passed nil (rather than panicking later inside Auth) so a partner
// missing atomefin.New error-handling fails fast at the dial site.
func New(c *atomefin.Client) *Service {
	if c == nil {
		return nil
	}
	return &Service{c: c}
}

// Client exposes the underlying *atomefin.Client. Useful for code that
// has only the *Service handle but needs the request-id generator or
// the verifier slot for callback wiring.
//
// Returns nil when the receiver is nil or was constructed with a nil
// Client (i.e. from payment.New(nil)) — callers should checkConfigured
// before relying on the result.
func (s *Service) Client() *atomefin.Client {
	if s == nil {
		return nil
	}
	return s.c
}

// checkConfigured guards against a nil Service or a Service holding a
// nil *atomefin.Client. The struct is normally constructed via
// payment.New(c); when c == nil that constructor returns nil so a
// caller that ignored New's return value will reach here with s ==
// nil. Either case is a configuration bug, surfaced as
// *atomefin.ValidationError so callers can errors.As uniformly.
//
// This guard runs as the first line of every public Service method so
// nil-deref panics from auto-mint paths (NewRequestID), validation, or
// invoke never escape into partner code.
func (s *Service) checkConfigured() error {
	if s == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "nil *payment.Service (likely from payment.New(nil) — check the *atomefin.Client returned by atomefin.New)",
		}
	}
	if s.c == nil {
		return &atomefin.ValidationError{
			Field:   "service",
			Message: "*payment.Service has nil *atomefin.Client",
		}
	}
	return nil
}

// invoke is the private DoSigned wrapper used by Auth / Capture /
// VoidAuth. It marshals via atomefin.MarshalSigning (HTML escaping
// off — critical for signing canonical correctness, see DESIGN
// batteries-review #4), threads any extra per-call headers
// (e.g. /auth's sessionid), invokes Client.DoSigned, and unmarshals
// the response body into out.
//
// On 2xx the JSON body is decoded into out and the function returns
// nil. On non-2xx Client.DoSigned has already produced an *APIError —
// invoke just propagates it; on transport / signing failures the
// matching error type is propagated unchanged.
func (s *Service) invoke(ctx context.Context, op string, in any, out any, opts ...atomefin.DoSignedOption) error {
	if s == nil || s.c == nil {
		return errors.New("atomefin/payment: nil Service or Client")
	}
	if in == nil {
		return &atomefin.ValidationError{Field: "request", Message: "nil request"}
	}
	body, err := atomefin.MarshalSigning(in)
	if err != nil {
		// Treat marshal failure as a signature-class error: the bytes
		// fed to the signer never made it past json. SignatureError
		// is the closest type-classification — it is non-temporary.
		return &atomefin.SignatureError{Reason: "marshal", Err: err}
	}
	resp, err := s.c.DoSigned(ctx, http.MethodPost, op, body, opts...)
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

// IsTerminal reports whether an api response status is SUCCESS or
// FAILED. Nil-safe so callers can chain `if !resp.IsTerminal() { ... }`
// without first nil-checking the .Data pointer.
func IsTerminal(s atomefin.Status) bool { return s.IsTerminal() }

// IsProcessing reports whether status is PROCESSING.
func IsProcessing(s atomefin.Status) bool { return s == atomefin.StatusProcessing }

// PollOptions control PollUntilTerminal's loop. Zero-value uses
// reasonable defaults: 30s ceiling, 250ms initial poll, 8s cap.
type PollOptions struct {
	// MaxWait bounds the total time the poll loop will run, in addition
	// to the parent ctx's deadline. Defaults to 30s.
	MaxWait time.Duration
	// InitialDelay is the wait before the first re-poll after PROCESSING.
	InitialDelay time.Duration
	// MaxDelay caps any single backoff step.
	MaxDelay time.Duration
	// Multiplier is the per-step backoff multiplier; default 2.0.
	Multiplier float64
}

func (p PollOptions) withDefaults() PollOptions {
	if p.MaxWait <= 0 {
		p.MaxWait = 30 * time.Second
	}
	if p.InitialDelay <= 0 {
		p.InitialDelay = 250 * time.Millisecond
	}
	if p.MaxDelay <= 0 {
		p.MaxDelay = 8 * time.Second
	}
	if p.Multiplier < 1 {
		p.Multiplier = 2.0
	}
	return p
}

// PollUntilTerminal repeatedly invokes invoke (with the same body
// bytes — and therefore the same requestId, which is why we use the
// caller's pre-built request) until the response Status is terminal
// (SUCCESS / FAILED), the parent ctx expires, or MaxWait elapses.
//
// Generic over T to share one implementation across Auth, Capture,
// and Void responses. Use the typed wrappers AuthPollUntilTerminal /
// CapturePollUntilTerminal where you can — they document the spec
// semantics.
//
// The function is intentionally a thin loop; partners that need
// fancier semantics (jitter, observability between attempts) should
// drive the loop themselves and reuse Service.Auth / .Capture
// directly.
func PollUntilTerminal[T any](
	ctx context.Context,
	opts PollOptions,
	getStatus func(*T) atomefin.Status,
	once func(context.Context) (*T, error),
) (*T, error) {
	o := opts.withDefaults()

	deadline := time.Now().Add(o.MaxWait)
	delay := o.InitialDelay

	for {
		resp, err := once(ctx)
		if err != nil {
			return nil, err
		}
		if resp != nil && getStatus(resp).IsTerminal() {
			return resp, nil
		}

		// Compute remaining budget against both the parent ctx and the
		// PollOptions ceiling.
		now := time.Now()
		if !now.Before(deadline) {
			return resp, &atomefin.TransportError{
				Op:  "poll",
				URL: "payment.PollUntilTerminal",
				Err: fmt.Errorf("max wait %v exceeded without terminal status", o.MaxWait),
			}
		}
		// Sleep, but never longer than ctx allows.
		if err := sleepWithCtx(ctx, delay); err != nil {
			return resp, err
		}
		// Exponential backoff with cap.
		next := time.Duration(float64(delay) * o.Multiplier)
		if next > o.MaxDelay {
			next = o.MaxDelay
		}
		delay = next
	}
}

func sleepWithCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// guard against unused imports flagged by the tooling once the file is
// expanded by future T-N work; keep transport imported for the
// occasional metric we emit.
var _ = transport.RedactedAuthorization
