package transport

import (
	"log/slog"
	"strings"
)

// NewSlogLogger wraps an *slog.Logger so calls flow through the SDK's PII
// redactor before reaching slog handlers. Pass nil to use slog.Default().
//
// Why a constructor exists: *slog.Logger already satisfies the Logger
// interface byte-for-byte (the four method shapes line up), so a partner
// could write `atomefin.WithLogger(slog.Default())` and the SDK would
// never log a single redaction. Authorization headers, sessionid tokens,
// shipping-* PII would all surface raw to whichever handler the partner
// has wired up. Pass *slog.Logger through this constructor and the
// adapter walks each call's key/value pairs, redacting any value whose
// preceding key is on the §10 sensitive list.
//
// For v0.1 we only adapt slog (DESIGN team-lead decision D1). logrus /
// zap adapters live under atomefin/log/ in a future minor.
func NewSlogLogger(l *slog.Logger) Logger {
	if l == nil {
		l = slog.Default()
	}
	return slogAdapter{l: l}
}

type slogAdapter struct{ l *slog.Logger }

func (a slogAdapter) Debug(msg string, kv ...any) { a.l.Debug(msg, redactKVs(kv)...) }
func (a slogAdapter) Info(msg string, kv ...any)  { a.l.Info(msg, redactKVs(kv)...) }
func (a slogAdapter) Warn(msg string, kv ...any)  { a.l.Warn(msg, redactKVs(kv)...) }
func (a slogAdapter) Error(msg string, kv ...any) { a.l.Error(msg, redactKVs(kv)...) }

// redactKVs walks alternating (key, value) slog args and replaces the
// value with RedactedAuthorization when the key matches the sensitive
// list. Defensive against malformed slices: odd-length input passes the
// dangling element through, non-string keys leave the pair untouched.
//
// We intentionally do NOT recurse into struct values — partners that
// stuff a whole HTTP request into a single arg are responsible for
// applying RedactHeaders themselves.
func redactKVs(kv []any) []any {
	if len(kv) == 0 {
		return kv
	}
	out := make([]any, len(kv))
	copy(out, kv)
	for i := 0; i+1 < len(out); i += 2 {
		k, ok := out[i].(string)
		if !ok {
			continue
		}
		if isSensitiveLogKey(k) {
			out[i+1] = RedactedAuthorization
		}
	}
	return out
}

// isSensitiveLogKey returns true for log key names the SDK considers PII
// or auth material. Lowercased comparison so partners can write
// "Authorization", "authorization" or "AUTHORIZATION" interchangeably.
//
// The list mirrors the DESIGN.md §10 redaction policy with an eye toward
// keys the SDK *itself* emits in log lines. Partners that pass their own
// custom keys are responsible for naming them sensibly.
func isSensitiveLogKey(k string) bool {
	switch strings.ToLower(k) {
	case "authorization",
		"auth",
		"cookie",
		"set-cookie",
		"proxy-authorization",
		"sessionid",
		"session_id",
		"session-id",
		"shipping_name",
		"shippingname",
		"shipping_phone_no",
		"shippingphoneno",
		"external_reference_uid",
		"externalreferenceuid":
		return true
	}
	return false
}
