# `auth_capture` example

End-to-end demo of the SDK's outbound flow: builds an `atomefin.Client`,
issues `/auth`, prints the response, and (optionally) issues `/capture`
against the resulting `authOrderId`.

## Run

```sh
go build ./examples/auth_capture/
ATOME_FIN_PRIV_KEY_PEM=/etc/atome/partner.pem \
ATOME_FIN_SESSION_ID=session-xyz \
ATOME_FIN_EXTERNAL_UID=user-42 \
ATOME_FIN_RUN_CAPTURE=1 \
./auth_capture
```

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `ATOME_FIN_PRIV_KEY_PEM` | *(required)* | Path to partner RSA-2048 private key (PEM) |
| `ATOME_FIN_PARTNER_ID` | *(optional)* | Partner identifier — log-enrichment label only (Q7 RESOLVED: not transmitted on the wire) |
| `ATOME_FIN_SESSION_ID` | *(required)* | Per-`/auth` `sessionid` header value (max 64 chars) |
| `ATOME_FIN_EXTERNAL_UID` | `user-42` | Partner-side user identifier |
| `ATOME_FIN_ENV` | `test` | One of `test`, `pre`, `prod` (placeholder URLs from the spec) |
| `ATOME_FIN_BASE_URL` | unset | Explicit base URL — overrides `ATOME_FIN_ENV` |
| `ATOME_FIN_TOTAL_AMOUNT` | `1500000` | Integer minor units |
| `ATOME_FIN_PERIOD_TYPE` | `3` | Installment tenor `1\|3\|6\|9\|12` |
| `ATOME_FIN_RUN_CAPTURE` | unset | `1` issues `/capture` after a SUCCESS auth |

## Notes

- The example uses the SDK's auto-minted `RequestID` (a ULID-like 32-char
  hex). Partners with their own idempotency-key scheme should set
  `req.RequestID` explicitly.
- `Sessionid` travels in the HTTP `sessionid` header via the SDK's
  `atomefin.WithRequestHeader` plumbing — it never appears in the
  signed JSON body.
- The base URLs for `test` / `pre` / `prod` are still spec placeholders
  (DESIGN.md Q1). Use `ATOME_FIN_BASE_URL` to override once the partner
  confirms the production gateway URL.
