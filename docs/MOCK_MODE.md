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

A future `atomefin/mock` package will **refuse to construct a
mock Client when `Environment == EnvProd`**. Until then, the
two patterns above carry no guard — review your test config so
nothing under `if env == "prod" { ... }` accidentally takes the
mock path.

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
| `atomefin/mock` v0.4 | planned | Pre-built scenarios (`AlwaysSuccess`, `AlwaysProcessing`, `AlwaysFailed(code)`, `PerEndpoint(map)`); 7 callback-sender helpers (`FireAuthCallback` etc.); bundled test keypairs (with `WithMockKeysAllowed()` opt-in); EnvProd refusal guard. |
| `atomefin/mock` v0.5 | planned | Spec-server promotion (`atomefin/mock/internal/spec/`); response-signing; idempotency cache; fluent scenario DSL; auto-callback firing. Drop-in replacement for the v0.3.1 patterns. |

**Migration commitment:** v0.4 and v0.5 will leave the v0.3.1
patterns intact. Adopting `atomefin/mock` later is opt-in; tests
written against `WithHTTPClient` or `httptest.NewServer` won't
need rewrites.
