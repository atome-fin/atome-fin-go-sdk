# Testing without the live atome-fin gateway

This page is for partners who need to write tests against the SDK
**without** hitting the real upstream — local CI, offline dev,
fault-injection drills, that kind of thing.

The SDK is intentionally `httptest`-friendly. Two patterns work
today against the v0.3.x surface; a richer `atomefin/mock`
sub-package is on the v0.4 / v0.5 roadmap (see [Roadmap](#roadmap)
below).

---

## Pattern A — `WithHTTPClient` + `RoundTripper` (no port, no listener)

Substitute a custom `http.RoundTripper` so every outbound request
is intercepted in-process. Cleanest for unit tests where you
control all responses.

```go
package mypkg_test

import (
    "bytes"
    "io"
    "net/http"
    "testing"

    "github.com/atome-fin/atome-fin-go-sdk/atomefin"
    "github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// canned200 is a tiny RoundTripper that replies SUCCESS to every
// request — adapt the body / status per test.
type canned200 struct{ body string }

func (rt canned200) RoundTrip(*http.Request) (*http.Response, error) {
    return &http.Response{
        StatusCode: 200,
        Header:     http.Header{"Content-Type": {"application/json"}},
        Body:       io.NopCloser(bytes.NewBufferString(rt.body)),
    }, nil
}

func TestMyAuthFlow(t *testing.T) {
    c, err := atomefin.New(
        atomefin.WithBaseURL("https://atome-fin.test"), // any URL — never dialled
        atomefin.WithPrivateKeyPEM(testPrivKeyPEM),     // your own test key
        atomefin.WithHTTPClient(&http.Client{
            Transport: canned200{body: `{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","authOrderId":"AUTH-1","status":"SUCCESS","totalAmount":1500000,"currency":"IDR"}}`},
        }),
    )
    if err != nil {
        t.Fatal(err)
    }
    resp, err := payment.New(c).Auth(/* ctx, req */)
    // … assert business logic
    _ = resp
}
```

## Pattern B — `httptest.NewServer` (real socket, real headers)

A real local HTTP server. Useful when you need to assert the SDK's
exact wire shape — headers (`Authorization`, `sessionid`,
`Encrypt`), URL paths, query canonicalization, retry counts.
Stable across SDK versions.

```go
package mypkg_test

