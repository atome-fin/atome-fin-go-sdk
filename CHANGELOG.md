# Changelog

All notable changes to atome-fin-go-sdk will be documented in this
file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/);
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
post-1.0. Pre-1.0 minor versions may break.

## [0.3.0] — 2026-05-07

Adds AES-ECB-PKCS5 + RSA-PKCS#1 v1.5 hybrid encryption for
`/credit-information` and `/credit-application` POSTs (Q31 — Q34
RESOLVED 2026-05-06). Reverts the v0.2.2 stub `*ValidationError`
on those two methods — they now actually work, given the new
encrypt cert option. Every other v0.2 endpoint is unchanged.

### Added

- **`atomefin/encrypt/`** — new sub-package, stdlib-only:
  - `EncryptBody` / `DecryptBody` — AES-ECB-PKCS5 against a
    32-byte key. ECB block walker is partner-protocol-mandated;
    the SDK does not choose it. `crypto/cipher` deliberately
    omits ECB, so `aes.go` walks the block cipher manually via
    `aes.NewCipher`'s `Encrypt` / `Decrypt`.
  - `WrapAESKey` / `UnwrapAESKey` — RSA-PKCS#1 v1.5 (NOT OAEP).
    `MinKeyBits = 2048` enforced.
  - `BuildEncryptHeader` / `ParseEncryptHeader` —
    `Encrypt: symmetricKey=<urlEncoded(base64(...))>`. Parser
    returns `map[string]string` for forward-compat (e.g. an
    `iv=` field if the spec ever moves to CBC).
  - `RandomAESKey` — 32-char A — Z key, **rejection-sampled**
    (cutoff = 234) — fixes the modulo bias the partner sample at
    `~/Downloads/main.go` line 367-370 carries. Statistical
    uniformity test in `key_test.go` over 100k keys / 3.2M chars
    asserts each letter falls within ±1% of 1/26.
  - `Marshal` / `Unmarshal` — high-level envelope; what
    `Client.DoEncryptedSigned` reaches for.
  - External vector test (`external_vector_test.go`,
    hermetic): pre-computed AES ciphertext + key + plaintext +
    encrypt key pair committed under `testdata/`. Pins the AES
    output byte-for-byte; round-trip the RSA wrap (non-
    deterministic). NO `os/exec`.

- **`Client.DoEncryptedSigned(ctx, method, path, plainBody, opts...)`** —
  sibling of `DoSigned`. Hybrid-encrypts the body via
  `encrypt.Marshal`, injects the `Encrypt:` header through
  `WithRequestHeader` (works because `Encrypt` is NOT in the
  reserved-header allowlist), signs the encrypted body bytes,
  and dispatches via the shared `signAndDispatch` retry loop.
  Per-retry the SDK sends the same encrypted body + same
  signature (idempotency keyed on the partner-supplied
  `requestId` inside the plaintext).

