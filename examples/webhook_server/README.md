# `webhook_server` example

Minimal HTTP server that mounts `callback.AuthHandler` and
`callback.CaptureHandler` against the Atome public cert and prints
each terminal-state event.

## Run

Single-cert (the common path):

```sh
go build ./examples/webhook_server/
ATOME_FIN_ATOME_CERT_PEM=/etc/atome/atome.crt.pem \
ATOME_FIN_LISTEN_ADDR=:8443 \
./webhook_server
```

Multi-cert (rotation overlap, the spec):

```sh
ATOME_FIN_ATOME_CERT_PEMS=/etc/atome/atome.old.pem:/etc/atome/atome.new.pem \
ATOME_FIN_LISTEN_ADDR=:8443 \
./webhook_server
```

## Routes

| Path | Handler |
|---|---|
| `POST /atome/auth` | `callback.AuthHandler` — auth-terminal events |
| `POST /atome/capture` | `callback.CaptureHandler` — capture-terminal events |
| `GET /healthz` | trivial liveness probe |

## Environment variables

| Variable | Default | Purpose |
|---|---|---|
| `ATOME_FIN_ATOME_CERT_PEM` | *(required, single)* | Path to Atome public cert (PEM) |
| `ATOME_FIN_ATOME_CERT_PEMS` | *(required, multi)* | Colon-separated PEM paths — overrides the single-cert variable |
| `ATOME_FIN_LISTEN_ADDR` | `:8080` | Listen address |

## Behavioural contract

The handlers implement the spec's terminal-state ack semantics
(DESIGN.md §8):

- 200 + `{code: SUCCESS, message: ack}` on handler success
- 401 + `{code: INVALID_SIGNATURE}` on bad / missing signature
- 500 + `{code: SERVER_ERROR, message: <reason>}` on handler error → Atome retries
- 400 + `{code: WRONG_PARAMS_FORMAT}` on oversize body / malformed JSON
- 405 on non-POST verbs

Every response sets `Content-Type: application/json; charset=utf-8`
and `X-Content-Type-Options: nosniff`.

## Idempotency

Atome callbacks are at-least-once. The example handlers above are
*illustrative*: real partner code MUST dedupe on
`event.Data.RequestID` so a duplicate delivery does not mark the same
order paid twice.
