# Polling: configuration and troubleshooting

This guide covers the SDK's `*PollUntilTerminal` helpers: how they
work, how to turn on their logs, and how to find out why a poll did
not reach a terminal status.

Requires **v0.8.11** or later.

## How polling works

The spec has no status-poll endpoint for `/auth`, `/capture`,
`/refund` or `/repayment-request`. The helpers re-submit the original
POST with the **same `requestId`**. The server treats that as an
idempotent retry and returns the current result, until `data.status`
is `SUCCESS` or `FAILED`.

| Helper | Endpoint |
|---|---|
| `payment.Service.AuthPollUntilTerminal` | `POST /auth` |
| `payment.Service.CapturePollUntilTerminal` | `POST /capture` |
| `refund.Service.RefundPollUntilTerminal` | `POST /refund` |
| `repayment.Service.RepaymentPollUntilTerminal` | `POST /repayment-request` |

The loop stops on the first of:

1. a terminal `data.status` (`SUCCESS` or `FAILED`);
2. a **synchronous business rejection**: HTTP 200 with a non-`SUCCESS`
   `code` and no `data.status` (`data` is `null`, absent, or `{}`),
   e.g. `USER_CREDIT_LIMIT_INSUFFICIENT` or
   `REFUNDABLE_AMOUNT_INSUFFICIENT`. Re-submitting would only return
   the same rejection;
3. `PollOptions.MaxWait` elapsing;
4. the caller's `ctx` being cancelled or reaching its deadline;
5. an HTTP / transport error that survived the client's retry policy.

`MaxWait` bounds both in-flight requests and backoff sleeps, so a call
never runs longer than `MaxWait` (or the caller's `ctx` deadline,
whichever is earlier).

## Configuration

```go
opts := payment.PollOptions{
    MaxWait:      120 * time.Second, // total budget; default 30s
    InitialDelay: time.Second,       // wait before the 2nd attempt; default 250ms
    MaxDelay:     15 * time.Second,  // cap on a single wait; default 8s
    Multiplier:   2.0,               // backoff factor; default 2.0
}
resp, err := refund.New(c).RefundPollUntilTerminal(ctx, req, opts)
```

With the values above the waits between attempts are 1s, 2s, 4s, 8s,
15s, 15s, ... until 120s have passed.

Each attempt is also subject to the client's per-request timeout
(`atomefin.WithTimeout`) and retry policy (`atomefin.WithRetry`).

## Handling the result

```go
resp, err := svc.RefundPollUntilTerminal(ctx, req, opts)

var rej *atomefin.BusinessRejectionError
switch {
case err == nil:
    // Terminal: check resp.Data.Status (SUCCESS / FAILED) and
    // resp.Data.FailureCode.
case errors.As(err, &rej):
    // Synchronous business rejection. Retrying will not help.
    // Handle rej.Code, e.g. REFUNDABLE_AMOUNT_INSUFFICIENT.
case errors.Is(err, context.DeadlineExceeded):
    // MaxWait (or your ctx deadline) elapsed before a terminal status.
    // resp is the last response received, usually PROCESSING.
    // Wait for the callback, or check later with QueryRefund /
    // QueryRepayment.
default:
    // HTTP / transport / signing error. See atomefin.APIError,
    // atomefin.TransportError, atomefin.SignatureError.
}
```

> **Upgrading from v0.8.10 or earlier:** refund and repayment
> rejections used to surface as a `MaxWait` timeout after polling for
> the full budget. They now return `*atomefin.BusinessRejectionError`
> on the first response. If you treated a polling timeout as "still
> processing", add the `BusinessRejectionError` branch above.

## Enabling logs

The SDK is silent by default. Pass a logger when building the client:

```go
logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelDebug, // Debug: every round. Info: exit lines only.
}))

c, err := atomefin.New(
    // ... your existing options
    atomefin.WithLogger(transport.NewSlogLogger(logger)),
)
```

Any type implementing `transport.Logger` (`Debug` / `Info` / `Warn` /
`Error` with key-value pairs) works as well. No change to the polling
calls is needed.

### Log lines

**One line per non-terminal round**, at Debug:

```json
{"level":"DEBUG","msg":"atomefin: poll round","op":"/refund","request_id":"REFUND-123","round":1,"elapsed":312000000,"status":"PROCESSING","code":"SUCCESS","message":"success","next_delay":1000000000}
```

**Exactly one line when the loop exits**. `atomefin: poll finished` at
Info for a terminal status, `atomefin: poll stopped` at Warn otherwise:

```json
{"level":"WARN","msg":"atomefin: poll stopped","op":"/refund","request_id":"REFUND-123","round":1,"elapsed":298000000,"status":"","code":"REFUNDABLE_AMOUNT_INSUFFICIENT","message":"...","reason":"business_rejection"}
```

`slog.NewJSONHandler` writes durations (`elapsed`, `next_delay`,
`max_wait`) as integer nanoseconds; `slog.NewTextHandler` writes them
as `312ms`.

| Field | Meaning |
|---|---|
| `op` | Endpoint being polled (`/auth`, `/capture`, `/refund`, `/repayment-request`) |
| `request_id` | The idempotency key re-submitted on every round |
| `round` | Attempt number, starting at 1 |
| `elapsed` | Time since the poll started |
| `status` | `data.status` of the latest response (empty if none) |
| `code` / `message` | Envelope `code` and `message` of the latest response |
| `next_delay` | Wait before the next round (round lines only) |
| `reason` | Why the loop exited (exit line only), see below |
| `max_wait` / `err` | Present for `max_wait_exceeded` / `parent_context_done` and `request_error` |

Sensitive keys (`Authorization`, `sessionid`, `externalReferenceUid`,
shipping name / phone, ...) are redacted by `transport.NewSlogLogger`.
None of them are emitted by the polling lines.

## Troubleshooting

Look up the exit line for the affected `request_id` and read `reason`:

| `reason` | What happened | What to do |
|---|---|---|
| `terminal` | Reached `SUCCESS` or `FAILED` | Nothing. Check `status` and `failureCode` |
| `business_rejection` | Synchronous rejection. See `code` | Handle as a business error; do not retry with the same request |
| `max_wait_exceeded` with `status=PROCESSING` | The server reported in-progress for the whole budget | Send the `request_id` to Atome for investigation; rely on the callback for the final result |
| `max_wait_exceeded` with empty `status` | Responses were neither terminal nor a recognised rejection | Send us the raw response body for that `request_id` |
| `parent_context_done` | Your `ctx` was cancelled or hit its deadline before `MaxWait` | Check the timeout of the calling code (HTTP handler, job runner, ...) |
| `request_error` | An HTTP / network error survived the retry policy. See `err` | Check network, gateway reachability, and signing configuration |

When reporting a polling issue to Atome, please include:

- SDK version (`atomefin.Version()`);
- the `PollOptions` in use;
- all `atomefin: poll round` / `poll stopped` / `poll finished` lines
  for the affected `request_id`.

## Custom polling loops

To poll with your own request function (for example, GET-based
polling via `QueryRepayment`) while keeping the same logs and rejection
handling, use `payment.PollUntilTerminalOrRejected`:

```go
status := func(r *repayment.RepaymentResponse) atomefin.Status {
    if r == nil || r.Data == nil {
        return ""
    }
    return r.Data.Status
}
resp, err := payment.PollUntilTerminalOrRejected(ctx, opts,
    payment.PollTrace[repayment.RepaymentResponse]{
        Logger:    c.Logger(),
        Op:        "/repayment-result",
        RequestID: requestID,
        Code: func(r *repayment.RepaymentResponse) (atomefin.Code, string) {
            return r.Code, r.Message
        },
    },
    status,
    func(r *repayment.RepaymentResponse) (atomefin.Code, string, bool) {
        return r.Code, r.Message, payment.IsSyncRejection(r.Code, status(r))
    },
    func(ctx context.Context) (*repayment.RepaymentResponse, error) {
        return svc.QueryRepayment(ctx, requestID, externalReferenceUID)
    },
)
```

Pass `nil` as the rejection function to only get logs. The plain
`payment.PollUntilTerminal` does neither logging nor rejection
detection.