- **Two new options** (`atomefin/options.go`):
  - `WithEncryptAtomePublicCertPEM(pem []byte)` — Atome's
    encrypt public key, used to wrap per-request AES keys.
    Required for `/credit-information` and `/credit-application`.
  - `WithEncryptPrivateKeyPEM(pem []byte, password ...[]byte)` —
    partner's encrypt private key, used to unwrap inbound
    encrypted bodies. Q31 RESOLVED 2026-05-06: credit callbacks
    are plaintext today, so v0.3 has no inbound caller — shipped
    for symmetry + forward-compat.
  - Both reuse `sign.LoadPublicCertPEM` / `sign.LoadPrivateKeyPEM`
    (PKCS#1 / PKCS#8 / PKIX / X.509 CERTIFICATE blocks). 2048-bit
    floor enforced.

- **`Client.EncryptAtomePublicKey()` / `Client.EncryptPrivateKey()`** —
  accessors for partners that need to call `encrypt.Marshal` /
  `encrypt.Unmarshal` directly (e.g. custom callback decryption
  tooling).

- **`qa/specserver` v0.3 extension** — walker now collects
  required header parameters from the spec; dispatcher checks
  header presence on every request. When an `Encrypt` header is
  required AND present, body validation is bypassed because the
  spec server has no decryption key. The Authorization header is
  filtered out (signature validation is the sign package's job,
  not the spec server's). `qa/specserver/runner.go`'s `MustClient`
  now ships an encrypt keypair so credit-flow cases run cleanly.

### Changed

- **`credit.SubmitInformation` / `credit.SubmitApplication`** —
  reverts the v0.2.2 `*ValidationError` stubs. Both methods now
  marshal the request, route through `Client.DoEncryptedSigned`,
  and decode the plaintext response normally. The validators
  (`validateCreditInformation`, `validateCreditApplication`) are
  re-wired into the call path — `validation_internal_test.go`
  continues to exercise them white-box.
- **Test rewires** — `TestSubmitInformation_BlockedUntilV0_3` /
  `TestSubmitApplication_BlockedUntilV0_3` removed; replaced by
  `TestSubmitInformation_RejectsMissingEncryptOption` /
  `TestSubmitApplication_RejectsMissingEncryptOption` that pin
  the new precondition guard (no encrypt key → typed
  `*ValidationError`, no network).
- **`qa/specserver/coverage_test.go`** — `outboundCovered`
  re-adds `POST /credit-information` and `POST /credit-application`.
  `TestSpec_AllOutboundEndpointsCovered` no longer skip-warns
  them.
- **New end-to-end test** —
  `atomefin/credit/encrypted_e2e_test.go` drives a decrypting
  httptest server end-to-end through `SubmitInformation` and
  `SubmitApplication`: the server unwraps the AES key, decrypts
  the body, decodes the plaintext into the spec-defined struct,
  and asserts the SDK sent the right shape.

### Migration

```go
// before (v0.2.2 / v0.2.3 — guaranteed *ValidationError)
c, _ := atomefin.New(
    atomefin.WithBaseURL(...),
    atomefin.WithPrivateKeyPEM(signPriv),
)
_, err := credit.New(c).SubmitInformation(ctx, req)
// err = *ValidationError "requires AES+RSA hybrid encryption — lands in v0.3"

// after (v0.3.0 — works on the wire)
c, _ := atomefin.New(
    atomefin.WithBaseURL(...),
    atomefin.WithPrivateKeyPEM(signPriv),
    atomefin.WithAtomePublicCertPEM(atomeSignPub),
    atomefin.WithEncryptAtomePublicCertPEM(atomeEncryptPub), // NEW
)
resp, err := credit.New(c).SubmitInformation(ctx, req)
// err = nil; resp.Data.JumpURL is the KYC web flow.
```

Partners that had the v0.2.2 / v0.2.3 stubs flowing through their
error-handling tree must now treat `*ValidationError` from these
methods as a configuration bug (missing
`WithEncryptAtomePublicCertPEM`) rather than a "this method is
blocked" signal.

## [0.2.3] — 2026-05-06

Patch release closing the v0.1 → v0.2 spec-drift gaps surfaced by
v0.2.2's `qa/specserver/` framework: 5 endpoints undergo
breaking-but-pre-1.0 field renames or signature additions to match
the 2026-05-06 spec snapshot. Plus one signing-canonical fix for a
latent multi-value foot-gun.

v0.2.0 — v0.2.2 partners: every change below is public-API
breaking. Migration code samples follow each entry.

### Fixed

- **`/refund` field renames.** `RefundParam.AuthOrderID` →
  `RefundParam.CaptureRequestID` (semantics also shift: was the
  authOrderId returned by /auth, now the requestId of the prior
  /capture call). `RefundParam.SubOrderRefunds` →
  `RefundParam.SubOrders`. Per-line
  `SubOrderRefundRequest.RefundAmount` → `SubOrderRefundRequest.Amount`.
  Wire JSON keys move from `authOrderId` / `subOrderRefunds` /
  `subOrderRefunds[].refundAmount` to `captureRequestId` /
  `subOrders` / `subOrders[].amount`. Validators update in lockstep;
  Q25 sum-rule preserved.

  Migration:
  ```go
  // before (v0.2.0 — broken on the wire)
  refund.New(c).Refund(ctx, &refund.RefundParam{
      RequestID:            "r-1",
      ExternalReferenceUID: "u-1",
      AuthOrderID:          authOrderID,
      RefundAmount:         500000,
      SubOrderRefunds: []refund.SubOrderRefundRequest{
          {SubOrderID: "so-1", RefundAmount: 500000},
      },
  })

  // after (v0.2.3)
  refund.New(c).Refund(ctx, &refund.RefundParam{
      RequestID:            "r-1",
      ExternalReferenceUID: "u-1",
      CaptureRequestID:     captureRequestID, // was authOrderID
      RefundAmount:         500000,
      SubOrders: []refund.SubOrderRefundRequest{
          {SubOrderID: "so-1", Amount: 500000}, // was RefundAmount
      },
  })
  ```

- **`/bills` field renames.** `BillsParams.StartDate` /
  `BillsParams.EndDate` → `BillsParams.StartMonth` /
  `BillsParams.EndMonth`. Wire JSON / query keys: `startDate` /
  `endDate` → `startMonth` / `endMonth`. Value format also shifts
  from `yyyy-MM-dd` to `yyyyMM` (per spec).

  Migration:
  ```go
  // before
  bill.New(c).Bills(ctx, &bill.BillsParams{
      ExternalReferenceUID: "u-1",
      StartDate:            "2026-04-01",
      EndDate:              "2026-05-31",
  })

  // after (v0.2.3)
  bill.New(c).Bills(ctx, &bill.BillsParams{
      ExternalReferenceUID: "u-1",
      StartMonth:           "202604",
      EndMonth:             "202605",
  })
  ```

- **`/billDetail` signature change — adds required
  `externalReferenceUid`.** `Service.BillDetail(ctx, billID)` →
  `Service.BillDetail(ctx, billID, externalReferenceUID)`. Both
  query params are spec-required.

  Migration:
  ```go
  // before
  bill.New(c).BillDetail(ctx, "202605")

  // after (v0.2.3)
  bill.New(c).BillDetail(ctx, "202605", externalReferenceUID)
  ```

- **`/transactions` field rename + value-format shift.**
  `TransactionsParams.TradeType` → `TransactionsParams.TransactionType`
  with the underlying enum type `TradeType` renamed to
  `TransactionType` and the constant set replaced (was AUTH /
  CAPTURE / VOID / REFUND; now PAYMENT / REFUND / REPAYMENT per
  spec). Wire query keys: `tradeType` → `transactionType`. The
  spec also declares `startDate` / `endDate` in `yyyyMMdd` format
  (no dashes). Response shape's `Transaction.TradeType` field
  also renamed to `Transaction.TransactionType`.

  Migration:
  ```go
  // before
  tx.New(c).Transactions(ctx, &transaction.TransactionsParams{
      ExternalReferenceUID: "u-1",
      TradeType:            transaction.TradeTypeAuth,
      StartDate:            "2026-04-01",
      EndDate:              "2026-05-01",
  })

  // after (v0.2.3)
  tx.New(c).Transactions(ctx, &transaction.TransactionsParams{
      ExternalReferenceUID: "u-1",
      TransactionType:      transaction.TransactionTypePayment,
      StartDate:            "20260401",
      EndDate:              "20260501",
  })
  ```

- **`/transactionDetail` signature change — replaces `tradeID`
  with required `requestID` + `externalReferenceUID` +
  `transactionType`.** `Service.TransactionDetail(ctx, tradeID)` →
  `Service.TransactionDetail(ctx, requestID, externalReferenceUID, transactionType)`.
  The 2026-05-06 spec eliminates `tradeId` entirely; the lookup is
  now keyed by the original payment / refund / repayment requestId
  plus its discriminator.

  Migration:
  ```go
  // before
  tx.New(c).TransactionDetail(ctx, tradeID)

  // after (v0.2.3)
  tx.New(c).TransactionDetail(ctx, originatingRequestID, externalReferenceUID,
      transaction.TransactionTypePayment) // or Refund / Repayment
  ```

- **`sign.CanonicalQuery` returns `(string, error)`; multi-value
  queries hard-fail with `*sign.MultiValueQueryError`.** The
  upstream gateway only retains the first value for repeated keys;
  emitting all values silently produced an asymmetric canonical
  that failed verification with a generic `INVALID_SIGNATURE`. The
  v0.2.3 contract makes the rule explicit: callers must
  pre-flatten multi-value queries before signing. `Client.DoSignedGET`
  surfaces the failure as `*atomefin.ValidationError`. Architect's
  recommendation in
  `docs/internal/SIGN_VERIFY_ENCRYPT_REVIEW.md`; only in-repo
  caller of `sign.CanonicalQuery` is `Client.DoSignedGET`, updated
  atomically.

  Migration (only affects partners that called `sign.CanonicalQuery`
  directly):
  ```go
  // before
  canonical := sign.CanonicalQuery(values)

  // after (v0.2.3)
  canonical, err := sign.CanonicalQuery(values)
  if err != nil {
      // multi-value input — pre-flatten before signing
  }
  ```

### Changed — `qa/specserver/` `SkipRequired:` cleanup

Each closed gap drops the matching `Case.SkipRequired` entry from
the per-package `*_spec_test.go`:

- `atomefin/refund/spec_test.go` — removed 4 entries
  (`captureRequestId`, `subOrders`, `subOrders[].subOrderId`,
  `subOrders[].amount`).
- `atomefin/bill/spec_test.go` — removed 3 entries (`startMonth`,
  `endMonth`, `externalReferenceUid` for /billDetail).
- `atomefin/transaction/spec_test.go` — removed 4 entries
  (`transactionType` × 2, `requestId`, `externalReferenceUid`).

The remaining `SkipRequired:` entries are partner-pending fields
on `/payment-precheck` (5) and `/payment-plan` (11). Tracked
toward a future v0.2.x or v0.3 closure once the partner agreement
solidifies the commerce-domain SubOrder shape.

## [0.2.2] — 2026-05-06

Patch release adding the spec-driven test framework (`qa/specserver/`)
that v0.2.0 should have shipped with: every outbound SDK call is now
re-validated against the pinned upstream `swagger.yaml`'s
required-field set on every CI run, so the next §2.1-class regression
fails locally rather than at production first-call. Also pre-emptively
blocks the two credit-flow methods that today require a hybrid-
encryption envelope the SDK does not yet implement.

### Added

- **`qa/specserver/`** — spec-assertion test framework per
  `docs/internal/SPEC_ASSERTION_TEST_DESIGN.md`. Loads a pinned
  `swagger-{date}-{sha-prefix}.yaml`, walks the schema tree to
  extract per-endpoint required-body / required-query sets
  (resolves `$ref` and recurses into array `items` schemas), and
  stands up a strict `httptest.NewServer` wrapper that emits
  `400 PARAMS_MISSING` when the SDK omits a spec-required field.
  `specserver.RunCases(t, []Case{…})` is the per-package entry
  point; `Case.SkipRequired` provides an inline allowlist for
  partner-pending fields the SDK is knowingly not yet emitting.
  Single test-only dep: `gopkg.in/yaml.v3`.
- **Per-package `*_spec_test.go` coverage of all 25 outbound SDK
  endpoints** — exercises every `payment.*`, `refund.*`,
  `repayment.*`, `bill.*`, `transaction.*`, `credit.*` (account-ops
  and the GETs), and `Client.HeartBeat` method against the
  spec-server on each test pass. The seven inbound callback paths
  are out of scope per the framework's outbound-only design.
- **`TestSpec_AllOutboundEndpointsCovered`** — cross-checks
  `outboundCovered` against the pinned spec's path inventory.
  Stale entries (covered claims an endpoint the spec doesn't
  declare) hard-fail; uncovered outbound spec endpoints skip-warn.
- **Drift-detection sentinel** — `TestSpec_PinnedMatchesUpstream`
  in `qa/specserver/drift_test.go` (build-tagged
  `//go:build specnetwork`). Fetches `https://doc.apaylater.net/white-label/G/swagger.yaml`,
  SHA-256s it, and fails if the upstream digest no longer matches
  the SHA prefix in the pinned filename. Allowlist via
  `qa/specserver/spec_drift_allowlist.txt` for in-flight transitions.
  Wired as `make test-spec-drift`; default `make ci` stays
  hermetic (offline).
- **`make test-spec-drift`** — Makefile target invoking the
  network-fetching drift sentinel.

### Changed

- **`credit.SubmitInformation` and `credit.SubmitApplication` are
  blocked in v0.2.x.** Both methods now return a typed
  `*atomefin.ValidationError` ("requires AES+RSA hybrid encryption
  — lands in v0.3") *before* any network attempt. The 2026-05-06
  spec mandates an `Encrypt` header plus an AES-ECB-PKCS5 body
  sealed with an RSA-encrypted session key on these two endpoints;
  v0.2.x does not yet implement that envelope, so unblocked calls
  would have produced a guaranteed `400 INVALID_ENCRYPTION` from
  upstream. Surfacing the failure locally with a clear migration
  message means partners that had v0.2.0 / v0.2.1 wired against
  the network path see the issue in their dev loop, not in
  production. Validators (`validateCreditInformation`,
  `validateCreditApplication`), request structs, and round-trip
  fixtures are preserved verbatim — v0.3 re-enables the path with
  one edit per method. White-box internal tests
  (`validation_internal_test.go`, package `credit`) continue to
  exercise the validators directly.
- **`atomefin/credit/service_test.go`** — drops the four tests
  that proved a network call against the now-blocked methods
  (`TestService_SubmitInformation_Success` /
  `_AutoMintsRequestID` / `_4xxBecomesAPIError`,
  `TestService_SubmitApplication_Success`); replaced by
  `TestSubmitInformation_BlockedUntilV0_3` and
  `TestSubmitApplication_BlockedUntilV0_3` that pin the typed
  `*ValidationError` shape.
  The retry, ctx-cancellation, and reserved-header semantics
  tests now probe via `ModifyApplicationInfo` (same `invokePost`
  pipeline), since the blocked methods can no longer be used as
  wire-touching probes.

### Known gaps surfaced by `qa/specserver/` (queued for v0.2.3)

The new framework picked up a number of v0.1 → v0.2 spec drifts
that v0.2.0 didn't catch. Each is acknowledged inline at the
`*_spec_test.go` call site via `Case.SkipRequired` so CI stays
green; closing each is a v0.2.3 patch:

- **`/refund`** — SDK sends `authOrderId` / `subOrderRefunds` /
  `subOrderRefunds[].refundAmount`; spec wants `captureRequestId`
  / `subOrders` / `subOrders[].amount`.
- **`/bills`** — SDK sends `startDate` / `endDate`; spec wants
  `startMonth` / `endMonth`.
- **`/billDetail`** — SDK signature takes only `billID`; spec
  also requires `externalReferenceUid`.
- **`/transactions`** — SDK sends `tradeType`; spec wants
  `transactionType`.
- **`/transactionDetail`** — SDK signature takes only `tradeID`;
  spec requires `requestId` + `externalReferenceUid` +
  `transactionType`.
- **`/payment-precheck`** — SDK `PaymentPreCheckSubOrder` lacks
  `categoryId` / `categoryOneName` / `merchantId` / `skuId`; spec
  also requires a top-level `event` field.
- **`/payment-plan`** — SDK lacks the
  `extendInfo.ecommerceOrder` tree (11 fields incl.
  `ecommerceSubOrders`, `orderAmount`, `paymentType`) plus the
  same commerce-side SubOrder fields as `/payment-precheck`.

These are real production-side mismatches, not test-framework
artefacts; v0.2.3 will land the rename + missing-arg pass.

## [0.2.1] — 2026-05-06

Patch release fixing two issues caught by the post-tag completeness
review (`docs/internal/COMPLETENESS_REVIEW_V0.2.md`). v0.2.0 partners
should upgrade — the first issue causes guaranteed `400 PARAMS_MISSING`
on every call to four query GET methods.

### Fixed

- **Four query GET methods now take spec-required `externalReferenceUid`
  param.** v0.2.0 shipped `payment.QueryAuth(ctx, requestID)`,
  `payment.QueryCapture(ctx, requestID)`, `payment.QueryVoidAuth(ctx,
  requestID)`, and `refund.QueryRefund(ctx, requestID)` — but the spec
  requires BOTH `requestId` AND `externalReferenceUid` query params on
  each. First production call would have returned `400 PARAMS_MISSING`.
  Signatures are now `(ctx, requestID, externalReferenceUID)` for all
  four; `repayment.QueryRepayment` and `credit.QueryInformationResult`
  already had the correct shape and are unchanged.
- **`credit.PlatformInformation.UserCreditScore` is `*float64`.**
  v0.2.0 declared it as bare `float64` with `,omitempty` — a
  worst-case `0.0` score silently disappeared on the wire. Now matches
  the v0.1 precedent set by `payment.RequestExtendInfo.UserCreditScore`
  (pointer; pointer-nil signals absence; `0.0` round-trips visibly).

### Migration

Both fixes are public-API breaking but pre-1.0. Adapt callers:

```go
// before (v0.2.0 — broken)
resp, err := payment.New(c).QueryAuth(ctx, "req-1")

// after (v0.2.1)
resp, err := payment.New(c).QueryAuth(ctx, "req-1", externalRefUID)
```

```go
// before (v0.2.0 — silent zero loss)
score := 0.0
info.UserCreditScore = score

// after (v0.2.1)
score := 0.0
info.UserCreditScore = &score
```

## [0.2.0] — 2026-05-06

**28 new endpoints across 6 chunks.** v0.2 lights up the rest of the
spec surface: refund, bill, transaction, repayment, credit, plus
pre-checkout helpers, three new callback handlers, and a one-line
liveness probe. Eight sub-packages now mirror the `atomefin/payment`
shape; the umbrella `atomefin.Client` exposes `DoSignedGET` (signed
GET path with R13 wire-≡-canonical guarantee) and `HeartBeat`. Every
`v0.1.x` import path and field surface is preserved verbatim — pure
additions.

CI is green at tag time; openssl-anchored signing vectors continue
to verify R10/R11/R12/R13 invariants across all new types.

### Added — heart-beat liveness probe (v0.2 chunk #9)

- **`Client.HeartBeat(ctx context.Context) error`** — one-call signed
  liveness probe against `GET /heart-beat`. Returns `nil` on 2xx,
  `*atomefin.APIError` on non-2xx (envelope decoded where present),
  `*atomefin.TransportError` on transport failure. Body is read and
  drained but not surfaced — the spec does not define a stable
  response payload, so partners that need response inspection can
  call `Client.DoSignedGET(ctx, "/heart-beat", nil)` directly until
  the spec stabilises.
- Routed through `DoSignedGET` so retry policy, observer hooks, and
  ctx cancellation in backoff sleeps all apply uniformly. The empty
  canonical query (`sign.CanonicalQuery(nil) == ""`) signs cleanly
  and verifies — pinned by an openssl-anchored test.

### Tests

- `atomefin/heartbeat_test.go` — happy path (Method=GET,
  Path=`/heart-beat`, empty RawQuery), 4xx → `*APIError`, 5xx-then-
  200 retry, ctx-cancel-in-sleep, transport-error pass-through,
  nil-Client safety.

### Added — account-change callback handler (v0.2 chunk #8)

- **`atomefin/callback/account_change_handler.go`** —
  `AccountChangeHandler(v *Verifier, fn func(context.Context, *AccountChangeEvent) error)`
  inbound-only handler for `<accountChangeNotifyUrl>`. Same generic
  `handle[T]` core as RefundHandler — Content-Type, nosniff, full
  ack-envelope semantics, multi-cert verifier support.
- **`atomefin/callback/account_change_types.go`** —
  `AccountChangeEvent` (full callback envelope) wrapping
  `AccountChangeData` with `payment.AccountChanges` (the 11-field
  shared shape), event-type discriminator (`balance.adjust` /
  `account.status.change` / etc.), and a Q24-position-scoped
  `currentStatus` validator that accepts `ACCOUNT_CLOSED` (whereas
  `previousStatus` must reject it).
- Reuses `payment.AccountChanges` deliberately — account-change
  callbacks ride the same credit-change wire shape that
  `auth` / `capture` / `voidAuth` / `refund` responses already emit.
  Repayment, by contrast, gets its own `CommerceAccountChanges` —
  see chunk #5 below for the rationale.

### Tests

- `atomefin/callback/account_change_handler_test.go` — happy paths
  for both event types (balance increase, status close), tampered-
  body 401, multi-cert e2e, replay invokes user fn twice (partner-
  owned dedupe contract), 500 on user error / nil-verifier / nil-
  userFn, fixture-decode round-trip.

### Added — pre-checkout endpoints (v0.2 chunk #7)

- **`payment.New(c).PaymentPreCheck(ctx, *PaymentPreCheckRequest)`** —
  POST `/payment-precheck`. Eligibility / risk pre-flight before
  `/auth`; returns `*PaymentPreCheckResponse` with `Eligible` /
  `AvailableCredit` / `DeniedReason`. Auto-mints `RequestID` when
  empty (mirrors `payment.Auth`'s shape); table-driven validator
  rejects empty `externalReferenceUid`, zero `totalAmount`, non-IDR
  currency.
- **`payment.New(c).PaymentPlan(ctx, *PaymentPlanRequest)`** —
  POST `/payment-plan`. Returns the available installment-plan
  options (1/3/6/9/12 tenors per spec) with per-month
  `CommerceInstallmentDetail` breakdowns. `PaymentPlanData.Plans` is
  bare `json:"plans"` (NOT omitempty) — codifies the paginated-list
  pattern from chunks #3 / #4 so a 0-tenor response round-trips as
  `"plans":[]` rather than dropping to `null`. Validator enforces
  sub-order amount sum == `totalAmount`.

### Tests

- `atomefin/payment/precheck_test.go` (8 tests): success, auto-mint
  request-id, 4xx → APIError, validation table (5 rejection cases),
  R10 amount round-trip, R11 fractional rejection, R12 integer-
  literal-only emission.
- `atomefin/payment/plan_test.go` (10 tests): success (asserts
  `Method=POST`, `Path=/payment-plan`, body shape, plan ordering),
  auto-mint, 4xx → APIError, validation table (6 rejection cases
  including sum-mismatch), `GoldenRoundTrip` × 3 fixtures (full,
  empty, response-only), R10 on `totalAmount` / `principal` / `fee`
  / `interest` / `amount`, R11 fractional rejection, R12
  integer-literal-only across 6 amount keys.

### Added — credit lifecycle + account-ops (v0.2 chunk #6)

- **`atomefin/credit`** — new sub-package covering the spec's
  credit-onboarding lifecycle plus the two account-ops endpoints.
  Constructor: `credit.New(c)`. Account-ops live here (rather than
  in a separate `atomefin/account/` package) by domain cohesion —
  modify-application-info and close-account both operate on a
  credit-application identifier and share the same response
  envelope shape.
  - `Service.SubmitInformation(ctx, *CreditInformationParam) (*CreditInformationResponse, error)` —
    POST `/credit-information`. KYC start; returns a `requestId` +
    `jumpUrl` into the Atome KYC web flow.
  - `Service.SubmitApplication(ctx, *CreditApplicationParam) (*CreditApplicationResponse, error)` —
    POST `/credit-application`. Submit the credit application after
    KYC completes.
  - `Service.QueryResult(ctx, externalReferenceUID) (*CreditApplicationResponse, error)` —
    GET `/credit-result?externalReferenceUid=<id>`. Polls
    application terminal state. **Note:** the spec's `INPROGESS`
    literal (sic — missing R) is preserved verbatim on the wire to
    stay byte-compatible.
  - `Service.QueryInformationResult(ctx, externalReferenceUID, requestID) (*CreditInformationCollectResponse, error)` —
    GET `/credit-information-result`. Polls KYC-collection
    terminal state.
  - `Service.BalanceHistory(ctx, *BalanceHistoryParams) (*BalanceHistoryResponse, error)` —
    GET `/query-balance-history`. Paginated balance ledger.
    **Pagination uses `start`/`count` (per spec), NOT
    `pageNumber`/`pageSize`** — the server's pagination dialect
    differs here from bill / transaction. Auto-pagination wrapper
    deferred to a later chunk.
  - `Service.ModifyApplicationInfo(ctx, *CreditApplicationChangeParam) (*ModifyApplicationInfoResponse, error)` —
    POST `/modify-application-info`. Account-ops: edit a submitted
    credit application.
  - `Service.CloseAccount(ctx, *CloseAccountParam) (*CloseAccountResponse, error)` —
    POST `/close-account`. Account-ops: terminate the account.
- **`atomefin/callback/credit_application_handler.go`** — new
  `CreditApplicationHandler` with type alias
  `CreditApplicationEvent = credit.CreditApplicationResponse`
  (full-envelope alias matching `RefundEvent`). Terminal-only.
- **`atomefin/callback/credit_information_handler.go`** — new
  `CreditInformationHandler` with type alias
  `CreditInformationEvent = credit.CreditInformationCollectResponse`.
  Terminal-only.

### Tests

- `atomefin/credit/service_test.go` — happy path per endpoint
  (asserts method, path, body / query shape), 4xx → `*APIError`,
  nil-Client / nil-Service safety, `New(nil) == nil`.
- `atomefin/credit/validation_test.go` — table-driven rejections
  for every method (empty externalReferenceUid, oversize requestId,
  invalid pagination ranges, etc.).
- `atomefin/credit/marshal_audit_test.go` — `GoldenRoundTrip` ×
  fixtures for every wire shape (information request / response,
  application request / response success / processing / failed,
  query-result, information-result, change request, callbacks);
  R10 / R11 / R12 audits across every amount-bearing type.
- `atomefin/callback/credit_application_handler_test.go` /
  `credit_information_handler_test.go` — happy path, tampered-body
  401, multi-cert e2e, replay, 500 on user error, fixture decode.

### Fixtures

- `qa/testdata/credit_information_request.json`
- `qa/testdata/credit_information_response_success.json`
- `qa/testdata/credit_information_result_response.json`
- `qa/testdata/credit_information_result_failed.json`
- `qa/testdata/credit_application_request.json`
- `qa/testdata/credit_application_response_success.json`
- `qa/testdata/credit_application_response_processing.json`
- `qa/testdata/credit_application_response_failed.json`
- `qa/testdata/credit_application_change_request.json`
- `qa/testdata/callback_credit_application_terminal_success.json`
- `qa/testdata/callback_credit_information_terminal_success.json`

### Added — repayment sub-package (v0.2 chunk #5)

- **`atomefin/repayment`** — new sub-package mirroring the
  payment / refund Service shape. Constructor: `repayment.New(c)`.
  - `Service.Repayment(ctx, *RepaymentParam) (*RepaymentResponse, error)` —
    POST `/repayment-request` (signed body via
    `atomefin.MarshalSigning`, HTML-escape OFF). Auto-mints
    `RequestID` when empty.
  - `Service.QueryRepayment(ctx, requestID, externalReferenceUID) (*RepaymentResponse, error)` —
    GET `/repayment-result?requestId=...&externalReferenceUid=...`
    via `Client.DoSignedGET`. Polling alternative to the PROCESSING
    webhook.
  - `Service.RepaymentPollUntilTerminal(ctx, req, opts)` — reuses
    `payment.PollUntilTerminal` so backoff semantics are identical
    across Service families.
  - Types: `RepaymentParam`, `RepaymentResponse`, `RepaymentResult`,
    `RepaymentExtendInfo`, `RepaymentSettlement`, `SubOrderRepayment`,
    `RepaymentEvent` enum, `RepaymentStatus` enum, and —
    deliberately — **`CommerceAccountChanges`**.
- **`atomefin/callback/repayment_handler.go`** — new
  `RepaymentHandler` with type alias
  `RepaymentEvent = repayment.RepaymentResponse`. Terminal-only.

### Credit-change vectors

The spec defines two credit-change wire shapes, modelled as
separate Go types:
- **`payment.AccountChanges`** is the canonical credit-change vector
  for auth / capture / voidAuth / refund responses and the
  account-change callback.
- **`repayment.CommerceAccountChanges`** is the canonical credit-
  change vector for commerce-domain responses (repayment).

Each carries the field set defined by the spec for its respective
endpoint family.

### Tests

- `atomefin/repayment/service_test.go` — happy paths (`Repayment`,
  `QueryRepayment`), auto-mint, 4xx → `*APIError`,
  `RepaymentPollUntilTerminal` PROCESSING → SUCCESS, `New(nil) ==
  nil`, full nil-Service safety on every public method.
- `atomefin/repayment/validation_test.go` — table-driven rejections
  (nil request, oversize requestId, missing externalReferenceUid /
  authOrderId, zero repaymentAmount, sum-mismatch on sub-orders).
- `atomefin/repayment/marshal_audit_test.go` — `GoldenRoundTrip`
  per fixture, R10 on `RepaymentParam.RepaymentAmount` and
  `CommerceAccountChanges` deltas, R11 fractional rejection, R12
  integer-literal-only.
- `atomefin/callback/repayment_handler_test.go` — happy path,
  tampered-body 401, multi-cert e2e, replay invokes user fn twice,
  500 on user error / nil-verifier / nil-userFn, fixture decode.

### Fixtures

- `qa/testdata/repayment_request.json`
- `qa/testdata/repayment_response_success.json`
- `qa/testdata/repayment_response_processing.json`
- `qa/testdata/query_repayment_response.json`
- `qa/testdata/callback_repayment_terminal_success.json`

### Added — transaction sub-package (v0.2 chunk #4)

- **`atomefin/transaction`** — new GET-only sub-package, smaller
  mirror of bill. Constructor: `transaction.New(c)` (no
  `c.Transaction` accessor — preserves tree-shake).
  - `Service.Transactions(ctx, *TransactionsParams) (*TransactionsResponse, error)`
    — GET `/transactions`, paginated. Optional filters:
    `ExternalReferenceUID`, `AuthOrderID`, `TradeType`,
    `StartDate`, `EndDate`. Pass nil for default first page
    (`PageNumber=1`, `PageSize=20`).
  - `Service.TransactionDetail(ctx, tradeID) (*TransactionDetailResponse, error)`
    — GET `/transactionDetail?tradeId=<id>`. Validator rejects
    empty.
  - `Service.TransactionsAll(ctx, *TransactionsParams) ([]Transaction, error)`
    — auto-pagination iterator (mirrors `bill.BillsAll`).
  - Types (selective per architect's chunk-scoping note — only the
    types reachable from these two endpoints): `Transaction`
    (list-row), `TransactionDetail` (embeds Transaction + optional
    `BillID` / `FailureCode` / `Notes`).
  - `TradeType` enum (closed) — `AUTH` / `CAPTURE` / `VOID` /
    `REFUND` — with `IsValid` + `String`. Reuses `atomefin.Status`
    for the lifecycle field (`tradeStatus`).
  - `TransactionsResponse.IsSuccess() bool` /
    `TransactionDetailResponse.IsSuccess() bool` nil-safe helpers.
  - **`TransactionsData.Items` is bare `json:"items"` (NOT
    omitempty)** — codifies the paginated-list pattern from
    chunk #3 (bill): empty pages round-trip as `[]` rather than
    disappearing to `null`.
- Pure GETs reuse `Client.DoSignedGET` from chunk #1; no signed-body
  marshalling, no callback handler, no new openssl vector.

### Tests

- `atomefin/transaction/service_test.go` (13 tests):
  `TestService_Transactions_Success`,
  **`TestService_Transactions_MultiParam_R13_AtScale`** (7-param
  query, server reconstructs canonical, runs verifier, pins
  `r.URL.RawQuery == canonical` byte-equal),
  `TestService_TransactionDetail_Success` +
  `RejectsEmptyTradeID`, `TestService_TransactionsAll_AutoPaginates`,
  `TestService_Transactions_4xxBecomesAPIError`,
  `TestNew_NilClient_ReturnsNil`,
  `TestNilService_AllMethodsReturnError`,
  `TestTransactions_Validate` (3 rejection cases),
  `TestTradeType_IsValid` (4 valid + 4 invalid),
  `TestTradeType_StringIsWireLiteral`,
  `TestTransactions_NilParamsUsesDefaults`.
- `atomefin/transaction/marshal_audit_test.go` (8 tests):
  `GoldenRoundTrip` × 5 fixtures, R10 corpus on
  `Transaction.Amount`, R11 fractional rejection, R12
  integer-literal-only.

### Fixtures

- `qa/testdata/transactions_response.json` (3-row: AUTH+CAPTURE+REFUND)
- `qa/testdata/transactions_response_empty.json` (preserves `items: []`)
- `qa/testdata/transactionDetail_response.json` (CAPTURE with
  `billId` + `notes`)
- `qa/testdata/transactionDetail_response_minimal.json` (bare AUTH)
- `qa/testdata/transactionDetail_response_failed.json` (FAILED +
  `failureCode = RISK_REJECT`)

### Documentation

- `README.md` — two new endpoint table rows for `/transactions`
  and `/transactionDetail`; package map gains
  `atomefin/transaction`.

### Added — bill sub-package (v0.2 chunk #3)

- **`atomefin/bill`** — new GET-only sub-package mirroring the
  payment / refund Service shape. Constructor pattern: `bill.New(c)`
  (no `c.Bill` accessor — preserves tree-shake).
  - `Service.Bills(ctx, *BillsParams) (*BillsResponse, error)` —
    GET `/bills`, paginated. Optional filters via `BillsParams`:
    `ExternalReferenceUID`, `BillID`, `StartDate`, `EndDate`. Pass
    nil for the default first page (`PageNumber=1`, `PageSize=20`).
  - `Service.BillDetail(ctx, billID) (*BillDetailResponse, error)` —
    GET `/billDetail?billId=<yyyyMM>`. Validator rejects empty.
  - `Service.BillsUnpaid(ctx, *BillsUnpaidParams) (*BillsResponse, error)` —
    GET `/billUnpaid`, paginated. Server pre-filters to unpaid
    rows.
  - `Service.BillsAll(ctx, *BillsParams) ([]Bill, error)` —
    convenience auto-pagination iterator that walks every page
    until short-page or `Total` reached. ctx-cancellable between
    pages.
  - Types: `Bill`, `BillDetail` (embeds `Bill` plus `Orders` /
    `Discounts`), `BillOrder`, `BillDiscounts`, `Discount`,
    `DiscountDetail`. Plus `OverdueStatus` enum
    (`ON_TIME` / `GRACE_PERIOD` / `OVERDUE`) with `IsValid` +
    `String` helpers.
  - `BillsResponse.IsSuccess() bool` / `BillDetailResponse.IsSuccess() bool`
    nil-safe convenience helpers.
  - All money fields are `int64` minor units; `Currency` uses the
    named-type from v0.1.1.
  - Default pagination constants: `bill.DefaultPageNumber = 1`,
    `bill.DefaultPageSize = 20`. Validators cap `PageSize <= 1000`
    as a sanity guard (server-side cap may be stricter).
- Pure GETs reuse `Client.DoSignedGET` from chunk #1; no signed-body
  marshalling, no callback handler.

### Tests

- `atomefin/bill/service_test.go` (15 tests):
  `TestService_Bills_Success` (asserts `Method=GET`, `Path=/bills`,
  RawQuery sorted alphabetically),
  **`TestService_Bills_MultiParam_R13_AtScale`** (architect's stress
  case — 6-param query, server reconstructs canonical from
  `r.URL.Query()`, runs verifier, also pins
  `r.URL.RawQuery == canonical` byte-equal),
  `TestService_BillDetail_Success`,
  `TestService_BillDetail_RejectsEmptyBillID`,
  `TestService_BillsUnpaid_Success`,
  `TestService_BillsAll_AutoPaginates` (3 pages × 2 rows + 1 short
  page → terminates correctly),
  `TestService_BillsAll_TerminatesOnEmpty`,
  `TestService_Bills_4xxBecomesAPIError`,
  `TestNew_NilClient_ReturnsNil`,
  `TestNilService_AllMethodsReturnError` (no panic on every public
  method),
  `TestBills_Validate` (3 rejection cases),
  `TestBillsUnpaid_Validate` (3 rejection cases),
  `TestOverdueStatus_IsValid`,
  `TestOverdueStatus_StringIsWireLiteral`,
  `TestBills_NilParamsUsesDefaults`.
- `atomefin/bill/marshal_audit_test.go` (12 tests):
  `GoldenRoundTrip` × 5 fixtures (bills full / empty,
  billDetail full / no-discounts, billsUnpaid); R10 amount corpus
  on `Bill.TotalAmount`, `BillOrder.Amount`, `Discount.Amount`;
  R11 fractional rejection on `Bill.TotalAmount` and
  `Discount.Amount`; R12 integer-literal-only on `Bill` and
  `BillDiscounts`.

### Fixtures

- `qa/testdata/bills_response.json` — 2-row paginated response.
- `qa/testdata/bills_response_empty.json` — empty page (preserves
  `bills: []` shape — `BillsData.Bills` is bare `json:"bills"` so
  empty pages don't drop to `null`).
- `qa/testdata/billDetail_response.json` — full single-bill incl.
  orders + discount summary.
- `qa/testdata/billDetail_response_no_discounts.json` — variant
  without the optional discounts block.
- `qa/testdata/billsUnpaid_response.json` — overdue-bill row.

### Documentation

- `README.md` — three new endpoint table rows for `/bills` /
  `/billDetail` / `/billUnpaid`; package map gains an
  `atomefin/bill` row.

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

[Unreleased]: https://github.com/atome-fin/atome-fin-go-sdk/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.3.0
[0.2.3]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.2.3
[0.2.2]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.2.2
[0.2.1]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.2.1
[0.2.0]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.2.0
[0.1.1]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.1.1
[0.1.0]: https://github.com/atome-fin/atome-fin-go-sdk/releases/tag/v0.1.0
