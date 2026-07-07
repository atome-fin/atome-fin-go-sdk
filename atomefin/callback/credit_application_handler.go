package callback

import (
	"context"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
)

// CreditApplicationEvent is the flat JSON shape posted to the
// partner's credit-application callback endpoint.
type CreditApplicationEvent = credit.CreditApplicationResult

// CreditApplicationHandlerFunc is the user-supplied callback for
// the inbound credit-application terminal events. Returning nil
// acks (HTTP 200); returning an error forces atome-fin to retry
// (HTTP 500).
type CreditApplicationHandlerFunc func(ctx context.Context, event *CreditApplicationEvent) error

// CreditApplicationHandler is the /<creditApplicationNotifyUrl>
// counterpart of AuthHandler / CaptureHandler / RefundHandler. Same
// behavioural contract:
//
//   - body read via io.LimitReader (Verifier.BodyLimit, default 1 MiB)
//   - signature verified against the raw body using Verifier
//     (multi-cert succeeds if any key verifies)
//   - body decoded into *CreditApplicationEvent
//   - fn invoked
//   - ack envelope emitted with the appropriate HTTP status
//
// At-least-once delivery applies — fn must be idempotent against
// duplicate deliveries. Typical pattern: dedupe on
// event.Data.ExternalReferenceUID (plus a per-application
// fingerprint) at the application layer.
func CreditApplicationHandler(v *Verifier, fn CreditApplicationHandlerFunc) http.Handler {
	if fn == nil {
		// Surface the nil user-fn before we wrap it; otherwise the
		// closure would be non-nil and `handle`'s nil-fn guard
		// wouldn't fire. Same defensive shape as AuthHandler /
		// CaptureHandler / RefundHandler.
		return handle[CreditApplicationEvent](v, nil)
	}
	return handle[CreditApplicationEvent](v, func(ctx context.Context, e *CreditApplicationEvent) error {
		return fn(ctx, e)
	})
}
