// Package transport contains the SDK's internal HTTP machinery — retry
// policy, request/response logging hooks, and the User-Agent assembly.
//
// It is exposed (rather than placed under internal/) so callers can write
// table-driven tests against the same RetryPolicy and Observer types the
// Client uses. Most partners will not import this package directly.
//
// Layout (per DESIGN.md §2.1, to be filled in by T2):
//
//   - retry.go     — RetryPolicy with jittered exponential backoff (defaults:
//     3 attempts, 250ms × 2^n ± 20%, cap 4s, retries on 5xx + transport
//     errors only).
//   - logging.go   — Logger / Observer interfaces and the PII-redaction
//     filter applied before bodies / headers reach a partner-supplied logger.
//   - useragent.go — "atome-fin-go-sdk/<ver>" assembly.
//   - pagination.go — reserved (no list endpoints in v1; see DESIGN §9).
//
// # Open questions affecting transport
//
//   - Q8 — Atome's outbound retry policy informs our reciprocal callback
//     ack expectations but does NOT affect our outbound retries.
//   - Q9 — Rate limits / 429 contract. We will need to thread a Retry-After
//     parser through RetryPolicy once a numeric limit is documented.
//   - Q18 — Whether HTTP 500 retries must be re-signed (default assumption:
//     yes, since the body is unchanged but Authorization is per-request).
package transport
