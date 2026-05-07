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
| `atomefin/encrypt` | AES-ECB-PKCS5 + RSA-PKCS#1 v1.5 hybrid envelope used by `/credit-information` and `/credit-application`. Stdlib-only. `Marshal` / `Unmarshal`, `RandomAESKey` (rejection-sampled A — Z), header build/parse. |
| `atomefin/mock` | First-class testing surface — `NewClient(t, ...)` / `NewServer(t, ...)` backed by pre-built `Scenario`s (`AlwaysSuccess` / `AlwaysProcessing` / `AlwaysFailed` / `AlwaysAPIError` / `PerEndpoint`); seven `Fire*Callback` helpers for exercising partner callback handlers. EnvProd-refusal guard + bundled test keypairs (opt-in via `WithMockKeysAllowed`). |
| `atomefin/transport` | `RetryPolicy`, `Logger`, `Observer`, `NewSlogLogger`, User-Agent assembly. |
| `atomefin/payment` | `Service` with `Auth` / `Capture` / `VoidAuth` + `QueryAuth` / `QueryCapture` / `QueryVoidAuth` + `*PollUntilTerminal` + `PaymentPreCheck` / `PaymentPlan` (v0.2 pre-checkout); typed request/response structs. |
| `atomefin/refund` | `Service` with `Refund` / `QueryRefund` / `RefundPollUntilTerminal`; types `RefundParam`, `RefundResult`, `SubOrderRefundRequest`, `SubOrderRefundInfo`. |
| `atomefin/repayment` | `Service` with `Repayment` / `QueryRepayment` / `RepaymentPollUntilTerminal`; types `RepaymentParam`, `RepaymentResult`, `CommerceAccountChanges`. |
| `atomefin/credit` | `Service` for the credit lifecycle plus account-ops: `SubmitInformation` (KYC start), `SubmitApplication`, `QueryResult` / `QueryInformationResult`, `BalanceHistory`, `ModifyApplicationInfo`, `CloseAccount`. |
| `atomefin/bill` | `Service` with `Bills` / `BillDetail` / `BillsUnpaid` + `BillsAll` auto-pagination; types `Bill`, `BillDetail`, `BillOrder`, `BillDiscounts`, `Discount`, `DiscountDetail` + `OverdueStatus` enum. |
| `atomefin/transaction` | `Service` with `Transactions` / `TransactionDetail` + `TransactionsAll` auto-pagination; types `Transaction`, `TransactionDetail` + `TradeType` enum (`AUTH` / `CAPTURE` / `VOID` / `REFUND`). |
| `atomefin/callback` | `Verifier` (multi-cert), `AuthHandler` / `CaptureHandler` / `RefundHandler` / `RepaymentHandler` / `CreditApplicationHandler` / `CreditInformationHandler` / `AccountChangeHandler`, `AckResponse`. |
| `qa/marshal` | Generic round-trip test harness wired against `qa/testdata/` fixtures. |
| `qa/specserver` | Spec-driven test framework — loads the pinned upstream `swagger.yaml`, walks the schema tree, and stands up an `httptest` server that rejects requests that omit any spec-required field. Per-package `*_spec_test.go` files cover all 25 outbound endpoints; `make test-spec-drift` runs the live upstream-drift sentinel. |

The umbrella `atomefin.Client` also exposes `Client.HeartBeat(ctx)` —
a one-call signed liveness probe against `GET /heart-beat`. Returns
`nil` on 2xx, `*atomefin.APIError` on non-2xx, `*atomefin.TransportError`
on transport failures.

### Hard rule — hybrid encryption on the two credit POSTs

`/credit-information` and `/credit-application` are the only v0.3
endpoints that require AES-ECB-PKCS5 + RSA-PKCS#1 v1.5 hybrid
encryption (Q31 — Q34 RESOLVED 2026-05-06). The SDK handles it
transparently inside `credit.SubmitInformation` and
`credit.SubmitApplication` via `Client.DoEncryptedSigned`.

You MUST construct the `Client` with the encrypt cert pair:

