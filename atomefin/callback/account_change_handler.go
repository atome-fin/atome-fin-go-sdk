package callback

import (
	"context"
	"net/http"
)

// AccountChangeHandlerFunc is the user-supplied callback for
// inbound account-change events. Returning nil acks (HTTP 200);
// returning an error forces atome-fin to retry (HTTP 500).
//
// Atome-fin guarantees at-least-once delivery, so fn must be
// idempotent against duplicate deliveries. Typical pattern: dedupe
// on event.Data.EventID (short-lived dedupe set is sufficient).
type AccountChangeHandlerFunc func(ctx context.Context, event *AccountChangeEvent) error

// AccountChangeHandler returns the http.Handler for the
// /<accountChangeNotifyUrl> endpoint. Same behavioural contract as
// AuthHandler / CaptureHandler / RefundHandler:
//
//   - body read via io.LimitReader (Verifier.BodyLimit, default 1 MiB)
//   - signature verified against the raw body using Verifier
//     (multi-cert succeeds if any key verifies)
//   - body decoded into *AccountChangeEvent
//   - fn invoked
//   - ack envelope emitted with the appropriate HTTP status
//
// Account-change is inbound-only — there is no outbound
// counterpart. Partners that aren't tracking account-state changes
// can leave the handler unmounted; the rest of the SDK is
// unaffected.
func AccountChangeHandler(v *Verifier, fn AccountChangeHandlerFunc) http.Handler {
	if fn == nil {
		// Surface the nil user-fn before we wrap it; otherwise the
		// closure would be non-nil and `handle`'s nil-fn guard
		// wouldn't fire. Same defensive shape as Auth/Capture/Refund.
		return handle[AccountChangeEvent](v, nil)
	}
	return handle[AccountChangeEvent](v, func(ctx context.Context, e *AccountChangeEvent) error {
		return fn(ctx, e)
	})
}
