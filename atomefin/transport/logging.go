package transport

import (
	"context"
	"net/http"
	"time"
)

// Logger is the structured, key-value logger interface the SDK calls into.
//
// The signature is intentionally compatible with log/slog so partners can
// pass an *slog.Logger directly via a thin adapter (one method per level,
// each forwarding `slog.Logger.Log(ctx, level, msg, args...)`). The default
// is a no-op logger; partners adapt their own (see DESIGN.md §10).
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// NopLogger discards every call. Default for Client when WithLogger is not
// set so the SDK is silent unless the partner opts in.
type NopLogger struct{}

func (NopLogger) Debug(string, ...any) {}
func (NopLogger) Info(string, ...any)  {}
func (NopLogger) Warn(string, ...any)  {}
func (NopLogger) Error(string, ...any) {}

// Observer is the metrics / tracing hook surface. Implementations should
// return quickly — the SDK invokes them inline on the request path.
//
// op is the API operation (today: "/auth", "/capture", "/voidAuth"). attempt
// is 1-indexed: 1 for the initial request, 2/3 for retries.
type Observer interface {
	OnRequest(ctx context.Context, op string, attempt int)
	OnResponse(ctx context.Context, op string, status int, dur time.Duration)
	OnRetry(ctx context.Context, op string, attempt int, err error)
}

// NopObserver discards every call. Default for Client when WithObserver is
// not set.
type NopObserver struct{}

func (NopObserver) OnRequest(context.Context, string, int)                 {}
func (NopObserver) OnResponse(context.Context, string, int, time.Duration) {}
func (NopObserver) OnRetry(context.Context, string, int, error)            {}

// RedactedAuthorization is the placeholder string written into log lines in
// place of the Authorization header value. We always redact at this layer
// regardless of debug-body-logging being enabled (DESIGN.md §10).
const RedactedAuthorization = "[REDACTED]"

// RedactHeaders returns a shallow clone of h with sensitive headers
// scrubbed. The original h is not modified. Today we redact the
// Authorization header; the redaction list will grow once the partner
// confirms additional sensitive headers (Q5 timestamp/nonce, Q7 partner
// header, etc.).
func RedactHeaders(h http.Header) http.Header {
	if len(h) == 0 {
		return http.Header{}
	}
	out := make(http.Header, len(h))
	for k, v := range h {
		if isSensitiveHeader(k) {
			out[k] = []string{RedactedAuthorization}
			continue
		}
		// Defensive copy of the slice so callers can't mutate the original
		// through the redacted view.
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

func isSensitiveHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Authorization", "Cookie", "Set-Cookie", "Proxy-Authorization",
		// `sessionid` is the spec's /auth header (DESIGN.md §1.3, Q6 — TTL
		// open, but the value is an opaque session token regardless of what
		// the lifecycle answer is). After CanonicalHeaderKey: "Sessionid".
		"Sessionid":
		return true
	}
	return false
}
