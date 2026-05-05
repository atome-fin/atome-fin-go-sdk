# Changelog

All notable changes to atome-fin-go-sdk will be documented in this
file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
post-1.0. Pre-1.0 minor versions may break.

## [Unreleased]

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
