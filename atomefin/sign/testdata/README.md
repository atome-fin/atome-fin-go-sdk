# `atomefin/sign/testdata/` — external openssl vector

This directory holds an **independently produced** RSA-2048 / SHA-256 /
PKCS#1-v1.5 signature vector used by `external_vector_test.go` to prove
the SDK's `Signer` / `Verifier` pair agrees with `openssl(1)` byte-for-
byte. Without an external reference, a bug that flipped to PSS, used
SHA-512, or perturbed the canonical bytes would silently pass the
existing Go round-trip tests.

## Files

| File | Purpose |
|---|---|
| `external_priv.pem` | RSA-2048 PRIVATE KEY in PKCS#1 PEM form |
| `external_pub.pem`  | matching SubjectPublicKeyInfo PEM |
| `external_body.json` | fixed POST signing-canonical bytes (compact JSON, no trailing newline — `printf`, not `echo`) |
| `external_sig.b64`  | openssl base64-standard signature over `external_body.json` |
| `external_query_canonical.txt` | fixed GET signing-canonical bytes (sorted-key `k=v&k=v`, no trailing newline) |
| `external_query_sig.b64` | openssl base64-standard signature over `external_query_canonical.txt` |

## Algorithm fingerprint

- Key: RSA-2048 (modulus = 2048 bits)
- Hash: SHA-256
- Padding: PKCS#1 v1.5 (RSASSA-PKCS1-v1_5)
- Signature encoding: base64 standard (NOT URL-safe; padding preserved)

This is the "RSA2" the apaylater spec refers to under DESIGN.md §1.3 /
§4. The "salted" / PSS variant is gated on `sign.WithSaltedPSS` and
out of scope for this vector.

## Regen command

If you ever need to rotate the vector (e.g. fresh key after a leak
during local dev), run:

```sh
cd atomefin/sign/testdata

# 1. Fresh keypair.
openssl genrsa -out external_priv.pem 2048
openssl rsa -in external_priv.pem -pubout -out external_pub.pem

# 2. Body — keep BYTE-IDENTICAL across edits. printf NOT echo, no trailing newline.
printf '{"requestId":"01HABC1234567890ABCDEFGHJK","externalReferenceUid":"user-42","totalAmount":1500000,"periodType":3,"subOrders":[{"subOrderId":"so-1","amount":1500000,"quantity":1}]}' > external_body.json

# 3. Sign with PKCS#1 v1.5 / SHA-256 (`-sha256` selects digest; default padding is v1.5).
openssl dgst -sha256 -sign external_priv.pem -out external_sig.bin external_body.json

# 4. Base64 standard, single line.
openssl base64 -in external_sig.bin | tr -d '\n' > external_sig.b64
echo "" >> external_sig.b64

# 5. Throw away the binary; we keep only the base64 form.
rm external_sig.bin

# 6. Run the tests — they MUST pass.
cd ../../..
go test ./atomefin/sign/... -run TestExternalVector
```

## Paper trail (originally committed 2026-05-05)

Generated with **LibreSSL 3.3.6**. SHA-256 of each artefact at commit
time:

```
external_body.json              42ea578933a77fd305d8b56d365f25710fe8882727176445223b79913747e443
external_priv.pem               0f86925595d8c35273591cffe0e6fd12435934084341b709d8653f2a26f3e58b
external_pub.pem                3dbf85bc1d8e78fdd01d026c192936493d8007a111fb0d0d87877549b5e049fb
external_sig.b64                1e70f2a3964fb40e2808f4c3691269d16855076664292b6b781aaf4672d64779
external_query_canonical.txt    732909b3380b7661c4ac75df51c2e168d91f8623761b7cf72622d45e76099d3d
external_query_sig.b64          c6f0d2a5350ada8600fc52867369ae0f359d5ac0291ab025cc90baad621ac351
```

### GET-path canonical (forward-compat)

The query vector regen mirrors the body vector but signs the
sorted-key canonical instead of a JSON body:

```sh
cd atomefin/sign/testdata
printf 'externalReferenceUid=user-42&periodType=3&requestId=01HABC1234567890ABCDEFGHJK&totalAmount=1500000' > external_query_canonical.txt
openssl dgst -sha256 -sign external_priv.pem -out external_query_sig.bin external_query_canonical.txt
openssl base64 -in external_query_sig.bin | tr -d '\n' > external_query_sig.b64
echo "" >> external_query_sig.b64
rm external_query_sig.bin
```

The keys are alphabetically sorted (`externalReferenceUid` < `periodType`
< `requestId` < `totalAmount`) — exactly what `sign.CanonicalQuery`
emits — so the openssl signature verifies against the SDK output
byte-for-byte.

> The private key is **test-only** — generated solely to anchor this
> vector. Do not reuse it for any production-adjacent purpose.

## What the tests assert

`atomefin/sign/external_vector_test.go`:

- `TestExternalVector_VerifyWithGo` — Go verifier accepts the openssl
  signature; SDK and reference agree.
- `TestExternalVector_SignWithGoMatchesOpenssl` — Go signer over the
  same key + body produces the IDENTICAL base64 string. PKCS#1 v1.5
  is deterministic, so any drift here is wire-incompatible.
- `TestExternalVector_TamperedBodyFails` — flipping any byte of the
  body causes the verifier to return `sign.ErrSignature`.
- `TestExternalVector_CanonicalQueryRoundTrip` — the GET-path
  forward-compat anchor: `sign.CanonicalQuery` output matches the
  openssl-signed canonical byte-for-byte AND the openssl signature
  verifies against `sign.CanonicalQuery`'s output.
- `TestExternalVector_PSSDoesNotMatchPKCS1v15` — a PSS-configured
  verifier REJECTS the PKCS#1-v1.5 signature, pinning the default
  scheme.

If any of those five ever fail, **stop the world**: either openssl or
the SDK has drifted, and you cannot ship a signing-correctness
regression.
