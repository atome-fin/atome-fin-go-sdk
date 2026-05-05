# QA Fixture Corpus

Each file in this directory is a **golden** wire-format example for one
spec endpoint shape. Lead-coder is expected to add the missing fixtures
once T3 lands and the matching Go structs exist.

Naming: `<endpoint>_<direction>_<scenario>.json`.

## Present

| File | Purpose | Used by |
|---|---|---|
| `auth_request.json` | Minimal `/auth` request body, two sub-orders, no extendInfo | `qa/marshal` self-tests |
| `auth_response_success.json` | `/auth` HTTP-200 with terminal `SUCCESS` | `qa/marshal` self-tests |

## Required for T3 sign-off (lead-coder to add)

Outbound:

- [ ] `auth_request_with_extend.json` — `/auth` with full `extendInfo`
- [ ] `auth_response_processing.json` — async `PROCESSING` envelope
- [ ] `auth_response_failed_credit.json` — terminal `FAILED` with
      `failureCode = USER_CREDIT_LIMIT_INSUFFICIENT`
- [ ] `auth_response_failed_risk.json` — terminal `FAILED` with
      `failureCode = RISK_REJECT`
- [ ] `auth_response_account_change.json` — `accountChanges` populated
- [ ] `capture_request.json` — minimal `/capture`
- [ ] `capture_response_success.json`
- [ ] `capture_response_processing.json`
- [ ] `capture_response_credit_insufficient.json` — sync 200 with
      business code `USER_CREDIT_LIMIT_INSUFFICIENT`
- [ ] `voidauth_request.json`
- [ ] `voidauth_response_success.json`
- [ ] `voidauth_response_auth_expired.json` — HTTP 400 / `AUTH_EXPIRED`

Errors:

- [ ] `error_400_params_missing.json`
- [ ] `error_400_wrong_format.json`
- [ ] `error_400_capture_amount_exceed.json` (capture only)
- [ ] `error_400_session_not_found.json` (auth only)
- [ ] `error_401_invalid_signature.json`
- [ ] `error_500_server_error.json`

Inbound (callbacks):

- [ ] `callback_auth_terminal_success.json`
- [ ] `callback_auth_terminal_failed.json`
- [ ] `callback_capture_terminal_success.json`
- [ ] `callback_ack_success.json`

Each file should be wired into the SDK via:

```go
marshal.GoldenRoundTrip[<TypeName>](t, "../../qa/testdata/<file>.json")
```

(or `marshal.StrictDecode[<TypeName>]` for files used purely for
schema-presence assertions).
