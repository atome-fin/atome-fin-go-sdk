package transport

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestNewSlogLoggerNilUsesDefault(t *testing.T) {
	l := NewSlogLogger(nil)
	if l == nil {
		t.Fatal("NewSlogLogger(nil) returned nil")
	}
	// Smoke: must not panic.
	l.Debug("hello")
}

func TestSlogAdapterRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	sl := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	l := NewSlogLogger(sl)

	l.Info("outbound",
		"op", "/auth",
		"Authorization", "secret-token",
		"sessionid", "session-secret",
		"shipping_name", "Foo Customer",
		"externalReferenceUid", "uid-123",
		"safe_field", "ok",
	)

	out := buf.String()

	// Sensitive keys must have RedactedAuthorization as their value.
	for _, k := range []string{"Authorization", "sessionid", "shipping_name", "externalReferenceUid"} {
		// slog renders kv as `key=value` in text handler; the value should
		// be the redaction marker, never the original.
		switch k {
		case "Authorization":
			if strings.Contains(out, "secret-token") {
				t.Errorf("Authorization value leaked: %s", out)
			}
		case "sessionid":
			if strings.Contains(out, "session-secret") {
				t.Errorf("sessionid value leaked: %s", out)
			}
		case "shipping_name":
			if strings.Contains(out, "Foo Customer") {
				t.Errorf("shipping_name value leaked: %s", out)
			}
		case "externalReferenceUid":
			if strings.Contains(out, "uid-123") {
				t.Errorf("externalReferenceUid value leaked: %s", out)
			}
		}
	}

	if !strings.Contains(out, RedactedAuthorization) {
		t.Errorf("expected redaction marker %q in output, got %q", RedactedAuthorization, out)
	}
	if !strings.Contains(out, "safe_field=ok") {
		t.Errorf("non-sensitive value should pass through; out=%q", out)
	}
	if !strings.Contains(out, "op=/auth") {
		t.Errorf("op pass-through missing; out=%q", out)
	}
}

func TestSlogAdapterTolerantOfMalformedKVs(t *testing.T) {
	var buf bytes.Buffer
	l := NewSlogLogger(slog.New(slog.NewTextHandler(&buf, nil)))

	// Odd-length kv slice (dangling key with no value).
	l.Info("odd", "lone")
	// Non-string keys.
	l.Info("bad-key", 42, "value")

	if buf.Len() == 0 {
		t.Error("adapter dropped log lines on malformed input")
	}
}

func TestRedactKVsCaseInsensitive(t *testing.T) {
	got := redactKVs([]any{"AUTHORIZATION", "secret", "Op", "/auth"})
	if got[1] != RedactedAuthorization {
		t.Errorf("uppercase key was not redacted: %v", got)
	}
}
