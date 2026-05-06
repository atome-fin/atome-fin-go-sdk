# Changelog

All notable changes to atome-fin-go-sdk will be documented in this
file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
post-1.0. Pre-1.0 minor versions may break.

## [Unreleased]

v0.2 work-in-progress; chunks accumulate here until tag.

### Added — refund sub-package (v0.2 chunk #2)

- **`atomefin/refund`** — new sub-package mirroring `atomefin/payment`.
  Constructor pattern: `refund.New(c)` (no `c.Refund` accessor — keeps
  the umbrella tree-shake-friendly for partners that only need
  payment).
  - `Service.Refund(ctx, req *RefundParam) (*RefundResponse, error)` —
    POST `/refund` (signed body via `atomefin.MarshalSigning`,
    HTML-escape OFF). Auto-mints `RequestID` via `Client.NewRequestID()`
    when empty; mirrors `payment.Auth`'s shape.
  - `Service.QueryRefund(ctx, requestID string) (*RefundResponse, error)` —
    GET `/query-refund?requestId=<id>` via the new
    `Client.DoSignedGET` (chunk #1). Same envelope as `Refund` —
    polling alternative to the PROCESSING webhook.
  - `Service.RefundPollUntilTerminal(ctx, req, opts)` — reuses
    `payment.PollUntilTerminal` so backoff semantics are identical
    across Service families.
  - Types: `RefundParam`, `RefundResponse` (wraps `RefundResult`),
    `SubOrderRefundRequest`, `SubOrderRefundInfo`. `RefundResult.AccountChanges`
    re-uses `payment.AccountChanges` (no duplication; refund imports
    payment, no cycle).
  - `IsTerminal` / `IsProcessing` helpers on `RefundResponse`,
    nil-safe; mirrors the payment response helpers.
  - `(s *Service).Client()` accessor; nil-safe `checkConfigured`
    guard at the top of every public method (mirrors payment's
    nil-Service safety from v0.1.1).
- **`atomefin/callback/refund_handler.go`** — new `RefundHandler`
  reusing the generic `handle[T]` core. Type alias
  `RefundEvent = refund.RefundResponse` so partners don't learn a
  parallel callback schema.

### Q25 — partial-refund semantics (partner-pending, conservative)

- The 2026-05-06 spec snapshot is silent on whether `refundAmount`
  may be less than the prior `authAmount` (or less than the sum of
  the sub-order refund lines). Validator enforces the strict-equal
  rule
  `refundAmount == Σ subOrderRefunds[].refundAmount` mirroring
  capture's sum-rule. Documented in `atomefin/refund/doc.go` and in
  `atomefin/refund/refund.go`'s `validateRefund`. Partners that
  need partial refunds should construct `subOrderRefunds` covering
  only the lines they want refunded and set `refundAmount` =
  Σ of those lines. The validator can relax in a minor release once
  the spec clarifies.

### Documentation

- `README.md` — three new rows in the Implemented endpoints table
  for `/refund`, `/query-refund`, `<refundNotifyUrl>`. Package map
  gains an `atomefin/refund` entry; the existing `atomefin/payment`
  row updated to list the v0.2 `Query*` additions; the
  `atomefin/callback` row gains `RefundHandler`.

### Tests

- `atomefin/refund/service_test.go` — happy paths
  (`TestService_Refund_Success` asserts `Method=POST`, path=`/refund`,
  body contains the requestId), auto-mint, 4xx → `*APIError`,
  QueryRefund happy path + 4xx, `RefundPollUntilTerminal` PROCESSING
  → SUCCESS, `New(nil) == nil`, full nil-Service safety on every
  public method, table-driven validation with 9 rejection cases
  (nil request, long requestId, missing externalReferenceUid,
  missing authOrderId, zero refundAmount, empty subOrderRefunds,
  empty subOrderId, zero sub-refundAmount, sum-mismatch Q25),
  QueryRefund empty/long requestId rejection.
- `atomefin/refund/marshal_audit_test.go` — `GoldenRoundTrip` per
  fixture × matching type (RefundParam, RefundResponse success /
  processing / failed, query-refund response, callback body); R10
  amount corpus on `RefundParam.RefundAmount` and
  `SubOrderRefundRequest.RefundAmount`; R11 fractional-amount
  rejection on both; R12 integer-literal-only assertion;
  R3 omitempty / R4 required-emit on `RefundParam`.
- `atomefin/callback/refund_handler_test.go` — happy path
  (Content-Type, X-Content-Type-Options=nosniff, ack envelope
  shape), tampered-body 401, multi-cert end-to-end (signed with old
  key, verifier holds both), replay invokes user fn twice (partner-
  owned dedupe contract), 500 on user error, nil-verifier / nil-
  userFn → 500, fixture decode.

### Fixtures

- `qa/testdata/refund_request.json`
- `qa/testdata/refund_response_success.json`
- `qa/testdata/refund_response_processing.json`
- `qa/testdata/refund_response_failed.json`
- `qa/testdata/query_refund_response_success.json`
- `qa/testdata/callback_refund_terminal_success.json`

### Added — DoSignedGET + Query* (v0.2 chunk #1)

- **`Client.DoSignedGET(ctx, path, query, opts...)`** —
  GET-equivalent of `DoSigned`. Signs `sign.CanonicalQuery(query)` and
  assigns the SAME bytes to `req.URL.RawQuery` so the wire is
  byte-equal to the signing canonical (architect §1 R13 invariant —
  prevents `+`-vs-`%20` drift that `url.Values.Encode()` would
  introduce). Same retry/observer/headers/error-envelope/limit-
  reader pipeline as `DoSigned`. ctx cancellation is honoured
  during backoff sleeps. `DoSigned` and `DoSignedGET` now share a
  private `signAndDispatch` helper — no behaviour change to the
  POST path.
- **`payment.New(c).QueryAuth(ctx, requestID)`** /
  **`QueryCapture(ctx, requestID)`** / **`QueryVoidAuth(ctx, requestID)`**
  — typed wrappers around `DoSignedGET` for the spec's polling
  endpoints (`GET /query-auth` / `GET /query-capture` /
  `GET /query-voidAuth`). Each returns the SAME envelope shape as
  the POST counterpart (`*payment.AuthResponse` etc.) so partners
  can switch between webhook-driven and poll-driven completion
  detection without learning a parallel response schema. Validators
  reject empty / >64-char `requestID` before the network round-trip.

### Changed

- `Client.DoSigned`'s `ValidationError` message on non-POST verbs
  updated to point at `DoSignedGET` for the GET path.

### Documentation

- `README.md` — three new rows in the Implemented endpoints table
  for the `/query-*` GET endpoints.
- `DESIGN.md` §5 — GET-path note rewritten: GET is no longer
  reserved-only, `DoSignedGET` is the first-class entry point,
  `payment.QueryX` are the typed wrappers; cross-reference to the
  R13 wire-≡-canonical invariant test.

### Tests

- `atomefin/dosigned_get_test.go` — 9 tests covering R13
  (`TestDoSignedGET_R13_WireEqualsCanonical` — server-side rebuild
  of canonical from `r.URL.Query()` + verifier round-trip),
  4xx → APIError, 5xx retries, retry idempotency (canonical bytes
  identical across attempts), ctx-cancel-in-sleep, reserved-header
  allowlist, custom-header passthrough, bad-path validation,
  nil-client safety, empty-query passthrough.
- `atomefin/payment/query_test.go` — 7 tests: happy path per
  endpoint (asserts Method=GET, Path, RawQuery byte-shape),
  empty-requestID rejection, >64-char requestID rejection,
  nil-Service safety, 4xx-becomes-APIError.

## [0.1.1] — 2026-05-06

Patch release tracking the upstream spec snapshot dated 2026-05-06:
one latent-bug fix, one tightening, and the snapshot stamp updates.
Non-breaking on the wire and on the public API surface (named-type
Currency promotion is the one micro-break — `string(c)` casts are
the only callers affected).

### Fixed

- **`payment.CaptureRequest.ExternalReferenceUID`** is now a required
  field on the request body. v0.1.0 shipped without it, which made
  `/capture` non-compliant against both the v1-draft and the
  2026-05-06 snapshot of the upstream spec. The validator rejects
  empty values before the network round-trip; the QA fixture
  `qa/testdata/capture_request.json` is updated to include the field;
  `examples/auth_capture/main.go` now mirrors the auth request's
  `ExternalReferenceUID` into the capture request, matching the
  expected partner pattern.

### Changed

- **`atomefin.Currency` is now a named type, enum-locked to `IDR`.**
  v0.1.0 shipped `type Currency = string` (transparent alias) and
  flagged Q10 (currency set) as open. The 2026-05-06 spec snapshot
  resolves Q10 to a single supported currency: Indonesian rupiah.
  v0.1.1 promotes the type to `type Currency string` (named) and
  adds:
  - `const CurrencyIDR Currency = "IDR"`
  - `(Currency).IsValid() bool` — strict, returns true only for IDR
  - `(Currency).String() string` — wire-literal passthrough

  Decode policy stays **permissive** (any string accepted on inbound
  for forward-compat with v2 currencies); outbound validators reject
  non-IDR via `IsValid`. Migration: code that used to pass `string`
  values directly to a `Currency` field via raw `string(...)` casts
  now needs an explicit `atomefin.Currency(s)` conversion. String
  literals (`var c atomefin.Currency = "IDR"`) continue to work
  unchanged.

### Documentation

- **Spec snapshot stamp updated** from 2026-04-22 → **2026-05-06**
  in `README.md` (Implemented endpoints section) and `DESIGN.md`
  (§13/Q20 spec-stability note).
- **`DESIGN.md` §13/Q10` annotated **RESOLVED 2026-05-06** with the
  IDR-only resolution and a pointer to the new `Currency` named-type
  + `CurrencyIDR` constant + `IsValid` helper.

### Coming next

v0.2 is currently scoped for the partner-pending Q-set (separate
PSS cert exchange, Q5 timestamp/nonce header, Q9 rate-limit/Retry-
After handling). No timeline yet; behind any of those landing.

## [0.1.0] — 2026-05-05

First feature-complete cut covering the atome-fin white-label "G"
Auth-Capture-Void spec end-to-end.

### Renamed

- **Project rebrand: `apaylater` → `atomefin` → `atome-fin`** (all
  on 2026-05-05). The published brand and module path settle on
  **`atome-fin`** (with hyphen):
  - **Module path**: `github.com/atome-fin/atome-fin-go-sdk`
  - **Go package name**: `atomefin` (NO hyphen — Go syntax forbids
    `-` in package identifiers, so the importable name retains the
    fused form). Partners write
    `import "github.com/atome-fin/atome-fin-go-sdk/atomefin"` and
    call `atomefin.New(...)`. The disambiguation is documented at
    the top of `atomefin/doc.go`.
  - **Environment variables**: `ATOME_FIN_*` (env-var convention is
    underscores, so `ATOME_FIN_PRIV_KEY_PEM` etc., not
    `ATOME-FIN_*`).
  - **Brand prose** in READMEs / DESIGN / docs: `atome-fin`.
  - **LICENSE**: copyright reads "atome-fin-go-sdk contributors".
  - **What does NOT change**: `package atomefin` declarations
    (Go syntax), the `atomefin/` directory (matches the package
    name), `atomefin.X` Go callsites, and the `"atomefin: ..."`
    error-message prefix (idiomatic Go — error strings carry the
    importable package name).
  - **Atome (the counterparty)** is untouched everywhere —
    `Atome`-prefixed identifiers (`WithAtomePublicCertPEM`,
    `SchemeAtomeKeyed`, "Atome public cert") and the spec's external
    URLs (`apaylater.net`, `api.atome.id`) stay verbatim. Atome and
    atome-fin are distinct entities.

### Changed

- **Q7 RESOLVED — partner identity is NOT a wire header.** The spec's
  partner-identification mechanism is the dedicated API URL plus the
  RSA certificate exchange; there is no `X-Partner-Id` /
  `X-Merchant-Id` header on the wire. Removed the provisional header
  emission from `populateHeaders`. `WithPartnerID` / `WithMerchantID`
  stay supported but are now optional log-enrichment hooks only —
  `atomefin.New(...)` no longer requires `WithPartnerID`. Existing
  partners that constructed with the option keep working without a
  code change; the value flows into `Client.PartnerID()` /
  `MerchantID()` accessors for use in log fields.

### Added

- **`atomefin/sign`** — RSA-2048 signing primitives:
  - `Signer` / `Verifier` interfaces with PKCS#1-v1.5 (default) and
    RSA-PSS (`WithSaltedPSS` / `WithVerifierSaltedPSS`) implementations.
  - `LoadPrivateKeyPEM` / `LoadPublicCertPEM` accepting PKCS#1, PKCS#8,
    PKIX, and X.509 CERTIFICATE blocks.
  - `CanonicalQuery` for the GET signing canonical (reserved; no GET
    endpoints in v1).
  - 95.9% test coverage with fuzz tests on the canonical-query path.

- **`atomefin`** — umbrella package:
  - `Client` constructed via `atomefin.New(opts...)` with functional
    options for HTTP client, base URL / environment, signer, Atome
    public key, retry policy, request-id generator, logger, observer,
    body-size cap, custom authorization scheme, and more.
  - `DoSigned(ctx, method, path, body, opts...)` — single signed-transport
    entry point used by `payment.Service`.
  - `WithRequestHeader(key, value)` per-call header injection (used by
    `/auth` for the `sessionid` header). Reserved headers
    (`Authorization`, `Content-Type`, `User-Agent`, `Accept`) cannot be
    overridden.
  - `MarshalSigning(v) ([]byte, error)` — `json.Encoder` with
    `SetEscapeHTML(false)`, trailing-newline-stripped — guarantees the
    bytes signed equal the bytes transmitted.
  - Typed errors: `APIError`, `TransportError`, `SignatureError`,
    `ValidationError`. All implement `Temporary()` and `Unwrap()`.
  - Typed enums: `Status`, `Code`, `FailureCode`, `AccountStatus`.
    Spec-byte-for-byte literals; `IsTerminal` / `IsSuccess` helpers.
  - `Amount = int64` alias and `Currency = string` alias.
  - `Environment` (`EnvTest` / `EnvPre` / `EnvProd`) with the spec's
    placeholder URLs and `BaseURL(env)` accessor.
  - `DefaultRequestID` — 32-char ULID-like hex idempotency-key
    generator (well under the spec's 64-char cap).
  - `Client.Close() error` — no-op stub for v0.1; reserved for v0.2
    background goroutine cleanup.

- **`atomefin/transport`** — exposed plumbing:
  - `RetryPolicy` with jittered exponential backoff (3 attempts,
    250ms × 2^n ± 20%, cap 4s, retries on 5xx + transport-level
    errors only).
  - `Logger` / `NopLogger` interface (slog-compatible shape).
  - `NewSlogLogger(*slog.Logger) Logger` — wraps slog with the SDK's
    PII redactor (`Authorization`, `sessionid`, shipping-* fields,
    `externalReferenceUid`, etc.).
  - `Observer` / `NopObserver` for metrics / tracing. Every Observer
    call site is wrapped in panic-recovery.
  - `BuildUserAgent(version, suffix)` for the default
    `atome-fin-go-sdk/0.1.0 (go1.22; goos/goarch)` header.

- **`atomefin/payment`** — outbound service:
  - `Service` with `Auth` / `Capture` / `VoidAuth` methods plus
    `*PollUntilTerminal` typed wrappers around the generic
    `PollUntilTerminal[T]` helper.
  - `payment.New(*atomefin.Client)` constructor (avoids the
    `client.Payment` import-cycle constraint; documented in
    `payment/doc.go`).
  - Full sub-type tree: `SubOrder`, `RequestExtendInfo`, `DeviceInfo`,
    `GeoPoint`, `DeviceProfile`, `BuildInfo`, `WifiAP`, `IPAddress`,
    `Address`, `ShippingAddress`, `PaymentRiskInfo`, `Platform`,
    `AccountChanges` (11 fields, signed deltas), `InstallmentDetail`,
    `InstallmentPlan`, `SubOrderInstallmentPlans`,
    `AuthExtendInfoResp`, `CaptureExtendInfoResp`, `AgreementRef`.
  - Position-scoped `AccountStatus` validation: `IsValidPreviousStatus`
    rejects `ACCOUNT_CLOSED`; `IsValidCurrentStatus` accepts.
  - `IsValidScore` for the `userCreditScore` 0..1 range.
  - Per-method client-side validation (sub-order sum mismatch,
    `requestId` length, `sessionid` length, etc.).

- **`atomefin/callback`** — inbound receivers:
  - `Verifier` with multi-cert slot (slice of `sign.Verifier`,
    succeeds on any).
  - `NewVerifier`, `FromClient`, `FromCertPEMs` constructors.
  - `AuthHandler` / `CaptureHandler` returning `http.Handler` with
    spec-aligned ack semantics: 200/`SUCCESS`, 401/`INVALID_SIGNATURE`,
    500/`SERVER_ERROR`, 400/`WRONG_PARAMS_FORMAT`, 405 on non-POST.
    Every response sets `Content-Type: application/json; charset=utf-8`
    and `X-Content-Type-Options: nosniff`.
  - `AuthEvent = payment.AuthResponse` and
    `CaptureEvent = payment.CaptureResponse` type aliases — partners
    do not learn a parallel callback schema.
  - 1 MiB default body cap (`WithBodyLimit`).

- **`qa/marshal`** — generic round-trip harness:
  - `GoldenRoundTrip[T]`, `StrictDecode[T]`, `AssertOmitemptyZero[T]`,
    `AssertRequiredEmits[T]`, `AssertAmountRoundtrip[T]`,
    `AssertRejectsFractionalAmount[T]`, `AssertAmountKeysAreInteger[T]`.
  - `AmountCorpus()` covering the full `int64` range plus negatives.

- **`qa/testdata/`** — 24 wire-format fixtures covering every spec
  request, every spec response, every error envelope, and every
  callback shape.

- **`examples/`** — runnable demos:
  - `auth_capture/` — outbound `Client` → `/auth` → `/capture` flow.
  - `webhook_server/` — inbound callback receiver, single-cert and
    multi-cert (rotation overlap) modes.

- **Tooling**:
  - `Makefile` with `build`, `test`, `test-race`, `lint`, `vet`,
    `fmt`, `fmtcheck`, `cover`, `examples`, `sandbox-smoke` (gated
    behind `ATOME_FIN_RUN_SMOKE=1`), `sandbox-webhook`, `clean`.
  - GitHub Actions workflow (`go vet`, `go test -race`, `gofmt`,
    `go build ./examples/...`).
  - `golangci-lint` config.

### Known assumptions (track via `DESIGN.md` §13)

| Q | Assumption | Switch on |
|---|---|---|
| Q1 | Spec-placeholder base URLs | `atomefin.WithBaseURL` |
| Q2 | **PARTIALLY RESOLVED 2026-05-05** — algorithm locked to RSA-2048 / SHA-256 / PKCS#1 v1.5 / base64-standard via openssl vector at `atomefin/sign/testdata/`; wire-format of `Authorization` header value still partner-pending | `atomefin.WithAuthorizationScheme` |
| Q2b | **NEW 2026-05-05** — spec mandates a SEPARATE cert exchange for the salted-PSS variant; today's `sign.WithSaltedPSS` / `WithVerifierSaltedPSS` reuse the default keypair → not spec-compliant for PSS, doc-only gap (PKCS#1 v1.5 default path is unaffected) | v0.2: add `WithSaltedPSSPrivateKeyPEM` / `WithSaltedPSSAtomePublicCertPEM` |
| Q3 | No `keyId` header — cert rotation out-of-band | `callback.NewVerifier` multi-cert |
| Q4 | PKCS#1-v1.5 default; PSS via opt-in (see Q2b for separate-cert gap) | `sign.WithSaltedPSS` |
| Q5 | No timestamp / nonce header | `populateHeaders` extension |
| Q6 | Caller-managed `sessionid` lifecycle | n/a |
| ~~Q7~~ | **RESOLVED 2026-05-05** — partner identity = dedicated API URL + cert exchange; no partner header emitted | n/a |
| Q9 | No 429 / Retry-After handling | extend `RetryPolicy.RetryOnStatus` |
| Q11 | `billDate` / `dueDate` are pass-through strings | parse when TZ confirmed |
| Q22 | `originalAmount` is `int64` despite spec `type: number` | R11 catches drift |

### Coverage at v0.1.0 ship

| Package | Coverage |
|---|---|
| `atomefin/sign` | 95.9% |
| `atomefin/transport` | 92.7% |
| `atomefin/callback` | 88.6% |
| `atomefin` | 87.8% |
| `qa/marshal` | 76.4% |
| `atomefin/payment` | 73.8% |

[Unreleased]: https://github.com/atome-fin/atome-fin-go-sdk/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.1.0
