// Package payment implements the outbound Auth / Capture / VoidAuth calls of
// the atomefin white-label "G" API.
//
// # Constructor pattern (small DESIGN.md deviation)
//
// DESIGN.md §3 originally sketched `client.Payment.Auth(ctx, req)` as the
// call site. T2 implementation revealed an import-cycle constraint: the
// payment package depends on atomefin (for Client.DoSigned, NewRequestID,
// errors, codes) so atomefin cannot in turn hold a typed *payment.Service
// field. The fix is the standard stdlib resource-client pattern:
//
//	c, err := atomefin.New(...)
//	pay := payment.New(c)
//	res, err := pay.Auth(ctx, req)
//
// One extra constructor call, no functional difference.
//
// Layout (per DESIGN.md §2.1, to be filled in by T3):
//
//   - payment.go — Service struct hung off Client.
//   - auth.go    — AuthRequest / AuthResponse / (*Service).Auth.
//   - capture.go — CaptureRequest / CaptureResponse / (*Service).Capture.
//   - void.go    — VoidAuthRequest / VoidAuthResponse / (*Service).VoidAuth.
//   - types.go   — SubOrder, ExtendInfo, AccountChanges, InstallmentDetail,
//     InstallmentPlan and other shared structs.
//
// # Idempotency
//
// Every outbound request carries `requestId` (≤64 chars) inside the JSON body
// — this is the business-level idempotency key, not a header. T3's Service
// methods preserve the same RequestID across retries; partners that want to
// surface their own order ID prefix should set atomefin.WithRequestIDGenerator
// on the Client.
//
// # Sync vs. callback semantics
//
// First submission returns HTTP 200 with `data.status = PROCESSING`; the
// terminal state arrives via the atomefin/callback handlers. Idempotent
// retries of the same RequestID after a terminal state return the terminal
// payload synchronously. Helper PollUntilTerminal will wrap this loop.
//
// # Open questions blocking T3
//
//   - Q11 — Time zone for billDate / dueDate (yyyy-MM-dd).
//   - Q12 — Concrete shape of paymentRiskInfo.
//   - Q13 — Whether skuId is mandatory (per-merchant vs. per-country).
//   - Q15 — Semantics of reapplyTime (wall clock vs. retry-after hint).
//   - Q16 — Whether the periodType set is fixed or merchant-configurable.
//   - Q17 — Relationship between order-level and sub-order periodType.
//
// Items above only affect type ergonomics (e.g. typed enum vs. raw int) and
// not the wire shape, so we can land plain int / string types in T3 and
// tighten them in a minor release without breaking JSON compatibility.
package payment
