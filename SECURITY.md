# Security policy

This project handles partner-side payment authorization,
capture, refund, repayment, and credit-flow signing for the
atome-fin gateway. The cryptographic surface is small but
load-bearing — please report issues responsibly.

## Reporting a vulnerability

Email **security@atome-fin.example** (replace with the partner
ops contact you've been given) with:

- a short description of the issue,
- a reproduction (test snippet or `go test`-style scenario is
  ideal),
- the SDK version (`git rev-parse HEAD` or the tag you're on),
- whether you've already shared the report with anyone outside
  the partner-ops team.

We aim to acknowledge reports within 2 business days. Please do
not file a public GitHub issue or pull request describing the
vulnerability before it is fixed; we'll coordinate disclosure
once a patch is ready and partners on the affected version line
have had a chance to upgrade.

## Supported versions

| Version | Status |
|---|---|
| latest released `v0.x` minor (currently v0.5.x) | Active patch line — security fixes ship as v0.5.y patch releases. |
| previous released `v0.x` minor | Best-effort — security fixes back-ported when feasible. |
| pre-1.0 (`v0.x`) generally | The upstream API is still draft (see [DESIGN.md §13](DESIGN.md#13-open-questions-for-the-partner--atome)); minor versions may break. Production deployments should pin to a tagged release and watch the [CHANGELOG](CHANGELOG.md). |

`v1.0.0` will lock down the supported-version matrix per
standard semver — until then, please upgrade to the latest
patch on the latest minor when reporting.

## Cryptographic surface

The SDK ships these crypto primitives. Each is small enough to
audit; the test surface includes openssl-anchored vectors
(`sign/external_vector_test.go`,
`atomefin/encrypt/external_vector_test.go`) that pin algorithm
choice byte-for-byte.

### Signing — `atomefin/sign/`

- **RSA-2048 PKCS#1 v1.5 over SHA-256** (default) and **RSA-PSS
  over SHA-256 with 32-byte salt** (opt-in via
  `sign.WithSaltedPSS`).
- Min key bit-length **2048** enforced at signer-construction
  time (`sign.MinKeyBits`).
- Wire signature is **base64-standard-encoded** (NOT base64-url).
- The signing canonical for GET requests is **first-value per
  key** (alphabetical key sort, RFC 3986 `%20` for space). The
  wire query keeps every value when partners supply multi-value
  `url.Values` — this asymmetric semantic is the v0.5.1 fix and
  matches the upstream gateway.
- External openssl vector at `atomefin/sign/testdata/external_*`
  pins the algorithm choice across SDK releases.

### Hybrid encryption — `atomefin/encrypt/`

Used **only** by the two credit-flow POSTs
(`/credit-information`, `/credit-application`) per the partner
protocol (Q31 — Q34, RESOLVED 2026-05-06). All other endpoints
travel plaintext.

- **AES-ECB-PKCS5** for the body cipher.
- **RSA-PKCS#1 v1.5** (NOT OAEP) for the per-request AES key
  wrap. Min 2048-bit RSA modulus enforced.
- 32-byte AES key restricted to A — Z (partner-mandated). The
  generator is **rejection-sampled** to avoid the modulo bias
  the upstream reference impl tolerates; a 100k-key statistical
  uniformity test pins the distribution.
- ECB by mandate, NOT a Go-side preference. `crypto/cipher`
  deliberately omits ECB because of the known plaintext-pattern
  leak; the partner protocol nonetheless requires it (mirrors
  Java's `Cipher.getInstance("AES/ECB/PKCS5Padding")`).
  `atomefin/encrypt/aes.go` walks the cipher block-by-block via
  `aes.NewCipher`'s `Encrypt`/`Decrypt` to implement the
  mandated mode without contradicting Go's standard-library
  design. **A top-of-file comment names this; please do not
  "fix" it to a stronger mode without a partner-protocol
  change.**
- Authorization signs the **encrypted body bytes** (NOT the
  plaintext). Inbound (callback) callers verify the signature
  on the encrypted body before decrypting. The ordering is:
  outbound: encrypt → sign; inbound: verify → decrypt.

### Callbacks — `atomefin/callback/`

- Verification uses the **multi-cert verifier slot**: callbacks
  signed by either of the configured Atome public keys verify
  successfully. This is the rotation-overlap design hook.
- Tampered-body / wrong-key signatures return HTTP 401 with the
  canonical `INVALID_SIGNATURE` ack envelope — the user-supplied
  callback function is never invoked on a failed-verify path.
- Body is read via a 1 MiB `io.LimitReader` cap (`WithBodyLimit`
  to override).
- Atome callbacks are **at-least-once**. Deduping on
  `event.Data.RequestID` is the partner's responsibility; the
  SDK does not silently swallow duplicates.

### Test surface — `atomefin/mock/`

The `mock` package ships **bundled test keypairs** under
`atomefin/mock/testdata/`. They are clearly labelled in their
PEM headers and accessible only via `WithMockKeysAllowed()`
(opt-in). **These keys are public and FOR TESTING ONLY.**
Production systems must never accept them.

`mock.NewClient(t, ...)` refuses to construct when
`WithEnvironment(EnvProd)` is supplied — `t.Fatalf` with a
clear message. The protective intent: nothing in
`atomefin/mock` should ever co-exist with a production
configuration accidentally.

## What is NOT in scope for security reports

- Bugs in the upstream atome-fin gateway itself — please report
  those to your partner-ops contact, not to this SDK.
- Test-only `*_test.go` code paths or `mock` package surface —
  these are sandbox-realism helpers; they are not on the
  outbound production code path.
- Behaviour of `qa/specserver`'s drift-detection harness — it's
  a CI gate, not a production verification primitive.
- Pinned spec snapshot drift (`internal/spec/testdata/swagger-*.yaml`).
  Spec drift is tracked separately via `make test-spec-drift`
  and the documented bump workflow.

## Coordinated disclosure

If a vulnerability requires partner-side action (key rotation,
gateway-side fix, etc.), we'll coordinate disclosure timing
with the upstream atome-fin operations team and send affected
partners advance notice via the partner-ops channel. Public
disclosure (release notes + CHANGELOG entry) follows the
patched SDK release.

## See also

- [DESIGN.md](DESIGN.md) — full architectural reference,
  including the open-question table at §13.
- [CHANGELOG.md](CHANGELOG.md) — historic security-relevant
  fixes are flagged in their release entry (v0.2.1 §2.1
  externalReferenceUid; v0.5.1 multi-value canonical asymmetry;
  v0.5.2 ctx leak through auto-callback signing path).
