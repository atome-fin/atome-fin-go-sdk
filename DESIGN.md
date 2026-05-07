# atome-fin-go-sdk — Design Proposal

> Source spec: <https://doc.apaylater.net/white-label/G/> (Redoc renderer over
> `swagger.yaml`). Spec version is **`partner-order-draft`** /
> *Partner Order Auth-Capture API v1 (Draft)*. The spec itself is explicitly
> marked draft and several fields are placeholders, so this design must remain
> easy to evolve.

This document proposes a server-side Go SDK that a partner imports to integrate
with atome-fin's white-label "G" Auth–Capture–Void flow. It is an architecture
proposal only — no implementation has been started.

---

## 1. API surface (extracted from spec)

### 1.1 Endpoints

| Direction | Method | Path | Purpose |
|---|---|---|---|
| Outbound (partner → Atome) | POST | `/auth` | Freeze user credit limit |
| Outbound | POST | `/capture` | Settle order against a prior auth |
| Outbound | POST | `/voidAuth` | Cancel an unconsumed auth |
| Inbound  (Atome → partner) | POST | `<authNotifyUrl>` | Auth terminal-state callback |
| Inbound  | POST | `<captureNotifyUrl>` | Capture terminal-state callback |

### 1.2 Environments

| Env | Base URL (placeholder per spec) |
|---|---|
| Test | `https://id-api.apaylater.net/white-label/G` |
| Pre-production | `https://id-api-pre.apaylater.net/white-label/G` |
| Production | `https://api.atome.id/white-label/G` |

The spec annotates all three URLs as **placeholders to align with gateway
routing before go-live** — see Open Questions §10.

### 1.3 Auth / signing scheme

> **Spec text (verbatim, Signature section)**:
>
> > We use **RSA2(2048 bit)** encryption algorithm, and through the
> > exchange of public key certificate to verify the signature.
> > Encrypt the signature with salt if necessary, and in this
> > condition we should exchange another public key certificate.
> >
> > ATOME provide: Public key certificate: validity 3 years
> > Partner provide: Public key certificate: validity 3 years
> >
> > **Params**
> > Request Headers: add param `Authorization`
> > GET: Sign the request parameters which parameter names are
> > sorted in alphabetical natural order
> > POST: Sign with the payload.

