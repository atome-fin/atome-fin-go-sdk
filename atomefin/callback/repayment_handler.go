package callback

import (
	"context"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
)

// RepaymentEvent is the JSON shape posted to the partner's
// /repayment-callback endpoint. Byte-for-byte the same envelope as
// the synchronous /repayment-request response — partners do not learn
// a parallel callback schema (DESIGN.md §8 contract; mirrors AuthEvent
// / CaptureEvent / RefundEvent).
type RepaymentEvent = repayment.RepaymentResponse

// RepaymentHandlerFunc is the user-supplied callback for the inbound
// repayment-terminal events. Returning nil acks (HTTP 200);
// returning an error forces atome-fin to retry (HTTP 500).
type RepaymentHandlerFunc func(ctx context.Context, event *RepaymentEvent) error

// RepaymentHandler is the /repayment-callback counterpart of
// AuthHandler / CaptureHandler / RefundHandler. Same behavioural
// contract:
//
//   - body read via io.LimitReader (Verifier.BodyLimit, default 1 MiB)
//   - signature verified against the raw body using Verifier
//     (multi-cert succeeds if any key verifies)
//   - body decoded into *RepaymentEvent
//   - fn invoked
//   - ack envelope emitted with the appropriate HTTP status
//
// At-least-once delivery applies (atome-fin retries up to 14 times
// per spec on non-200 responses) — fn must be idempotent against
// duplicate deliveries. Typical pattern: dedupe on
// event.Data.RequestID at the application layer.
func RepaymentHandler(v *Verifier, fn RepaymentHandlerFunc) http.Handler {
	if fn == nil {
		// Surface the nil user-fn before we wrap it; otherwise the
		// closure would be non-nil and `handle`'s nil-fn guard
		// wouldn't fire. Same defensive shape as AuthHandler /
		// CaptureHandler / RefundHandler.
		return handle[RepaymentEvent](v, nil)
	}
	return handle[RepaymentEvent](v, func(ctx context.Context, e *RepaymentEvent) error {
		return fn(ctx, e)
	})
}
