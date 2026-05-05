package transport

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNopLoggerSilent(t *testing.T) {
	var l Logger = NopLogger{}
	l.Debug("d")
	l.Info("i", "k", 1)
	l.Warn("w")
	l.Error("e")
}

func TestNopObserverSilent(t *testing.T) {
	var o Observer = NopObserver{}
	o.OnRequest(context.Background(), "/auth", 1)
	o.OnResponse(context.Background(), "/auth", 200, time.Millisecond)
	o.OnRetry(context.Background(), "/auth", 1, nil)
}

func TestRedactHeaders(t *testing.T) {
	in := http.Header{
		"Authorization": []string{"super-secret"},
		"Content-Type":  []string{"application/json"},
		"Cookie":        []string{"sid=abc"},
	}
	out := RedactHeaders(in)

	if got := out.Get("Authorization"); got != RedactedAuthorization {
		t.Errorf("Authorization redacted = %q, want %q", got, RedactedAuthorization)
	}
	if got := out.Get("Cookie"); got != RedactedAuthorization {
		t.Errorf("Cookie redacted = %q, want %q", got, RedactedAuthorization)
	}
	if got := out.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want pass-through", got)
	}

	// Source header must not be mutated.
	if in.Get("Authorization") != "super-secret" {
		t.Error("RedactHeaders mutated the source map")
	}
	// Empty input is fine.
	if got := RedactHeaders(nil); got == nil {
		t.Error("RedactHeaders(nil) returned nil; want empty header")
	}
}