```go
c, err := atomefin.New(
    atomefin.WithBaseURL(...),
    atomefin.WithPrivateKeyPEM(signPriv),                  // signing
    atomefin.WithAtomePublicCertPEM(atomeSignPub),         // verification
    atomefin.WithEncryptAtomePublicCertPEM(atomeEncryptPub), // hybrid-encrypt wrap
    atomefin.WithEncryptPrivateKeyPEM(partnerEncryptPriv),   // optional — partner-side decryption tooling
)
```

Calling `SubmitInformation` / `SubmitApplication` without
`WithEncryptAtomePublicCertPEM` returns
`*atomefin.ValidationError` BEFORE any network round-trip — the
gateway's `400 INVALID_ENCRYPTION` is surfaced locally.

Algorithm details, the partner-protocol-mandated ECB block
walker, the rejection-sampled key generator, and the external
test vector all live in [`atomefin/encrypt`](./atomefin/encrypt/doc.go).

### Credit-change vectors

The spec defines two distinct credit-change wire shapes, modelled
as separate Go types:

- **`payment.AccountChanges`** is the canonical credit-change vector
  for `/auth`, `/capture`, `/voidAuth`, `/refund` responses and the
  `<accountChangeNotifyUrl>` callback.
- **`repayment.CommerceAccountChanges`** is the canonical credit-
  change vector for commerce-domain responses (`/repayment-request`,
  `/repayment-result`, `<repaymentNotifyUrl>`).

