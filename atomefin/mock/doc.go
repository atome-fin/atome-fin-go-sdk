// Package mock provides a first-class testing surface for partners
// who want to exercise their own code against the SDK without
// dialling atome-fin.
//
// The two patterns documented in `docs/MOCK_MODE.md` (RoundTripper
// substitution via `atomefin.WithHTTPClient`; socket-based via
// `httptest.NewServer`) remain the lower-level options. This
// package layers a higher-level surface on top of them:
//
//   - **Pre-built scenarios** — `AlwaysSuccess`,
//     `AlwaysProcessing`, `AlwaysFailed`, `AlwaysAPIError`,
//     `PerEndpoint(map)`. Each is a `Scenario` that decides how
//     the mock transport responds to inbound SDK requests.
//
//   - **`mock.NewClient(t, opts...)`** — returns an
//     `*atomefin.Client` wired to the in-process `Transport`. No
//     listener; no network. EnvProd is hard-blocked.
//
//   - **`mock.NewServer(t, opts...)`** — same scenario dispatch
//     surfaced via a real `httptest.NewServer`. Useful when the
//     test is an HTTP client other than `atomefin.Client` (e.g. a
//     bash smoke harness, a curl-based contract test).
//
//   - **`Fire*Callback` helpers** — seven entry points
//     (`FireAuthCallback`, `FireCaptureCallback`,
//     `FireRefundCallback`, `FireRepaymentCallback`,
//     `FireAccountChangeCallback`, `FireCreditApplicationCallback`,
//     `FireCreditInformationCallback`) that build a signed callback
//     body and dispatch it to a partner's `http.Handler`. Lets
//     partners exercise their callback handlers without a separate
//     signing-test harness.
//
//   - **Bundled mock keypair** — one RSA-2048 keypair for signing
//     and one for hybrid encryption, committed under `testdata/`.
//     Off by default (partners must opt-in via
//     `WithMockKeysAllowed`) so a miswired test can't pick up the
//     bundled keys silently. Production private keys are never in
//     this package — the bundled keys are clearly labelled and
//     committed to-disk for reproducibility.
//
// # EnvProd refusal
//
// `mock.NewClient` and `mock.NewServer` REFUSE to construct when
// `Environment == EnvProd`. The intent is partner-protective:
// nothing in this package should ever co-exist with a production
// configuration accidentally, and the simplest way to guarantee
// that is to fail loud at test setup.
//
// # Migration
//
// Tests written against `WithHTTPClient` or `httptest.NewServer`
// (the v0.3.1 patterns) continue to work unchanged. Adopting
// `atomefin/mock` is opt-in — the SDK does NOT require it.
package mock
