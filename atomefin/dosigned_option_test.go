package atomefin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Tests for the DoSignedOption / WithRequestHeader extension added for
// T3 (specifically the /auth sessionid header per DESIGN.md §1.3).

func TestWithRequestHeader_AppliedToOutgoing(t *testing.T) {
	var sawSession string
	var sawCustom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawSession = r.Header.Get("sessionid")
		sawCustom = r.Header.Get("X-Atome-Trace-Id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`),
		WithRequestHeader("sessionid", "session-token-xyz"),
		WithRequestHeader("X-Atome-Trace-Id", "trace-123"),
	)
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if sawSession != "session-token-xyz" {
		t.Errorf("sessionid header = %q", sawSession)
	}
	if sawCustom != "trace-123" {
		t.Errorf("X-Atome-Trace-Id = %q", sawCustom)
	}
}

func TestWithRequestHeader_CannotOverrideReserved(t *testing.T) {
	var sawAuth string
	var sawCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`),
		WithRequestHeader("Authorization", "tampered-sig"),
		WithRequestHeader("Content-Type", "text/evil"),
	)
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if sawAuth == "tampered-sig" {
		t.Error("partner overrode Authorization header — must be reserved")
	}
	if sawCT == "text/evil" {
		t.Error("partner overrode Content-Type — must be reserved")
	}
	if sawCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", sawCT)
	}
}

func TestWithRequestHeader_NilOptionTolerated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	// Passing a nil DoSignedOption shouldn't panic.
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`), nil)
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
}