- Algorithm: **RSA2(2048 bit)** per the spec quote above. Concretely
  this is **RSA-2048 modulus / SHA-256 digest / RSASSA-PKCS#1 v1.5
  padding / base64-standard signature encoding** — the byte-for-byte
  output of `openssl dgst -sha256 -sign privkey.pem` (libcrypto's
  default padding for `RSA_sign` is PKCS#1 v1.5). This is the
  prod-default path and is anchored by an external openssl test
  vector at `atomefin/sign/testdata/external_*` (see §4.1).
- **"Encrypt the signature with salt if necessary"** was originally
  read as a reference to RSA-PSS (v0.1 — v0.6 shipped opt-in
  `sign.WithSaltedPSS` / `sign.WithVerifierSaltedPSS` scaffolding).
  v0.3's `SIGN_VERIFY_ENCRYPT_REVIEW.md` re-read the phrasing as
  describing the encrypt-cert pair (the AES-key wrap path on the
  two credit POSTs — see §16). Atome engineering subsequently
  confirmed that the gateway only supports **PKCS#1 v1.5** signing.
  v0.7.0 (2026-05-07) removed the PSS code path entirely; signing
  is single-scheme.
- The wire is **signature-only**: there is no body encryption, no
  AES envelope, no field-level encryption. Confidentiality is
  provided by TLS at the transport layer; the signature provides
  authenticity + integrity.
- Two-way trust: Atome and partner each issue a public key certificate (3-year
  validity). Partner signs outbound calls with **partner private key**; Atome
  verifies with **partner public cert**. Atome signs callbacks with **Atome
  private key**; partner verifies with **Atome public cert**.
- Header: `Authorization: <signature>` on every request **and** every callback.
- Canonical input:
  - **POST**: sign over the JSON request body (raw bytes after marshal).
  - **GET**: sign the query parameters with names sorted alphabetically.
    (Currently no GET endpoints, kept for forward compatibility.)
- `/auth` additionally requires a `sessionid` header (≤64 chars), described as
  an "opaque session token for this checkout or authorization flow".

### 1.4 Idempotency model

- Every outbound request carries `requestId` (≤64 chars) inside the JSON body —
  this is the **business-level idempotency key**, not a header.
- Sync semantics:
  - First submission of a `requestId`: HTTP 200 with `data.status =
    PROCESSING`. Final outcome will arrive via callback.
  - Idempotent retry of the same `requestId` **after** terminal state: HTTP
    200 with `data.status = SUCCESS | FAILED` (the same terminal payload that
    was/will be sent on the callback).
- Callbacks fire **only** at terminal states. They never carry `PROCESSING`.
- `/capture` rules: same `authOrderId`, `totalAmount`, `periodType`, and
  identical `subOrders` set as the original `/auth`. Uplift, partial capture,
  and re-allocation are not supported in v1.

### 1.5 Money & locale

- Amounts are integers in **the smallest currency unit** of the configured
  country (Rupiah / centavo / etc.).
- `currency` is ISO-4217. Supported set is agreed per partner/country.
- `periodType` is an integer installment tenor; spec lists `1, 3, 6, 9, 12`.
- Time fields:
  - `version` and `extendInfo.reapplyTime`: Unix milliseconds.
  - `billId`: `yyyyMM`.
  - `billDate`, `dueDate`: `yyyy-MM-dd` (timezone unspecified — see §10).

### 1.6 Error model

| HTTP | Schema | Codes |
|---|---|---|
| 200 | `*AckResponse` | Business `code` + `data.status`. Business codes that may surface in 200: `SUCCESS`, `INVALID_ORDER_AMOUNT`, `CREDIT_APPLICATION_NOT_APPROVED`, `ACCOUNT_BLOCKED_OVERDUE`, `ACCOUNT_BLOCKED`, `ACCOUNT_TEMP_BLOCKED`, `FEE_CHANGE`, `USER_CREDIT_LIMIT_INSUFFICIENT` (capture only) |
| 400 | `PaymentStyle400Error` / `Capture400Error` / `Void400Error` | `PARAMS_MISSING`, `WRONG_PARAMS_FORMAT`, `PARAMS_WRONG`, `NOT_FOUND`, `SESSION_NOT_FOUND`, `CAPTURE_AMOUNT_EXCEED`, `AUTH_EXPIRED` |
| 401 | `AuthorizationErrorResponse` | `INVALID_SIGNATURE` |
| 500 | `ServerErrorResponse` | `SERVER_ERROR` (spec recommends retry up to 3× with backoff) |

`failureCode` (terminal `FAILED`): `USER_CREDIT_LIMIT_INSUFFICIENT`, `RISK_REJECT`.
Account-status enum: `NORMAL`, `ACCOUNT_BLOCKED_OVERDUE`, `ACCOUNT_BLOCKED`, `ACCOUNT_CLOSED`.

### 1.7 Things the spec does **not** define

- No `Idempotency-Key` HTTP header (idempotency lives in body `requestId`).
- No timestamp/nonce header for replay protection (see §10).
- No `keyId` / `keyVersion` field for rotating signing certs (see §10).
- No documented rate limits.
- No AES envelope or field-level encryption — only RSA signing.
- No pagination (no list endpoints in v1).

---

## 2. Module path & packaging

Proposed Go module path:

```
github.com/atome-fin/atome-fin-go-sdk
```

(Final org/path TBD with the partner — see §10.) Module `go.mod` pins
`go 1.22` so generics, `errors.Join`, and `net/http.ServeMux` patterns are
available; no third-party HTTP client; only stdlib + `crypto/rsa`,
`crypto/x509`, `encoding/pem`.

### 2.1 Package layout

```
atome-fin-go-sdk/
├── go.mod
├── atomefin/                  # umbrella; Client lives here
│   ├── client.go               # Client, Option, New
│   ├── doer.go                 # http.RoundTripper plumbing
│   ├── errors.go               # APIError, TransportError, SignatureError
│   ├── codes.go                # Code / Status / FailureCode enum types
│   ├── money.go                # Minor-unit Amount type, currency helpers
│   └── version.go              // SDK version constant for User-Agent
│
├── atomefin/payment/          # /auth, /capture, /voidAuth
│   ├── payment.go              # Service struct hung off Client
│   ├── auth.go                 # AuthRequest/AuthResponse, (*Service).Auth
│   ├── capture.go              # Capture
│   ├── void.go                 # VoidAuth
│   └── types.go                # SubOrder, ExtendInfo, AccountChanges, etc.
│
├── atomefin/callback/         # webhook receiver helpers
│   ├── verifier.go             # Verifier{} (RSA2 cert, salt mode, body limit)
│   ├── handler.go              # http.Handler wrappers per event
│   ├── events.go               # AuthEvent, CaptureEvent (typed)
│   └── ack.go                  # CallbackAckResponse helper
│
├── atomefin/sign/             # signing primitives (no service deps)
│   ├── signer.go               # Signer interface + RSA2 implementation
│   ├── canonical.go            # canonical string for POST body / GET query
│   └── pem.go                  # LoadPrivateKeyPEM / LoadPublicCertPEM
│
├── atomefin/transport/        # internal HTTP machinery (exposed for tests)
│   ├── retry.go                # Retry policy w/ jittered backoff
│   ├── logging.go              # Hook interface
│   └── useragent.go
│
└── examples/
    ├── auth_capture/           # canonical happy-path script
    └── webhook_server/         # net/http callback receiver
```

Sub-packages keep service surface small and let partners depend on
`callback` without pulling outbound transport, and vice versa.

---

## 3. `Client` construction

`Client` is the only stateful root. It is created via `atomefin.New(opts...)`
returning `(*Client, error)`. Functional options pattern, all optional unless
noted.

```go
type Option func(*config) error

func WithHTTPClient(h *http.Client) Option           // default: cloned http.DefaultClient with 30s timeout
func WithBaseURL(u string) Option                    // default: production
func WithEnvironment(env Environment) Option         // EnvTest / EnvPre / EnvProd
func WithTimeout(d time.Duration) Option             // per-request timeout
func WithLogger(l Logger) Option                     // structured, PII-redacting
func WithSigner(s sign.Signer) Option                // REQUIRED unless WithPrivateKeyPEM is set
func WithPrivateKeyPEM(pem []byte) Option            // shorthand: builds default RSA2 Signer
func WithAtomePublicCertPEM(pem []byte) Option       // for verifying sync error responses & callbacks
func WithKeyID(id string) Option                     // sent as header once §10/Q3 is resolved
func WithPartnerID(id string) Option                 // identifier sent in headers / used for logs
func WithMerchantID(id string) Option                // optional, defaults to partner-level
func WithRetry(p RetryPolicy) Option                 // default: 3 attempts on 5xx + transport errors
func WithUserAgent(ua string) Option                 // appended to default "atome-fin-go-sdk/<ver>"
func WithClock(now func() time.Time) Option          // testability
func WithRequestIDGenerator(fn func() string) Option // default: ULID
func WithObserver(o Observer) Option                 // hook: OnRequest / OnResponse / OnRetry
```

Required to construct a Client successfully:
- a `Signer` (or PEM convenience) — outbound calls must be signed,
- an `Environment` **or** explicit `BaseURL`,
- `PartnerID` (used in log fields and any future header — keep mandatory now to
  avoid migration churn later).

Sub-services hang off the client:

```go
client.Payment.Auth(ctx, req)
client.Payment.Capture(ctx, req)
client.Payment.VoidAuth(ctx, req)
```

Each returns a typed response and an `error` that callers can inspect with
`errors.As`.

---

## 4. Signing helper (`atomefin/sign`)

```go
type Signer interface {
    Sign(ctx context.Context, canonical []byte) (signature string, err error)
    KeyID() string // optional, may return ""
}

type Verifier interface {
    Verify(ctx context.Context, canonical, signature []byte) error
}

func NewRSA2Signer(priv *rsa.PrivateKey, opts ...SignerOption) Signer
func WithKeyID(id string) SignerOption

func LoadPrivateKeyPEM(pem []byte, password ...[]byte) (*rsa.PrivateKey, error)
func LoadPublicCertPEM(pem []byte) (*rsa.PublicKey, error)
```

Canonical string rules (per spec §Signature, §1.3 verbatim quote):

- **POST**: `Sign(body)` — bytes exactly as transmitted (after
  `json.Marshal`, no rewriting, no whitespace normalization). Spec
  text: *"POST: Sign with the payload."*
- **GET**: `Sign(CanonicalQuery(params))` — keys sorted in
  lexicographic / "alphabetical natural" order, `k=v` joined by `&`,
  values URL-encoded per RFC 3986. Spec text: *"GET: Sign the
  request parameters which parameter names are sorted in
  alphabetical natural order"*. Exposed as
  `sign.CanonicalQuery(url.Values)` for partners building extension
  calls.

> **GET path is now first-class as of v0.2** — the SDK exposes
> `Client.DoSignedGET(ctx, path, query, opts...)` for partners
> integrating the `/query-auth` / `/query-capture` / `/query-voidAuth`
> polling endpoints. `DoSignedGET` signs `sign.CanonicalQuery(query)`
> and assigns the SAME bytes to `req.URL.RawQuery` so the wire is
> byte-equal to the signing canonical (R13 invariant — exercised by
> `atomefin/dosigned_get_test.go`'s `TestDoSignedGET_R13_WireEqualsCanonical`).
> The typed wrappers `payment.QueryAuth` / `QueryCapture` / `QueryVoidAuth`
> consume this path; partners writing extension calls can call
> `DoSignedGET` directly. `Client.DoSigned` remains POST-only and
> returns `*ValidationError` on non-POST verbs.

Output is base64-standard-encoded; placed verbatim in the `Authorization`
header. We do not invent a custom auth scheme prefix until §10/Q2 is resolved.

Verifier mirrors the signer for callback handlers and (defensively) for the
partner who wants to sanity-check Atome's `/auth` HTTP-200 envelope.

#### 4.1 RSA2 interpretation — locked to openssl vector

The spec says "RSA2(2048 bit)" + "Encrypt the signature with salt if
necessary" (see §1.3 verbatim quote). Concretely the prod-default
this resolves to is:

- **RSA-2048 modulus**
- **SHA-256 digest**
- **RSASSA-PKCS#1 v1.5 padding** — the only scheme the gateway
  accepts. (Earlier SDK versions exposed PSS as an opt-in
  alternative; v0.7.0 removed it after Atome engineering
  confirmed PKCS#1 v1.5 is the only supported scheme — see
  CHANGELOG `## [0.7.0]` and §13/Q4 below.)
- **base64-standard signature encoding** (no URL-safe substitution,
  padding preserved)
- placed verbatim in the `Authorization` header

This is exactly the byte-for-byte output of
`openssl dgst -sha256 -sign privkey.pem`. `NewRSA2Signer` /
`NewRSA2Verifier` lock to this interpretation; there is no
toggle.

The interpretation is **anchored by an external openssl vector**
committed at `atomefin/sign/testdata/external_*`:

| File | Purpose |
|---|---|
| `external_priv.pem`  | RSA-2048 PRIVATE KEY (PKCS#1 PEM) |
| `external_pub.pem`   | matching SubjectPublicKeyInfo PEM |
| `external_body.json` | fixed signing-canonical bytes (compact JSON, no trailing newline) |
| `external_sig.b64`   | openssl-produced base64-standard signature |
| `README.md`          | regen command + algorithm fingerprint |

`atomefin/sign/external_vector_test.go` enforces four invariants:

1. The Go verifier ACCEPTS the openssl signature → SDK and reference
   agree on the algorithm.
2. The Go signer over the same key + body produces the IDENTICAL
   base64 string → PKCS#1 v1.5 is deterministic, so any drift is
   wire-incompatible.
3. Flipping any byte of the body causes the verifier to return
   `sign.ErrSignature`.

**Q2 — partial resolution (2026-05-05):** the wire-level Authorization
*format* (raw base64 vs. structured `Algorithm=RSA2,Sign=…`) is still
partner-pending and overridable via `WithAuthorizationScheme`. But
the *algorithm itself* — RSA-2048 / SHA-256 / PKCS#1 v1.5 — is now
locked. A future second vector from the partner's reference
implementation (P3) would close Q2 fully.

Verifier mirrors the signer for callback handlers and (defensively) for the
partner who wants to sanity-check Atome's `/auth` HTTP-200 envelope.

#### 4.2 ~~PSS-salted variant~~ (REMOVED v0.7.0)

The "Encrypt the signature with salt if necessary" spec phrasing
was originally read as a reference to RSA-PSS, and v0.1 — v0.6
shipped opt-in `WithSaltedPSS` / `WithVerifierSaltedPSS`
scaffolding behind that reading. v0.3's
`SIGN_VERIFY_ENCRYPT_REVIEW.md` re-read the phrase as referring
to the encrypt-cert pair (the AES-key wrap path on the two
credit POSTs — see §16); Atome engineering subsequently
confirmed the gateway only supports **PKCS#1 v1.5** signing.
v0.7.0 removed the PSS code path entirely.

The signing path is now single-scheme — see §4.1. Partners
that catch on the `*sign.MultiValueQueryError` boundary or any
PSS option in v0.6.x callsites must drop those references; the
default scheme has not changed.

---

## 5. Typed request/response structs

One concrete struct per request/response, json-tagged, no `interface{}` /
`map[string]any` on the public surface. Optional fields use pointer or `omitempty`
so they round-trip cleanly.

```go
package payment

type Status string
const (
    StatusProcessing Status = "PROCESSING"
    StatusSuccess    Status = "SUCCESS"
    StatusFailed     Status = "FAILED"
)

type Code string
const (
    CodeSuccess                     Code = "SUCCESS"
    CodeInvalidOrderAmount          Code = "INVALID_ORDER_AMOUNT"
    CodeCreditApplicationNotApproved Code = "CREDIT_APPLICATION_NOT_APPROVED"
    CodeAccountBlockedOverdue       Code = "ACCOUNT_BLOCKED_OVERDUE"
    CodeAccountBlocked              Code = "ACCOUNT_BLOCKED"
    CodeAccountTempBlocked          Code = "ACCOUNT_TEMP_BLOCKED"
    CodeFeeChange                   Code = "FEE_CHANGE"
    CodeUserCreditLimitInsufficient Code = "USER_CREDIT_LIMIT_INSUFFICIENT"
)

type FailureCode string
const (
    FailureCreditLimitInsufficient FailureCode = "USER_CREDIT_LIMIT_INSUFFICIENT"
    FailureRiskReject              FailureCode = "RISK_REJECT"
)

type AuthRequest struct {
    RequestID            string     `json:"requestId"`
    ExternalReferenceUID string     `json:"externalReferenceUid"`
    TotalAmount          int64      `json:"totalAmount"`
    PeriodType           int        `json:"periodType"`
    SubOrders            []SubOrder `json:"subOrders"`
    ExtendInfo           *ExtendInfo `json:"extendInfo,omitempty"`
}

type AuthResponse struct {
    Code    Code               `json:"code"`
    Message string             `json:"message"`
    Data    *AuthorizationData `json:"data,omitempty"`
}

type AuthorizationData struct {
    RequestID                string                     `json:"requestId"`
    Currency                 string                     `json:"currency"`
    AuthOrderID              string                     `json:"authOrderId"`
    TotalAmount              int64                      `json:"totalAmount"`
    Status                   Status                     `json:"status"`
    FailureCode              FailureCode                `json:"failureCode,omitempty"`
    SubOrderInstallmentPlans []SubOrderInstallmentPlans `json:"subOrderInstallmentPlans,omitempty"`
    AccountChanges           *AccountChanges            `json:"accountChanges,omitempty"`
    ExtendInfo               *AuthExtendInfoResp        `json:"extendInfo,omitempty"`
}
```

(`Capture*` / `Void*` follow the same pattern; `SubOrder`, `ExtendInfo`,
`InstallmentDetail`, `InstallmentPlan`, `AccountChanges` live in `types.go`.)

Helper methods for ergonomics:

```go
func (r *AuthResponse) IsTerminal() bool   // Status == SUCCESS || Status == FAILED
func (r *AuthResponse) IsProcessing() bool // Status == PROCESSING
```

Amount type strategy: use `int64` directly with `atomefin.Amount` as a
type alias, plus `atomefin.MinorUnitFromMajor(major, currency)` helpers
that resolve the right scale per currency. This keeps JSON wire format
unchanged (still a plain integer) but gives partners a typed entry point.

---

## 6. Error type hierarchy

```go
// atomefin/errors.go
type Error interface {
    error
    Temporary() bool
}

// 4xx / 5xx with parsed body
type APIError struct {
    HTTPStatus int
    Code       string         // mapped to typed Code where possible
    Message    string
    RequestID  string         // server-echoed if present
    Endpoint   string
    Raw        json.RawMessage
}

// transport / network / serialization
type TransportError struct {
    Op    string // "do", "marshal", "unmarshal"
    URL   string
    Err   error
    Retry bool
}

// signature verification (sync errors AND callback verification)
type SignatureError struct {
    Reason string // "decode", "verify", "missing-header"
    Err    error
}

// validation failures detected client-side before transmission
type ValidationError struct {
    Field   string
    Message string
}
```

Conventions:
- `errors.As` is the canonical way to type-switch.
- `APIError.Temporary()` is true for HTTP 500/502/503/504 and `SERVER_ERROR`.
- `TransportError.Temporary()` is true for `net.Error.Timeout()`, EOF on
  retryable verbs, and `context.DeadlineExceeded` while parent ctx is alive.
- All errors implement `Unwrap()` so callers can use `errors.Is(err, io.EOF)` etc.

---

## 7. Context, retries, and idempotency keys

- Every Service method takes `ctx context.Context` first.
- The HTTP layer respects `ctx.Done()` and propagates the deadline; per-request
  timeout from `WithTimeout` is applied via `context.WithTimeout` if shorter.
- **Retry policy** lives in `transport.RetryPolicy`:
  - Default: max 3 attempts (1 initial + 2 retries),
  - Trigger conditions: `TransportError.Retry == true` **or** HTTP 500 with
    code `SERVER_ERROR`. Never on 4xx (which includes `INVALID_SIGNATURE`).
  - Backoff: jittered exponential (250ms × 2^n ± 20%, cap 4s).
- **Idempotency** is preserved across retries because the same `RequestID`
  and same JSON body are reused — the spec guarantees Atome will return the
  prior terminal result for identical `requestId`.
- `RequestIDGenerator` defaults to ULID (sortable, 26-char base32) so it fits
  the spec's `maxlength:64` and is partner-collision-safe by default. Partners
  can supply their own generator if they want to embed their order ID prefix.

A small helper, `payment.PollUntilTerminal(ctx, client, req)`, wraps "submit
once → if PROCESSING, retry with the same requestId on a backoff until terminal
or ctx expires". This is the pragmatic equivalent of a status-poll endpoint
since none exists.

---

## 8. Webhook receiver helpers (`atomefin/callback`)

The same JSON shape as `AuthAckResponse` / `CaptureAckResponse` is delivered
to partner-hosted endpoints. Helpers:

```go
type Verifier struct {
    PublicKey   *rsa.PublicKey
    BodyLimit   int64               // default 1 MiB; reject larger to prevent abuse
    Clock       func() time.Time    // for replay-window checks if §10/Q5 adds a timestamp
}

func (v *Verifier) Verify(headers http.Header, body []byte) error

type AuthHandlerFunc    func(ctx context.Context, e *payment.AuthResponse) error
type CaptureHandlerFunc func(ctx context.Context, e *payment.CaptureResponse) error

func AuthHandler(v *Verifier, fn AuthHandlerFunc) http.Handler
func CaptureHandler(v *Verifier, fn CaptureHandlerFunc) http.Handler
```

Behavior:
- Read body with `io.LimitReader` — required because we sign over the raw
  bytes and must defend against unbounded request bodies.
- Verify signature **before** decoding JSON. Failure → respond `401` with
  `AuthorizationErrorResponse{Code: "INVALID_SIGNATURE"}`.
- After successful handler return, respond
  `CallbackAckResponse{Code: "SUCCESS", Message: "ack"}` (HTTP 200) so Atome
  stops retrying.
- If the handler returns an error, return HTTP 500 so Atome retries (per spec
  "Atome may retry the callback if a non-2xx response is returned"). The
  partner is responsible for keeping the handler **idempotent** (callbacks are
  at-least-once).

Callbacks reuse `payment.*Response` types so partners don't learn a parallel
schema.

---

## 9. Pagination

Not applicable in v1 — no list endpoints. The package layout leaves
`atomefin/transport/pagination.go` reserved for when reconciliation /
list APIs land.

---

## 10. Observability

- `Logger` interface with `Debug/Info/Warn/Error(msg string, kv ...any)`
  semantics, intentionally compatible with `log/slog`. Default is a no-op
  logger; partners adapt their own.
- **PII redaction is enforced by the SDK, not the partner.** A redaction
  layer scrubs known-sensitive fields before logging:
  - `Authorization` header → always redacted
  - `extendInfo.address.*`, `shippingName`, `shippingPhoneNo`,
    `extendInfo.deviceInfo.gps`, `extendInfo.deviceInfo.device.*`,
    `externalReferenceUid` → redacted by default; opt-in flag to log them in
    debug builds only.
  - Request/response body: log size + status, not body, unless
    `WithDebugBodyLogging(true)` is set.
- `Observer` hook lets partners stream metrics:
  ```go
  type Observer interface {
      OnRequest(ctx context.Context, op string, attempt int)
      OnResponse(ctx context.Context, op string, status int, dur time.Duration)
      OnRetry(ctx context.Context, op string, attempt int, err error)
  }
  ```
- `RequestID` (HTTP header `X-Request-Id` if/when Atome adopts one) is
  threaded into every log line with key `request_id`.

---

## 11. Testing strategy

1. **Unit tests** with `httptest.NewServer` per endpoint. The server fixture
   parses the `Authorization` header, verifies it against a test public key,
   and replies with golden JSON per scenario (PROCESSING, SUCCESS, FAILED,
   each 4xx code, 500).
2. **Golden files** under `testdata/` for every request body and every
   response variant. Marshal/unmarshal round-trip tests assert byte equality.
3. **Mock signer / verifier**: `signtest.StaticSigner{Sig: "DEADBEEF"}` and
   `signtest.PermissiveVerifier{}` so service tests don't need real keys.
4. **Property tests** (Go fuzz) for canonical-query construction and JSON
   amount encoding (no float coercion, no scientific notation).
5. **Integration test harness**: a `make sandbox-smoke` target running the
   `examples/auth_capture/` flow against the test environment, gated behind
   env vars `ATOME_FIN_PRIV_KEY_PEM`, `ATOME_FIN_ATOME_CERT_PEM`,
   `ATOME_FIN_PARTNER_ID`.
6. **Webhook tests**: a synthetic Atome callback signer in `callback/internal`
   that produces signed bodies, exercised against `AuthHandler` /
   `CaptureHandler` using `httptest.NewRecorder`.

---

## 12. Versioning & release policy

- **Module path** carries a major-version suffix once we hit v2:
  `github.com/atome-fin/atome-fin-go-sdk/v2/...`.
- Pre-1.0 (`v0.x`): minor versions may break — the upstream API is itself
  marked draft. Tag `v0.1.0` for the first complete `auth/capture/voidAuth`
  cut.
- `v1.0.0` cut **only** after the upstream spec drops "Draft" and the open
  questions in §13 are closed.
- Semver post-1.0: backward-compatible additions (new optional fields, new
  helper methods) are minor; renames or removed fields are major.
- Each tag ships a `CHANGELOG.md` entry and a verified `go.sum`.
- We commit to supporting the **two latest stable Go releases** at any time.

---

## 13. Open questions for the partner / Atome

1. **Final base URLs.** The spec marks all three server URLs as placeholders
   "to align with gateway routing before go-live". Production host
   `api.atome.id` differs from doc host `apaylater.net`; confirm the
   resolution and whether routing is per-country (e.g. `id-api.*` for
   Indonesia only).
2. **`Authorization` header format.** Is it the raw base64 RSA signature, or a
   prefixed scheme (e.g. `Algorithm=RSA2,KeyVersion=1,Sign=…`)? The spec just
   says "RSA2 signature over the POST JSON body".

   **PARTIALLY RESOLVED 2026-05-05.** The *algorithm* is locked to
   RSA-2048 / SHA-256 / PKCS#1 v1.5 / base64-standard via the openssl
   vector at `atomefin/sign/testdata/external_*` — see §4.1. The
   *wire format* of the header value (raw base64 vs. structured
   prefix) is still partner-pending; the SDK ships `SchemeRawBase64`
   as the default and `SchemeAtomeKeyed` as a one-line override target
   via `WithAuthorizationScheme`. A second openssl vector from the
   partner's reference implementation will close the wire-format
   half (P3 in Task #17).

2b. ~~**PSS-salted variant — separate cert exchange.**~~
   **MOOT (2026-05-07, v0.7.0).** This Q was opened on the
   reading that the spec's "Encrypt the signature with salt if
   necessary" phrasing referred to RSA-PSS as a parallel signing
   path. v0.3's `SIGN_VERIFY_ENCRYPT_REVIEW.md` re-read the
   phrase as describing the encrypt-cert pair (the AES-key wrap
   on the two credit POSTs — see §16); Atome engineering
   subsequently confirmed the gateway only supports
   PKCS#1 v1.5 signing. The v0.6.x `WithSaltedPSS` /
   `WithVerifierSaltedPSS` opt-ins were removed in v0.7.0; no
   second keypair, no separate cert exchange, no toggle.
3. **Key rotation.** How is the active partner cert / Atome cert identified?
   No `keyId`/`keyVersion` field is documented. Without one we cannot rotate
   without downtime.
4. ~~**PSS vs PKCS#1 v1.5.**~~ **REMOVED (2026-05-07, v0.7.0).**
   Atome engineering confirmed the gateway supports only PKCS#1
   v1.5; the "salt if necessary" phrasing was reframed in v0.3
   as the encrypt-cert pair (§16), not PSS. v0.7.0 removed the
   PSS code path entirely — `WithSaltedPSS` /
   `WithVerifierSaltedPSS` are gone. The signing scheme is now
   single-source: PKCS#1 v1.5 over SHA-256, anchored by the
   §4.1 openssl vector.
5. **Replay protection.** No timestamp/nonce header is required. Should we
   add one (e.g. `X-Atome-Timestamp` + `X-Atome-Nonce` covered by the
   signature) to prevent replay? Particularly important for inbound callbacks.
6. **`sessionid` lifecycle.** Required only on `/auth`. Where is it minted
   (presumably at SDK/checkout init), what is its TTL, and is it bound to
   `externalReferenceUid`?
7. **Partner / merchant identification at transport.** ~~Nothing in the headers
   identifies the partner — is this implicit in the signing key, or do we
   need an `X-Partner-Id` / `X-Merchant-Id` header?~~ **RESOLVED
   2026-05-05** — partner identity is established by the dedicated API
   URL (per environment) plus the RSA certificate exchange; there is no
   partner-identifying header on the wire. The SDK does not emit
   `X-Partner-Id` / `X-Merchant-Id`. `WithPartnerID` / `WithMerchantID`
   stay supported as log-enrichment hooks only.
8. **Callback retry policy.** Spec says "retry policy to be aligned with
   operations". Need: max attempts, backoff curve, retry window, dedupe
   semantics, success criterion (HTTP 2xx alone, or also `code: SUCCESS` in
   body?).
9. **Rate limits.** Not documented. Need numeric limits per endpoint and the
   429 / "rate limited" error contract.
10. **Currency set.** ~~Spec says supported currencies are agreed per partner;
    we need the concrete list to validate `Currency` and pick `Amount`
    minor-unit scale.~~ **RESOLVED 2026-05-06** — the spec enum-locks
    the supported set to **IDR** (Indonesian rupiah) for v0.1.1. The
    SDK promotes `atomefin.Currency` from a string alias to a named
    type with `CurrencyIDR` constant and `(Currency).IsValid()` helper.
    Decode policy is permissive (any string accepted on inbound for
    forward-compat with v2 currencies); validators on outbound request
    types reject non-IDR via `IsValid` before the network round-trip.
11. **Time zone for `billDate` / `dueDate`** (`yyyy-MM-dd`). Bills and
    dueDates without a TZ are ambiguous near midnight — is it user-local TZ,
    Atome operational TZ, or UTC?
12. **`paymentRiskInfo` shape.** Spec defines it as an empty object — what
    fields are required for partners using the risk module?
13. **`skuId` mandate.** Spec says "whether `skuId` is mandatory is agreed
    during integration." Is this a per-merchant config or a per-country rule?
14. **`agreementUrl` PII.** Loan agreement URL appears in success responses
    and presumably is short-lived signed link — confirm so logging policy
    can treat it appropriately.
15. **`reapplyTime` semantics.** Wall-clock retry-after, or a hint that the
    user must re-onboard? Affects whether we expose it as `time.Time` or as
    raw ms.
16. **`periodType` enumeration.** Spec lists `1, 3, 6, 9, 12` as examples.
    Is the set fixed or merchant-configurable? Affects whether we make it a
    typed enum in Go.
17. **`subOrders[].periodType`.** Both the order and each sub-order have
    `periodType`. Must they match, or can sub-orders carry independent
    tenors (and if so, how do they reconcile with the order-level tenor)?
18. **HTTP 500 retry semantics.** Spec says "retry up to three times before
    giving up." Confirm interval and whether the partner is expected to
    re-sign each retry.
19. **Final go-module path.** Will the SDK live at
    `github.com/atome-fin/atome-fin-go-sdk`, `github.com/atome/...`, or a
    partner-owned org? Affects import path baked into examples and tests.
20. **Spec stability.** Spec is tagged `partner-order-draft` /
    `Draft from Auth-Capture` (2026-05-06). What is the schedule for the
    non-draft cut, and will fields like `version` (currently described as
    "monotonic version or event time in Unix ms") be tightened?

---

## 14. Suggested task breakdown

Subject to team-lead approval, the implementation can be sliced as:

- **T1 — `sign` package**: Signer, Verifier, PEM loaders, canonical query,
  unit tests + fuzz. (Independent of transport.)
- **T2 — `atomefin` core**: Client, options, errors, codes, money,
  environment selection, retry policy. Depends on T1.
- **T3 — `payment` service**: typed request/response, `Auth/Capture/VoidAuth`,
  `IsTerminal`/`PollUntilTerminal`. Depends on T2. Includes httptest-server
  table tests.
- **T4 — `callback` package**: Verifier, handlers, ack response. Depends on
  T1 + T3 type definitions.
- **T5 — Examples + sandbox smoke**: `examples/auth_capture/`,
  `examples/webhook_server/`, `make sandbox-smoke`. Depends on T2–T4.
- **T6 — Docs + CI**: `README.md`, `CHANGELOG.md`, GitHub Actions for
  `go test ./...`, `golangci-lint`, `govulncheck`. Cross-cutting.

QA scope (for `qa`):
- Verify struct tags match spec field names exactly (no case drift).
- Round-trip every spec example through marshal/unmarshal with byte equality.
- Confirm signing canonicalization matches Atome reference (once provided).
- Validate retry/backoff doesn't violate idempotency (same `requestId` across
  retries; never auto-mint a new one).
- Confirm callback handler returns 200 only on handler success and
  401/`INVALID_SIGNATURE` on bad signature.
- Static check: no PII fields appear in default log output.
- Check race/data-races under `go test -race` for the Client (concurrent
  callers).

## 15. v0.2 changes (appendix, 2026-05-06)

v0.2 ships **28 new endpoints across 6 chunks**, all pure additions
on top of the v0.1.x surface. The architecture stayed within the
shapes set out in §§1–8; this appendix records what's new and the
two design decisions worth flagging for future coders.

### 15.1 New sub-packages (mirror the §2 / §5 pattern)

Each ships its own `Service` struct constructed by `pkg.New(c)` (no
`client.X` accessor — preserves tree-shake), its own `types.go` /
`validation.go` / `service_test.go` / `marshal_audit_test.go`, and
imports `atomefin/payment` for any shared types it can borrow
without owning.

| Package | Endpoints | Notes |
|---|---|---|
| `atomefin/refund` | `/refund`, `/query-refund`, `<refundNotifyUrl>` | re-uses `payment.AccountChanges` (the 11-field shape) |
| `atomefin/bill` | `/bills`, `/billDetail`, `/billUnpaid` + `BillsAll` auto-pagination | GET-only; codified the **paginated-list pattern** (bare `json:"bills"`, no `,omitempty`) — empty pages round-trip as `[]` rather than dropping to `null` |
| `atomefin/transaction` | `/transactions`, `/transactionDetail` + `TransactionsAll` | GET-only; same paginated-list pattern on `TransactionsData.Items` |
| `atomefin/repayment` | `/repayment-request`, `/repayment-result`, `<repaymentNotifyUrl>` | uses **`CommerceAccountChanges`** — *not* `payment.AccountChanges` (see §15.3) |
| `atomefin/credit` | `/credit-information`, `/credit-application`, `/credit-result`, `/credit-information-result`, `/query-balance-history`, `/modify-application-info`, `/close-account`, plus 2 callbacks | account-ops co-housed by domain cohesion (see §15.4); `INPROGESS` literal preserved verbatim from spec |

`atomefin/payment` also gained `PaymentPreCheck` + `PaymentPlan`
(pre-checkout chunk #7) and the `Query*` GETs from chunk #1.

### 15.2 GET handling pattern (`Client.DoSignedGET`)

§5's GET note is now first-class: `Client.DoSignedGET(ctx, path,
query, opts...)` is the GET-equivalent of `DoSigned`, sharing a
private `signAndDispatch` helper with the POST path. The R13
invariant — wire query bytes ≡ signing canonical bytes — is
enforced by assigning `sign.CanonicalQuery(query)` directly to
`req.URL.RawQuery` so `+`-vs-`%20` drift cannot happen. Pinned by
`TestDoSignedGET_R13_WireEqualsCanonical` and re-asserted at scale
(6+ params) by the bill / transaction stress tests. All paginated
GETs (bill, transaction, query-balance-history) and all `Query*`
endpoints route through `DoSignedGET`.

`Client.HeartBeat(ctx) error` — added in chunk #9 — is the umbrella
shortcut for the empty-canonical case (`GET /heart-beat`). It signs
zero bytes (`sign.CanonicalQuery(nil) == ""`) and verifies cleanly.

### 15.3 Credit-change vectors

The spec defines two credit-change wire shapes, each modelled as
its own Go type:

- **`payment.AccountChanges`** is the canonical credit-change
  vector for `/auth`, `/capture`, `/voidAuth`, `/refund` responses
  and the `<accountChangeNotifyUrl>` callback. 11 fields including
  `frozenCreditChange`.
- **`repayment.CommerceAccountChanges`** is the canonical credit-
  change vector for commerce-domain responses (`/repayment-request`,
  `/repayment-result`, `<repaymentNotifyUrl>`).

Each carries the field set defined by the spec for its respective
endpoint family. The distinction is documented in
`atomefin/repayment/types.go`'s package comment and surfaced in the
README package map.

### 15.4 Account-ops live in `atomefin/credit/`

`/modify-application-info` and `/close-account` both operate on a
credit-application identifier and share the same response-envelope
shape with the credit-result endpoints. They live in
`atomefin/credit/` alongside the lifecycle endpoints.

## 16. v0.3 hybrid encryption (appendix, 2026-05-07)

Q31 — Q34 RESOLVED 2026-05-06. `/credit-information` and
`/credit-application` POSTs require an AES-ECB-PKCS5 +
RSA-PKCS#1 v1.5 hybrid envelope; every other endpoint stays
plaintext.

### 16.1 Outbound: encrypt-then-sign

Per the partner reference at `~/Downloads/main.go` line 28-30, the
ordering is:

```
plaintext body
   │
   ├── AES-ECB-PKCS5(plaintext, fresh32CharKey)  →  encrypted body (base64)
   │                                                       │
   ├── RSA-PKCS#1-v1.5(fresh32CharKey, atomeEncryptPub)   ─┘
   │      → urlEncoded(base64(...))  → "Encrypt: symmetricKey=<...>"
   │
   └── Authorization: base64(SHA256WithRSA(<encrypted body>))   ← SIGNS THE CIPHERTEXT
```

Critical: the signing canonical is the **encrypted** body bytes,
NOT the plaintext. `Client.DoEncryptedSigned` (`atomefin/doer.go`)
embodies this ordering. The fresh AES key is generated once per
call (not per retry) so signature + body remain stable across
PKCS#1 v1.5's deterministic retry loop.

### 16.2 Two cert pairs — by design

The protocol uses TWO distinct RSA keypairs:

| Pair | Used for | Configured via |
|---|---|---|
| **Signing** (existing in v0.1) | `Authorization: <signature>` over the request body. Mutual: SDK signs outbound; verifier checks inbound callbacks. | `WithPrivateKeyPEM` + `WithAtomePublicCertPEM` |
| **Encrypt** (new in v0.3) | RSA-wrap the per-request AES key. Atome's encrypt public key is the wrap target; the partner's encrypt private key would unwrap inbound encrypted bodies (Q31 leaves credit callbacks plaintext today, so v0.3 has no inbound caller). | `WithEncryptAtomePublicCertPEM` + `WithEncryptPrivateKeyPEM` |

Distinct keys, separate rotation cadence — the partner agreement
mandates the split (Q34). Min 2048-bit on each pair. The SDK
enforces the floor inside `WithEncrypt*PEM` and inside
`encrypt.Wrap/UnwrapAESKey`.

### 16.3 ECB is partner-protocol-mandated, not the SDK's choice

Go's `crypto/cipher` deliberately omits an ECB BlockMode — ECB
leaks plaintext patterns at block boundaries and the Go authors
consider it dangerous for general use. The apaylater protocol
nonetheless requires ECB on this code path (mirrors Java's
`Cipher.getInstance("AES/ECB/PKCS5Padding")`).
`atomefin/encrypt/aes.go` walks the cipher block-by-block via
`aes.NewCipher`'s `Encrypt` / `Decrypt` — preserves Go's standard-
library invariant that ECB is opt-in-by-construction while still
implementing the mandated mode.

If the spec ever moves to AES-CBC (or any IV-bearing mode), the
`Encrypt: symmetricKey=<...>` header value's `k=v,k=v` shape
leaves room for an `iv=` field — `header.ParseEncryptHeader`
returns a `map[string]string` so callers can read additional
fields without an API churn.

### 16.4 Spec server header validation (v0.3)

`qa/specserver/walker.go` now collects required header
parameters in addition to query params. The dispatcher checks
header presence on every request; when an Encrypt header is
required AND present, body validation is bypassed because the
spec server has no decryption key — the wire body is opaque
ciphertext from the spec server's perspective. R-invariants on
the plaintext request shape continue to live in
`qa/marshal_audit_test.go`; the `encrypted_e2e_test.go` in
`atomefin/credit/` exercises the round-trip through a decrypting
httptest server.

The Authorization header is filtered from the walker's
RequiredHeader collection — every signed endpoint declares it
required, but signature validation is the sign package's job
(architect §1.7).
