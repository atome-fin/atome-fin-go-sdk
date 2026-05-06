// Package refund implements the outbound refund flow plus the
// inbound refund-terminal callback type-aliases used by
// atomefin/callback. v0.2 chunk; the spec exposes three refund
// surfaces:
//
//   - POST /refund          partner → atome-fin (signed body)
//   - GET  /query-refund    partner → atome-fin (signed query;
//     polling alternative to the PROCESSING webhook)
//   - POST <refundNotifyUrl> atome-fin → partner (terminal-state
//     webhook; handler in atomefin/callback)
//
// # Constructor pattern (mirrors payment)
//
// Like payment, refund avoids the import-cycle that a typed
// `client.Refund` field would create. Construct the Service via
// refund.New(c) where c is an *atomefin.Client:
//
//	c, err := atomefin.New(...)
//	rfd := refund.New(c)
//	res, err := rfd.Refund(ctx, &refund.RefundParam{...})
//
// One extra constructor call vs. method-chaining; partners that
// don't need refund don't pay for it (tree-shake-friendly).
//
// # Q25 — partial-amount semantics (partner-pending)
//
// The 2026-05-06 spec snapshot is silent on whether partial refunds
// (refundAmount < authAmount) are permitted. The SDK's validator is
// **conservative**: it requires `refundAmount` to equal the sum of
// `subOrderRefunds[].refundAmount`, mirroring the capture rule. If
// the partner clarifies that under-refund or over-refund is
// supported, the validator can relax in a minor release.
//
// Documented assumption baked into validateRefund:
//   - refundAmount > 0
//   - refundAmount equals Σ subOrderRefunds[].refundAmount
//
// Until Q25 closes, partners that need to issue partial refunds
// against a sub-order should construct subOrderRefunds covering only
// the lines they want to refund and set refundAmount = sum of those
// lines.
package refund
