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
//
// Note: the 2026-10-07 spec moves this callback's signature from RSA2
// (`Authorization`) to OVO.HMAC (HMAC-SHA256) headers `client-id` /
// `time` / `signature`, verified at the gateway boundary. The handler
// still verifies RSA2 until the OVO.HMAC signing canonical is
// confirmed; see CHANGELOG.md §0.8.13.
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
