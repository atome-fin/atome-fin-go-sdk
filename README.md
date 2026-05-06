# atome-fin-go-sdk

Server-side Go SDK for the atome-fin payment API. Provides the
partner-side outbound calls (`/auth`, `/capture`, `/voidAuth`),
inbound terminal-state webhook handlers, RSA-2048 request signing,
multi-cert verifier rotation, and a typed payment surface.

The SDK ships pre-1.0 (`v0.x`); the upstream API is still evolving
and the public surface may tighten in subsequent minor versions
(see [Known assumptions](#known-assumptions)).

```sh
go get github.com/atome-fin/atome-fin-go-sdk@latest
```

## Quick start

```go
package main

import (
    "context"
    "log"
    "os"

    "github.com/atome-fin/atome-fin-go-sdk/atomefin"
    "github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func main() {
    priv, _ := os.ReadFile("/etc/atome/partner.pem")

    c, err := atomefin.New(
        atomefin.WithPrivateKeyPEM(priv),
        atomefin.WithEnvironment(atomefin.EnvTest),
        // atomefin.WithPartnerID("partner-foo"), // optional log-enrichment label
    )
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()

    res, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
        ExternalReferenceUID: "user-42",
        TotalAmount:          1500000, // minor units
        PeriodType:           3,
        SubOrders: []payment.SubOrder{
            {SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
        },
        Sessionid: "session-token-from-checkout", // travels in the HTTP `sessionid` header
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("auth status=%s authOrderId=%s", res.Data.Status, res.Data.AuthOrderID)
}
```

Two runnable examples ship in [`examples/`](examples/):

- [`examples/auth_capture/`](examples/auth_capture/) — outbound happy
  path: `Client` → `/auth` → `/capture`.
- [`examples/webhook_server/`](examples/webhook_server/) — inbound
  callback handlers wired into a stdlib `http.ServeMux`, with
  multi-cert rotation support.

## Package map

| Package | Purpose |
|---|---|
| `atomefin` | `Client`, functional `Option`s, error types, retry policy plumbing, `MarshalSigning`. |
| `atomefin/sign` | RSA-2048 PKCS#1-v1.5 / PSS signer + verifier; PEM loaders; canonical-input helpers. |
| `atomefin/transport` | `RetryPolicy`, `Logger`, `Observer`, `NewSlogLogger`, User-Agent assembly. |
| `atomefin/payment` | `Service` with `Auth` / `Capture` / `VoidAuth` / `QueryAuth` / `QueryCapture` / `QueryVoidAuth` + `*PollUntilTerminal` helpers, all typed request/response structs. |
| `atomefin/refund` | `Service` with `Refund` / `QueryRefund` / `RefundPollUntilTerminal`; types `RefundParam`, `RefundResult`, `SubOrderRefundRequest`, `SubOrderRefundInfo`. |
| `atomefin/callback` | `Verifier` (multi-cert), `AuthHandler` / `CaptureHandler` / `RefundHandler`, `AckResponse`. |
| `qa/marshal` | Generic round-trip test harness wired against `qa/testdata/` fixtures. |

## Implemented endpoints

Implementation pinned to the upstream spec snapshot dated **2026-05-06**.
`DESIGN.md` is the canonical per-endpoint reference for fields,
optionality, and constraints; this table is the surface inventory you
diff against when the spec moves.

| Method | Path | Direction | SDK entry point | Request | Response | Notes |
|---|---|---|---|---|---|---|
| `POST` | `/auth` | partner → atome-fin | `payment.New(c).Auth(ctx, req)` | `*payment.AuthRequest` | `*payment.AuthResponse` | `sessionid` header required (≤64 chars), travels via `payment.AuthRequest.Sessionid` (json:"-") |
| `POST` | `/capture` | partner → atome-fin | `payment.New(c).Capture(ctx, req)` | `*payment.CaptureRequest` | `*payment.CaptureResponse` | `subOrders`, `totalAmount`, `periodType` MUST mirror the prior `/auth` |
| `POST` | `/voidAuth` | partner → atome-fin | `payment.New(c).VoidAuth(ctx, req)` | `*payment.VoidAuthRequest` | `*payment.VoidAuthResponse` | three-field body: `requestId`, `externalReferenceUid`, `authOrderId` |
| `POST` | `<authNotifyUrl>` | atome-fin → partner | `callback.AuthHandler(v, fn)` | `*callback.AuthEvent` (= `payment.AuthResponse`) | `callback.AckResponse` | terminal-only (no `PROCESSING`); at-least-once — handler must be idempotent |
| `POST` | `<captureNotifyUrl>` | atome-fin → partner | `callback.CaptureHandler(v, fn)` | `*callback.CaptureEvent` (= `payment.CaptureResponse`) | `callback.AckResponse` | terminal-only; same idempotency contract as auth-callback |
| `GET` | `/query-auth` | partner → atome-fin | `payment.New(c).QueryAuth(ctx, requestID)` | `requestId` query | `*payment.AuthResponse` | polling alternative to PROCESSING webhooks; sorted-canonical query per spec |
| `GET` | `/query-capture` | partner → atome-fin | `payment.New(c).QueryCapture(ctx, requestID)` | `requestId` query | `*payment.CaptureResponse` | polling alternative to PROCESSING webhooks; sorted-canonical query per spec |
| `GET` | `/query-voidAuth` | partner → atome-fin | `payment.New(c).QueryVoidAuth(ctx, requestID)` | `requestId` query | `*payment.VoidAuthResponse` | polling alternative to PROCESSING webhooks; sorted-canonical query per spec |
| `POST` | `/refund` | partner → atome-fin | `refund.New(c).Refund(ctx, req)` | `*refund.RefundParam` | `*refund.RefundResponse` | refund a prior `/auth` (full or per-sub-order); validator enforces `refundAmount == Σ subOrderRefunds[].refundAmount` (Q25 conservative) |
| `GET` | `/query-refund` | partner → atome-fin | `refund.New(c).QueryRefund(ctx, requestID)` | `requestId` query | `*refund.RefundResponse` | polling alternative to PROCESSING webhooks for refunds |
| `POST` | `<refundNotifyUrl>` | atome-fin → partner | `callback.RefundHandler(v, fn)` | `*callback.RefundEvent` (= `refund.RefundResponse`) | `callback.AckResponse` | terminal-only; same idempotency contract as auth/capture-callback |

For the async `PROCESSING` path on outbound calls (server returns the
typed envelope without a terminal `data.status`), use
`payment.AuthPollUntilTerminal` / `CapturePollUntilTerminal` — both
re-submit the same `RequestID` until terminal or the configured
`PollOptions.MaxWait` / parent `ctx` deadline expires.

## Hard rules baked into the public surface

The following constraints are **not** negotiable; partners writing
adapters or wrappers around this SDK should plan around them.

### Money is `int64` minor units everywhere on the wire

Every monetary field — `totalAmount`, `subOrders[].amount`,
`originalAmount`, every `*Change` delta on `AccountChanges` — is plain
`int64` in the smallest currency unit (rupiah, centavo, etc.). No
pointers, no `json.Number`, no string. Negatives are required for
credit-change deltas (a refund event reduces `usedCreditChange`).
`AccountChanges.*Change` rows do **not** use `,omitempty` so a
legitimate zero delta still serialises.

The qa/marshal harness enforces this with the R10/R11/R12 invariants
(full int64 round-trip; fractional decode rejected loudly; encoder
never emits `.` or scientific notation on any amount key).

### `userCreditScore` is the only float on the public surface

`payment.RequestExtendInfo.UserCreditScore` is a `*float64` in the
range `[0, 1]`. It is the **sole** carve-out from the int64 rule
because it is a probability, not money. No other `float32` / `float64`
is allowed anywhere on a request, response, or callback shape.

### `sessionid` is an HTTP header, never the JSON body

The `/auth` endpoint requires a `sessionid` header. The SDK surfaces
it on `payment.AuthRequest.Sessionid` with a `json:"-"` tag and routes
it through `atomefin.WithRequestHeader("sessionid", ...)` so the
value never appears in the signed JSON body. Test
`TestService_Auth_Success` asserts both halves of this contract.

### Multi-cert verifier slot for callback rotation

`callback.Verifier` accepts a slice of `sign.Verifier`. Verification
succeeds when ANY of them verifies the signature. This is the design
hook for cert rotation: during a cutover window, configure both the
outgoing-soon and the new public keys, and inbound callbacks signed
by either keep working.

```go
v, _ := callback.FromCertPEMs([][]byte{oldAtomeCertPEM, newAtomeCertPEM})
mux.Handle("/atome/auth", callback.AuthHandler(v, onAuth))
```

### Defaults are overridable

The spec marks all three base URLs as placeholders (Q1) and the
Authorization header format as not-quite-final (Q2). The SDK ships
sensible defaults but exposes one-line knobs to override:

| Default | Override |
|---|---|
| `EnvTest`/`EnvPre`/`EnvProd` placeholder URLs | `atomefin.WithBaseURL("https://your-gateway.example.com")` |
| `Authorization: <raw base64 sig>` | `atomefin.WithAuthorizationScheme(atomefin.SchemeAtomeKeyed)` |
| `atomefin.SchemeRawBase64` (default) | any custom `func(sig, keyID string) string` |
| Default no-op logger | `atomefin.WithLogger(transport.NewSlogLogger(slog.Default()))` |
| Default `*http.Client` (cloned `DefaultTransport`, 30s timeout, tuned pool) | `atomefin.WithHTTPClient(yourCustom)` |
| Default 3-attempt retry | `atomefin.WithRetry(transport.RetryPolicy{...})` |
| Default 1 MiB response cap | `atomefin.WithMaxResponseBytes(n)` |

## Behavioural contract

### Outbound (`Client.DoSigned` → `payment.Service`)

1. Body is marshalled via `atomefin.MarshalSigning` (HTML-escape OFF)
   so the bytes signed are byte-for-byte the bytes transmitted. Any
   payload field with `&` / `<` / `>` (e.g. a shipping address with an
   ampersand) signs correctly.
2. Signature is RSA-2048 PKCS#1-v1.5 over SHA-256 by default; PSS via
   `sign.WithSaltedPSS`. The signature is base64-standard-encoded and
   placed verbatim in the `Authorization` header.
3. `RequestID` is auto-minted (32-char ULID-like hex) when empty and
   reused across retries (DESIGN.md §1.4).
4. Retries fire on transport-level failures and HTTP 500/502/503/504
   per the configured `RetryPolicy`. 4xx (including
   `INVALID_SIGNATURE`) is never retried.
5. Non-2xx responses are decoded into `*atomefin.APIError`; transport
   failures into `*atomefin.TransportError`. Both implement
   `Temporary() bool` and `Unwrap()`.

### Inbound (`callback.AuthHandler` / `CaptureHandler`)

1. Body is read via `io.LimitReader` (1 MiB cap by default; symmetric
   to the outbound response cap). Oversize → HTTP 400.
2. `Authorization` header is verified against the **raw body bytes**
   (not the parsed JSON). Bad / missing signature → HTTP 401 with
   `AckResponse{Code: "INVALID_SIGNATURE"}`.
3. Body is decoded only after verification, into the typed event.
4. The user-supplied handler runs:
   - `nil`  → HTTP 200 + `AckResponse{Code: "SUCCESS"}`
   - error  → HTTP 500 + `AckResponse{Code: "SERVER_ERROR"}` (Atome retries)
5. Every response sets `Content-Type: application/json; charset=utf-8`
   and `X-Content-Type-Options: nosniff`.

> Atome callbacks are **at-least-once**. The SDK invokes the user
> handler exactly once per HTTP call; deduping on
> `event.Data.RequestID` at the application layer is the partner's
> responsibility.

## Observability

The `transport.Logger` interface matches `log/slog`'s shape; the
provided `transport.NewSlogLogger(*slog.Logger)` adapter redacts
known-sensitive keys (`Authorization`, `sessionid`,
`shipping_name`, `externalReferenceUid`, etc.) before forwarding to
slog so partners can pass any handler without bypassing the §10
redaction policy.

`transport.Observer` is the metrics / tracing hook surface
(`OnRequest`/`OnResponse`/`OnRetry`). The SDK contains panic-recovery
wrappers around every Observer call site so a misbehaving partner
metric implementation can never corrupt request handling.

## Testing

```sh
make test        # quick suite
make test-race   # race detector + coverage
make vet
make fmtcheck
make cover
```

The `qa/marshal` harness validates every public request/response
struct against the fixtures in `qa/testdata/`. Twelve invariants
(documented in `qa/marshal/roundtrip.go`) cover strict-decode,
byte-stable round-trip, omitempty / required-emit, full-int64
amount round-trip, fractional-amount rejection, and float-emission
prohibition. Adding a new spec field means adding the struct field,
dropping a fixture, and the round-trip / strict-decode checks light
up automatically.

## Development

`make ci` runs the same gates GitHub Actions runs — fmtcheck, `go vet`,
build (libraries + examples), `go test -race -cover`,
[`golangci-lint`](https://golangci-lint.run/), and
[`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck).
Both linters are required (the target fails fast with an install hint
if either is missing); the versions are pinned in the `Makefile` to
match the CI workflow so a clean local `make ci` is the same verdict
as a fresh CI run.

Install the pinned tooling once:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
go install golang.org/x/vuln/cmd/govulncheck@latest
# Make sure $(go env GOPATH)/bin is on your $PATH so `make ci` finds them.
```

Then before pushing:

```sh
make ci
```

If `make ci` is green, CI will be green.

Optional: opt into the bundled `pre-push` hook so `make ci` runs
automatically on every `git push`:

```sh
git config core.hooksPath .githooks
```

## Versioning

Pre-1.0 (`v0.x`): minor versions may break — the upstream API is
itself draft. `v0.1.0` is the first complete `auth/capture/voidAuth`
+ callback cut. `v1.0.0` ships once the spec drops "Draft" and the
open questions in `DESIGN.md` §13 close.

## Known assumptions

The following partner / Atome-ops decisions are baked in as defaults
the SDK can change without an API break:

| Q | Assumption | Switch when partner confirms |
|---|---|---|
| Q1 | `EnvTest`/`EnvPre`/`EnvProd` use the spec's placeholder URLs | Pin via `atomefin.WithBaseURL`; update consts in a minor release |
| Q2 | `Authorization` is the raw base64 signature | `atomefin.WithAuthorizationScheme(...)` |
| Q3 | No `keyId` / `keyVersion` header — cert rotation is out-of-band | Multi-cert verifier slot (`callback.NewVerifier([]sign.Verifier{...})`) |
| Q4 | Default scheme is PKCS#1 v1.5; PSS available via `sign.WithSaltedPSS` | Flip per partner spec |
| Q5 | No timestamp / nonce header emitted | Add to `populateHeaders` once the partner specifies the wire name |
| Q6 | `sessionid` lifecycle unspecified — caller-managed | No SDK change needed |
| ~~Q7~~ | **RESOLVED 2026-05-05** — partner identity is the dedicated API URL + RSA cert exchange; **no partner / merchant header is emitted on the wire**. `WithPartnerID` / `WithMerchantID` stay supported as log-enrichment hooks. | n/a |
| Q9 | No 429 / Retry-After handling yet | Extend `RetryPolicy.RetryOnStatus` |
| Q11 | `billDate` / `dueDate` are passed through as strings | No silent `time.Parse` until TZ confirmed |
| Q22 | `originalAmount` is `int64` minor units despite `type: number` in the spec | Loud failure on fractional payloads via R11 |

The full open-question list lives in
[`DESIGN.md` §13](DESIGN.md#13-open-questions-for-the-partner--atome).

## License

MIT — see [LICENSE](LICENSE).
