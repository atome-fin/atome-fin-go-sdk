// Package repayment implements the outbound repayment-apply / query
// flow plus the inbound repayment-terminal callback type-aliases used
// by atomefin/callback. v0.2 chunk; the spec exposes three repayment
// surfaces:
//
//   - POST /repayment-request    partner → atome-fin (signed body)
//   - GET  /repayment-result     partner → atome-fin (signed query;
//     polling alternative to the PROCESSING webhook). Two query
//     parameters: requestId + externalReferenceUid (BOTH required by
//     spec).
//   - POST /repayment-callback   atome-fin → partner (terminal-state
//     webhook; handler in atomefin/callback)
//
// # Constructor pattern (mirrors payment / refund)
//
// repayment avoids the import-cycle that a typed `client.Repayment`
// field would create. Construct the Service via repayment.New(c)
// where c is an *atomefin.Client:
//
//	c, err := atomefin.New(...)
//	rep := repayment.New(c)
//	res, err := rep.Repayment(ctx, &repayment.RepaymentParam{...})
//
// One extra constructor call vs. method-chaining; partners that
// don't need repayment don't pay for it (tree-shake-friendly).
//
// # Status / event semantics
//
// Two distinct enum surfaces live here:
//
//   - RepaymentResult.Status uses atomefin.Status — PROCESSING / SUCCESS
//     / FAILED. PROCESSING never appears in callbacks (the callback is
//     fired only at terminal states; mirrors auth/capture/refund).
//   - RepaymentResult.Event uses RepaymentEvent — NORMAL,
//     ATOME_REPAYMENT, OVERPAID_REPAYMENT. Identifies the channel that
//     originated the repayment.
//
// Separately, RepaymentStatus (REPAID / UNPAID / PARTIAL_REPAID) is
// the bill-level lifecycle enum used by Bill / BillDetail rows. It
// lives here as the primary domain (bill imports repayment if/when
// needed); v0.2 bill ships a separate OverdueStatus enum with
// different values.
//
// # Validator policy
//
// Per V0.2_DESIGN.md §5: requestId / externalReferenceUid non-empty,
// requestId max length 64, externalReferenceUid max length 64,
// repaymentAmount > 0, repaymentApplyTime > 0 (Unix-ms; partner-spec
// silent on TZ — Q11). Validators run before signing/network so a
// partner sees the rejection without paying a round-trip.
package repayment
