# `credit_information_pre` example

Pre-production smoke test for `POST /credit-information`: validates the
partner signing key, Atome **encrypt** public key, hybrid-encryption
pipeline, and live gateway connectivity.

## Prerequisites

| Key | Role | Env var |
|-----|------|---------|
| Partner signing **private** key (S1) | Signs `Authorization` | `ATOME_FIN_PRIV_KEY_PEM` |
| Atome **encrypt** public key (E4) | Wraps per-request AES key in `Encrypt:` header | `ATOME_FIN_ATOME_ENCRYPT_CERT_PEM` |

Do **not** use the Atome **signing** public key here — credit-information
requires the separate encrypt certificate pair (see `docs/CERTIFICATES.md`).

## Run (dry-run: local crypto only)

```sh
go build ./examples/credit_information_pre/

ATOME_FIN_PRIV_KEY_PEM=/path/to/partner_sign_priv.pem \
ATOME_FIN_ATOME_ENCRYPT_CERT_PEM=/path/to/atome_encrypt_pub.pem \
ATOME_FIN_DRY_RUN=1 \
./credit_information_pre
```

## Run (full: local crypto + pre env API)

```sh
ATOME_FIN_PRIV_KEY_PEM=/path/to/partner_sign_priv.pem \
ATOME_FIN_ATOME_ENCRYPT_CERT_PEM=/path/to/atome_encrypt_pub.pem \
ATOME_FIN_EXTERNAL_UID=grab-user-001 \
ATOME_FIN_MOBILE_NUMBER=+628129801929 \
ATOME_FIN_EMAIL=user@example.com \
ATOME_FIN_FULL_NAME="Test User" \
./credit_information_pre
```

Default target: `https://id-api-pre.apaylater.net/grabpaylater` (`ATOME_FIN_ENV=pre`).

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `ATOME_FIN_PRIV_KEY_PEM` | *(required)* | Partner RSA-2048 **signing** private key (PEM file path) |
| `ATOME_FIN_ATOME_ENCRYPT_CERT_PEM` | *(required)* | Atome RSA-2048 **encrypt** public key (PEM file path) |
| `ATOME_FIN_EXTERNAL_UID` | `grab-pre-<unix>` | Partner user identifier |
| `ATOME_FIN_MOBILE_NUMBER` | `+628129801929` | User mobile with country code |
| `ATOME_FIN_EMAIL` | `grab-pre-test@example.com` | User email |
| `ATOME_FIN_FULL_NAME` | `Grab Pre Test` | `applicationEssentialInfo.ocrResult.fullName` |
| `ATOME_FIN_ENV` | `pre` | `pre` or `prod` |
| `ATOME_FIN_BASE_URL` | unset | Override base URL |
| `ATOME_FIN_DRY_RUN` | unset | `1` = skip live HTTP call |
| `ATOME_FIN_DEBUG` | unset | `1` = log raw request/response bodies |

## What gets checked

1. **Local** — JSON marshal, AES hybrid encrypt, RSA2 sign over encrypted body, signature self-verify
2. **Live** — `credit.SubmitInformation` to pre env; prints `code` / `status` / `jumpUrl`

## Common failures

| Symptom | Likely cause |
|---------|--------------|
| `INVALID_SIGNATURE` (401) | Partner signing public key not registered on pre |
| `INVALID_ENCRYPTION` | Wrong Atome encrypt public key for pre env |
| `ACTIVE_ACCOUNT` | Re-use `ATOME_FIN_EXTERNAL_UID` for a user who already has an account |
| `encryptAtomePublicCert` validation error | PEM path wrong, or signing cert used instead of encrypt cert |