import (
    "io"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/atome-fin/atome-fin-go-sdk/atomefin"
    "github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func TestAuthHitsCorrectPath(t *testing.T) {
    var gotPath string
    var gotBody []byte
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        gotPath = r.URL.Path
        gotBody, _ = io.ReadAll(r.Body)
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(200)
        _, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
    }))
    defer srv.Close()

    c, err := atomefin.New(
        atomefin.WithBaseURL(srv.URL),               // points at the test server
        atomefin.WithPrivateKeyPEM(testPrivKeyPEM),
    )
    if err != nil {
        t.Fatal(err)
    }
    if _, err := payment.New(c).Auth(/* ctx, req */); err != nil {
        t.Fatal(err)
    }
    if gotPath != "/auth" {
        t.Errorf("path = %q", gotPath)
    }
    _ = gotBody
}
```

---

## Caveats

The two patterns above cover the **happy 80%**. The remaining 20%
needs care:

### 1. Signing still happens

`Client.DoSigned` and `Client.DoSignedGET` always sign their
canonical input — even against a mock server. You need a valid
`WithPrivateKeyPEM` (any well-formed RSA-2048 key — the test
server doesn't have to verify). Generate one with
`crypto/rsa.GenerateKey(rand.Reader, 2048)` and PEM-encode it
once at the top of your test file.

### 2. Hybrid-encryption endpoints

`/credit-information` and `/credit-application` (since v0.3) use
`Client.DoEncryptedSigned`. Those calls require
**`WithEncryptAtomePublicCertPEM`** at construction; without it
the SDK returns `*atomefin.ValidationError` BEFORE any
RoundTripper / httptest call. To exercise the encrypted path:

- Generate an RSA-2048 keypair for the "Atome encrypt" role and
  pass the public half via `WithEncryptAtomePublicCertPEM`.
- On the mock server side, hold the private half and call
  `encrypt.Unmarshal(header, body, partnerPriv)` to inspect the
  inbound plaintext.

See `atomefin/credit/encrypted_e2e_test.go` for a complete
worked example.

### 3. Inbound callbacks are the partner's responsibility

The SDK ships outbound (Client → atome-fin) only. Partner-hosted
callback endpoints (`/<authNotifyUrl>`, `/<refundNotifyUrl>`, …)
sit BEHIND your HTTP framework — you mount the
`callback.AuthHandler(verifier, fn)` etc. into your own router.
To unit-test the callback path: build a signed request body in
the test using `sign.NewRSA2Signer` and POST it to your handler
via `httptest.NewServer` or `httptest.NewRecorder`.

### 4. Request-ID auto-mint and retry counts

Auto-minted request-IDs are stable across SDK retries (DESIGN
§1.4). If your test asserts retry counts, count by RoundTripper
invocations, not by `RequestID` uniqueness.

### 5. Production guard

`mock.NewClient` (since v0.4) **refuses to construct when
`WithEnvironment(EnvProd)` is supplied** — the constructor calls
`t.Fatalf` with a clear message. Tests that route through
`mock.NewClient` cannot accidentally co-exist with a production
configuration; tests still using the v0.3.1 `WithHTTPClient` /
`httptest.NewServer` patterns carry no such guard, so review
your test config to ensure nothing under
`if env == "prod" { ... }` accidentally takes the mock path.

---

## Realistic sandbox (v0.5)

`atomefin/mock` v0.4 covered the unit-test 80%. v0.5 layers the
"behave like the real upstream" 20% on `mock.NewServer` via
four opt-in flags. Every flag is **off by default** so v0.4
tests pass unchanged; flip individually as your scenario
demands.

```go
srv := mock.NewServer(t,
    mock.PerEndpoint(map[string]mock.Scenario{
        "POST /auth":    mock.AuthSuccess("AUTH-1"),
        "POST /capture": mock.CaptureSuccess("AUTH-1"),
    }, mock.AlwaysSuccess()),

    mock.WithSpecValidation(),                      // reject 400 PARAMS_MISSING on bad body / missing headers
    mock.WithIdempotency(),                         // duplicate `requestId` → cached response
    mock.WithAutoCallback(map[string]http.Handler{  // fire callback after sync response
        "POST /<authNotifyUrl>":    authHandler,
        "POST /<captureNotifyUrl>": captureHandler,
    }),
)
```

### Typed scenario builders

Replace hand-rolled JSON with typed builders that carry both
the sync response shape AND the matching callback event:

| Endpoint | Success / Processing / Failed |
|---|---|
| `/auth`                  | `AuthSuccess(orderID)` / `AuthProcessing()` / `AuthFailed(code)` |
| `/capture`               | `CaptureSuccess(orderID)` / `CaptureProcessing()` / `CaptureFailed(code)` |
| `/voidAuth`              | `VoidAuthSuccess()` / — / `VoidAuthFailed(code)` (no callback per spec) |
| `/refund`                | `RefundSuccess(refundID)` / `RefundProcessing()` / `RefundFailed(code)` |
| `/repayment-request`     | `RepaymentSuccess(repayID)` / `RepaymentProcessing()` / `RepaymentFailed(code)` |
| `/credit-application`    | `CreditApplicationSuccess()` / `CreditApplicationProcessing()` / `CreditApplicationFailed()` |
| `/credit-information`    | `CreditInformationSuccess()` / `CreditInformationProcessing()` / `CreditInformationFailed()` |

PROCESSING outcomes don't fire callbacks (callbacks are
terminal-only per spec). When `WithAutoCallback` is also
configured, terminal-state outcomes drive the matching
`*Event` to the partner's handler — the multi-step lifecycle
emerges from composition without any new DSL.

### Multi-step lifecycle (no DSL needed)

The "script" is the union of `PerEndpoint` (sync responses) and
`WithAutoCallback` (async pushes):

```go
mock.NewServer(t,
    mock.PerEndpoint(map[string]mock.Scenario{
        "POST /auth":    mock.AuthSuccess("A-1"),     // sync SUCCESS → fires AuthEvent
        "POST /capture": mock.CaptureSuccess("A-1"),  // sync SUCCESS → fires CaptureEvent
        "POST /refund":  mock.RefundFailed(""),       // sync but business FAILED → fires RefundEvent
    }, mock.AlwaysSuccess()),
    mock.WithAutoCallback(map[string]http.Handler{
        "POST /<authNotifyUrl>":    authHandler,
        "POST /<captureNotifyUrl>": captureHandler,
        "POST /<refundNotifyUrl>":  refundHandler,
    }),
)
```

See [examples/mock_demo/realistic_test.go](../examples/mock_demo/realistic_test.go)
for a full worked example.

### Idempotency

`WithIdempotency` enables an LRU replay cache keyed on
`(method, path, requestId)`. A duplicate request returns the
original response byte-for-byte (with an `X-Mock-Replay: 1`
marker header). Default cache size 1024; tweak via
`WithIdempotencyCacheSize(n)`. `Server.Reset()` clears the
cache between sub-tests.

Encrypted POSTs (`/credit-information`, `/credit-application`)
bypass the cache in v0.5 — the spec server has no decryption
key, so the requestId can't be extracted from the encrypted
body. Plaintext endpoints work as expected.

### Forward-compat: response signing

`WithResponseSigning(privPEM)` signs every response body with
the supplied key and emits the signature in `Authorization`.
The SDK doesn't verify outbound responses today (Q5 partner-
pending), so this is forward-compat plumbing — flip it on to
exercise "what would the v0.6 verifying side look like?"
without touching production code.

---

## Roadmap

Both patterns above are stable. The heavier-weight options below
are coming and will share the same surface; today's
`WithHTTPClient` + `httptest.NewServer` tests will continue to
work unchanged.

| | Available | Description |
|---|---|---|
| Pattern A: `WithHTTPClient` | **today (v0.3.1)** | RoundTripper-based; no listener; cleanest for unit tests. |
| Pattern B: `httptest.NewServer` | **today (v0.3.1)** | Real local socket; asserts exact wire shape. |
| `atomefin/mock` v0.4 | **today (v0.4.0)** | Pre-built scenarios (`AlwaysSuccess`, `AlwaysProcessing`, `AlwaysFailed(code)`, `AlwaysAPIError(...)`, `PerEndpoint(map)`); 7 callback-sender helpers (`FireAuthCallback` etc.); bundled test keypairs (with `WithMockKeysAllowed()` opt-in); EnvProd refusal guard. See [examples/mock_demo/demo_test.go](../examples/mock_demo/demo_test.go). |
| `atomefin/mock` v0.5 | **today (v0.5.0)** | Realistic-sandbox flags on `mock.NewServer`: `WithSpecValidation` (presence-validate against pinned swagger), `WithIdempotency` (LRU replay cache keyed on `requestId`), `WithAutoCallback(map)` (fire matching `*Event` after sync response — multi-step lifecycle composes from this), `WithResponseSigning` (forward-compat). 21 typed scenario builders (`AuthSuccess(orderID)` / `RefundFailed(code)` / etc) replace hand-rolled JSON. All v0.4 surface preserved — every flag is opt-in. See [examples/mock_demo/realistic_test.go](../examples/mock_demo/realistic_test.go). |

**Migration commitment:** v0.4 and v0.5 will leave the v0.3.1
patterns intact. Adopting `atomefin/mock` later is opt-in; tests
written against `WithHTTPClient` or `httptest.NewServer` won't
need rewrites.
