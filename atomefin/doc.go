// Package atomefin (no hyphen) is the Go-importable name; the
// published brand is `atome-fin`. Go package names cannot contain a
// hyphen, so the importable name retains the fused form. The module
// path is `github.com/atome-fin/atome-fin-go-sdk`.
//
// Package atomefin is the umbrella package of the atome-fin white-label "G"
// Auth-Capture-Void SDK.
//
// The Client type lives here and is the only stateful root: partners
// construct a Client via atomefin.New(opts...) and dial sub-services off it
// (atomefin/payment for outbound calls, atomefin/callback for the inbound
// webhook receivers).
//
// # Spec
//
// The SDK targets the spec at https://doc.apaylater.net/white-label/G/
// (currently tagged "partner-order-draft"). Because the spec is explicitly
// draft, the SDK ships pre-1.0 (v0.x) and reserves the right to break minor
// versions until the spec stabilises.
//
// # Quick start
//
//	c, err := atomefin.New(
//	    atomefin.WithPrivateKeyPEM(privKeyPEM),
//	    atomefin.WithEnvironment(atomefin.EnvPre),
//	    atomefin.WithPartnerID("partner-foo"),
//	)
//	if err != nil { log.Fatal(err) }
//	// Pass `c` into the payment.Service / callback.Verifier constructors
//	// landing in T3 / T4. DoSigned is the single transport entry point used
//	// by both.
//
// # Open questions still shaping the public API
//
// The following questions from DESIGN.md §13 affect call-site behaviour but
// have ship-now placeholders so T3/T4 can land. The umbrella accepts
// configuration for each via a functional Option so partners can update
// without an SDK release once the partner confirms.
//
//   - Q1 — Final base URLs per environment. EnvPre/EnvProd are wired to the
//     partner gateway URLs; partners with a different gateway URL pass
//     WithBaseURL to override.
//   - Q2 — Authorization header format. Default is SchemeRawBase64 (raw
//     base64 signature, per DESIGN §5 + team-lead 2026-05-05 confirmation);
//     SchemeAtomeKeyed is provided as a one-line override target via
//     WithAuthorizationScheme.
//   - Q3 — Key rotation / keyId transport. WithKeyID stages a value and the
//     scheme function decides how to surface it; transport-level header
//     emission stays a no-op until the partner confirms.
//   - Q5 — Replay protection (timestamp / nonce). No header emitted today;
//     populateHeaders in doer.go is the single point that gains the new
//     header once Q5 closes.
//
// # Resolved questions
//
// Q7 (2026-05-05) — Partner / merchant identification is NOT a wire
// header: partner identity is established by the dedicated API URL plus
// the RSA certificate exchange. WithPartnerID and WithMerchantID stay
// supported as log-enrichment hooks only.
package atomefin
