package callback

import (
	"context"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
)

// CreditInformationEvent is the JSON shape posted to the partner's
// credit-information callback endpoint. Byte-for-byte the same
// envelope as the synchronous GET /credit-information-result
// response — partners do not learn a parallel callback schema
// (mirrors RefundEvent / CreditApplicationEvent).
type CreditInformationEvent = credit.CreditInformationCollectResponse

// CreditInformationHandlerFunc is the user-supplied callback for
// the inbound credit-information terminal events. Returning nil
// acks (HTTP 200); returning an error forces atome-fin to retry
// (HTTP 500).
type CreditInformationHandlerFunc func(ctx context.Context, event *CreditInformationEvent) error

// CreditInformationHandler is the /<creditInformationNotifyUrl>
// counterpart of AuthHandler / CaptureHandler / RefundHandler /
// CreditApplicationHandler.
//
// At-least-once delivery applies — fn must be idempotent against
// duplicate deliveries. Typical pattern: dedupe on
// event.Data.RequestID at the application layer.
//
// Deprecated: the 2026-10-12 G spec defines no credit-information
// callback path — results arrive via GET /credit-information-result
// polling only. Kept for partners wired against earlier drafts; it
// will be removed once the spec stabilises without it.
func CreditInformationHandler(v *Verifier, fn CreditInformationHandlerFunc) http.Handler {
	if fn == nil {
		return handle[CreditInformationEvent](v, nil)
	}
	return handle[CreditInformationEvent](v, func(ctx context.Context, e *CreditInformationEvent) error {
		return fn(ctx, e)
	})
}