Each carries the field set defined by the spec for its respective
endpoint family.

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
| `GET` | `/bills` | partner → atome-fin | `bill.New(c).Bills(ctx, *BillsParams)` | `pageNumber`/`pageSize`/optional filters | `*bill.BillsResponse` | paginated bill list; `BillsAll` walks every page |
| `GET` | `/billDetail` | partner → atome-fin | `bill.New(c).BillDetail(ctx, billID)` | `billId` query (yyyyMM) | `*bill.BillDetailResponse` | full single-bill view including `orders` + `discounts` |
| `GET` | `/billUnpaid` | partner → atome-fin | `bill.New(c).BillsUnpaid(ctx, *BillsUnpaidParams)` | `pageNumber`/`pageSize` + optional UID filter | `*bill.BillsResponse` | unpaid filter view |
| `GET` | `/transactions` | partner → atome-fin | `transaction.New(c).Transactions(ctx, *TransactionsParams)` | `pageNumber`/`pageSize` + optional `externalReferenceUid`/`authOrderId`/`tradeType`/date-range filters | `*transaction.TransactionsResponse` | paginated trade ledger; `TransactionsAll` walks every page |
| `GET` | `/transactionDetail` | partner → atome-fin | `transaction.New(c).TransactionDetail(ctx, tradeID)` | `tradeId` query | `*transaction.TransactionDetailResponse` | full single-transaction view (linked `billId`, `failureCode`, free-form `notes`) |
| `POST` | `/payment-precheck` | partner → atome-fin | `payment.New(c).PaymentPreCheck(ctx, req)` | `*payment.PaymentPreCheckRequest` | `*payment.PaymentPreCheckResponse` | eligibility / risk pre-flight before `/auth`; returns `Eligible` + `AvailableCredit` + `DeniedReason` |
| `POST` | `/payment-plan` | partner → atome-fin | `payment.New(c).PaymentPlan(ctx, req)` | `*payment.PaymentPlanRequest` | `*payment.PaymentPlanResponse` | installment-plan options (1/3/6/9/12 tenors) + per-month breakdown; partner surfaces choice to user |
| `POST` | `/repayment-request` | partner → atome-fin | `repayment.New(c).Repayment(ctx, req)` | `*repayment.RepaymentParam` | `*repayment.RepaymentResponse` | apply a repayment against a prior auth + bill; uses `CommerceAccountChanges` (distinct from `payment.AccountChanges`) |
| `GET` | `/repayment-result` | partner → atome-fin | `repayment.New(c).QueryRepayment(ctx, requestID, externalReferenceUID)` | `requestId` + `externalReferenceUid` query | `*repayment.RepaymentResponse` | polling alternative to PROCESSING webhook |
| `POST` | `<repaymentNotifyUrl>` | atome-fin → partner | `callback.RepaymentHandler(v, fn)` | `*callback.RepaymentEvent` (= `repayment.RepaymentResponse`) | `callback.AckResponse` | terminal-only |
| `POST` | `/credit-information` | partner → atome-fin | `credit.New(c).SubmitInformation(ctx, req)` | `*credit.CreditInformationParam` | `*credit.CreditInformationResponse` | KYC start; returns a `requestId` + jumpUrl into the Atome KYC web flow. **Hybrid encryption required** — see `atomefin/encrypt` |
| `POST` | `/credit-application` | partner → atome-fin | `credit.New(c).SubmitApplication(ctx, req)` | `*credit.CreditApplicationParam` | `*credit.CreditApplicationResponse` | submit credit application after KYC completes. **Hybrid encryption required** — see `atomefin/encrypt` |
| `GET` | `/credit-result` | partner → atome-fin | `credit.New(c).QueryResult(ctx, externalReferenceUID)` | `externalReferenceUid` query | `*credit.CreditApplicationResponse` | poll application terminal state; spec preserves the `INPROGESS` literal verbatim |
| `GET` | `/credit-information-result` | partner → atome-fin | `credit.New(c).QueryInformationResult(ctx, externalReferenceUID, requestID)` | `externalReferenceUid` + `requestId` query | `*credit.CreditInformationCollectResponse` | poll KYC-collection terminal state |
| `GET` | `/query-balance-history` | partner → atome-fin | `credit.New(c).BalanceHistory(ctx, *BalanceHistoryParams)` | `start`/`count` + filters | `*credit.BalanceHistoryResponse` | paginated balance ledger (uses spec's `start`/`count` rather than `pageNumber`/`pageSize`) |
| `POST` | `/modify-application-info` | partner → atome-fin | `credit.New(c).ModifyApplicationInfo(ctx, req)` | `*credit.CreditApplicationChangeParam` | `*credit.ModifyApplicationInfoResponse` | account-ops: edit a submitted credit application |
| `POST` | `/close-account` | partner → atome-fin | `credit.New(c).CloseAccount(ctx, req)` | `*credit.CloseAccountParam` | `*credit.CloseAccountResponse` | account-ops: terminate the account |
| `POST` | `<creditApplicationNotifyUrl>` | atome-fin → partner | `callback.CreditApplicationHandler(v, fn)` | `*callback.CreditApplicationEvent` (= `credit.CreditApplicationResponse`) | `callback.AckResponse` | terminal-only credit-application webhook |
| `POST` | `<creditInformationNotifyUrl>` | atome-fin → partner | `callback.CreditInformationHandler(v, fn)` | `*callback.CreditInformationEvent` (= `credit.CreditInformationCollectResponse`) | `callback.AckResponse` | terminal-only KYC-collection webhook |
| `POST` | `<accountChangeNotifyUrl>` | atome-fin → partner | `callback.AccountChangeHandler(v, fn)` | `*callback.AccountChangeEvent` | `callback.AckResponse` | inbound-only — atome-fin pushes credit-limit / account-status mutations; carries `payment.AccountChanges` for the credit-change vector |
| `GET` | `/heart-beat` | partner → atome-fin | `c.HeartBeat(ctx)` | (none) | `error` (nil on 2xx) | one-call liveness probe; signed via `DoSignedGET` with empty canonical |

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

### Testing **your own code** against the SDK

Two patterns let you exercise the SDK end-to-end without dialling
atome-fin: substitute the underlying `http.RoundTripper` via
`atomefin.WithHTTPClient`, or point `WithBaseURL` at a local
`httptest.NewServer`. Worked examples + caveats (signing,
hybrid-encryption endpoints, callback handlers, EnvProd guard
roadmap) live in [docs/MOCK_MODE.md](docs/MOCK_MODE.md). The
snippets in that doc are mirrored into
`docs/mock_mode_examples_test.go` and run on every CI pass —
copy-paste-confidence guaranteed.

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
