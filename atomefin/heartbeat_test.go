package atomefin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- HeartBeat happy path ----------

func TestHeartBeat_Success(t *testing.T) {
	var gotPath, gotMethod, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if err := c.HeartBeat(context.Background()); err != nil {
		t.Fatalf("HeartBeat: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/heart-beat" {
		t.Errorf("path = %q, want /heart-beat", gotPath)
	}
	// Empty query: sign.CanonicalQuery(nil) == "" → wire RawQuery
	// is also "". This is the boundary case the existing
	// TestDoSignedGET_EmptyQuery covers; HeartBeat should hit it
	// transparently.
	if gotQuery != "" {
		t.Errorf("RawQuery = %q, want empty", gotQuery)
	}
}

// ---------- 5xx surfaces APIError ----------

func TestHeartBeat_5xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"code":"SERVER_ERROR","message":"upstream offline"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	err := c.HeartBeat(context.Background())
	if err == nil {
		t.Fatal("expected error after 5xx retries")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.HTTPStatus != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus = %d, want 503", ae.HTTPStatus)
	}
	if !ae.Temporary() {
		t.Error("503 APIError should report Temporary() == true")
	}
}

// ---------- 4xx surfaces APIError without retry ----------

func TestHeartBeat_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"code":"INVALID_SIGNATURE","message":"x"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	err := c.HeartBeat(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if !ae.IsSignature() {
		t.Errorf("expected 401 INVALID_SIGNATURE, got %v", ae)
	}
}

// ---------- nil-Client guard ----------

func TestHeartBeat_NilClient(t *testing.T) {
	var c *Client
	err := c.HeartBeat(context.Background())
	if err == nil {
		t.Fatal("HeartBeat on nil *Client must return an error")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("err = %v; want mention of nil client", err)
	}
}

// ---------- ctx cancellation aborts retries ----------

func TestHeartBeat_CtxCancelStopsRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestClient(t, srv,
		WithRetry(transport.RetryPolicy{
			MaxAttempts:           5,
			Base:                  100 * time.Millisecond,
			Cap:                   100 * time.Millisecond,
			Jitter:                0,
			RetryOnStatus:         transport.DefaultRetryOnStatus,
			RetryOnTransportError: transport.DefaultRetryOnTransportError,
		}),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := c.HeartBeat(ctx)
	dur := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if dur > 400*time.Millisecond {
		t.Errorf("HeartBeat ran for %v; ctx-cancel-during-sleep should have aborted earlier", dur)
	}
}
