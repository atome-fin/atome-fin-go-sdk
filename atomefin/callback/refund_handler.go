package callback

import (
	"context"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
)

// RefundEvent is the flat JSON shape posted to the partner's
// refund-callback endpoint.
type RefundEvent = refund.RefundResult

// RefundHandlerFunc is the user-supplied callback for the inbound
// refund-terminal events. Returning nil acks (HTTP 200);
// returning an error forces atome-fin to retry (HTTP 500).
type RefundHandlerFunc func(ctx context.Context, event *RefundEvent) error

// RefundHandler is the /refundNotifyUrl counterpart of AuthHandler /
// CaptureHandler. Same behavioural contract:
//
//   - body read via io.LimitReader (Verifier.BodyLimit, default 1 MiB)
//   - signature verified against the raw body using Verifier
//     (multi-cert succeeds if any key verifies)
//   - body decoded into *RefundEvent
//   - fn invoked
//   - ack envelope emitted with the appropriate HTTP status
//
// At-least-once delivery applies — fn must be idempotent against
// duplicate deliveries. Typical pattern: dedupe on
// event.Data.RequestID at the application layer.
func RefundHandler(v *Verifier, fn RefundHandlerFunc) http.Handler {
	if fn == nil {
		// Surface the nil user-fn before we wrap it; otherwise the
		// closure would be non-nil and `handle`'s nil-fn guard
		// wouldn't fire. Same defensive shape as AuthHandler /
		// CaptureHandler.
		return handle[RefundEvent](v, nil)
	}
	return handle[RefundEvent](v, func(ctx context.Context, e *RefundEvent) error {
		return fn(ctx, e)
	})
}
