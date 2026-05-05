package atomefin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Tests for the architect's batteries-review must-fix items:
//   1. Observer panic recovery
//   2. Response body size cap (default 1 MiB, OOM defence)
//   5. Cloned http.DefaultTransport with tuned defaults
//   7. WithBaseURL hygiene (scheme + host)
//   D2. Client.Close() no-op stub

// ---------- (1) Observer panic recovery ----------

type panickingObserver struct {
	whichRequest, whichResponse, whichRetry    int32
	requestPanics, responsePanics, retryPanics int32
}

func (p *panickingObserver) OnRequest(_ context.Context, _ string, _ int) {
	if atomic.AddInt32(&p.whichRequest, 1) == 1 {
		atomic.AddInt32(&p.requestPanics, 1)
		panic("observer.OnRequest exploded")
	}
}
func (p *panickingObserver) OnResponse(_ context.Context, _ string, _ int, _ time.Duration) {
	atomic.AddInt32(&p.responsePanics, 1)
	panic("observer.OnResponse exploded")
}
func (p *panickingObserver) OnRetry(_ context.Context, _ string, _ int, _ error) {
	atomic.AddInt32(&p.retryPanics, 1)
	panic("observer.OnRetry exploded")
}

func TestObserverPanicsAreContained(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS"}`))
	}))
	defer srv.Close()

	obs := &panickingObserver{}
	c := newTestClient(t, srv, WithObserver(obs))

	resp, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err != nil {
		t.Fatalf("DoSigned: %v (panic in observer should NOT propagate)", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if atomic.LoadInt32(&obs.requestPanics) == 0 {
		t.Error("expected OnRequest to have been invoked (and panicked)")
	}
	if atomic.LoadInt32(&obs.responsePanics) == 0 {
		t.Error("expected OnResponse to have been invoked (and panicked)")
	}
}

func TestObserverPanicOnRetryContained(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS"}`))
	}))
	defer srv.Close()

	obs := &panickingObserver{}
	c := newTestClient(t, srv, WithObserver(obs))

	if _, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`)); err != nil {
		t.Fatalf("DoSigned: %v (panic in OnRetry should NOT propagate)", err)
	}
	if atomic.LoadInt32(&obs.retryPanics) == 0 {
		t.Error("expected OnRetry to have been invoked")
	}
}

// ---------- (2) Response body size cap ----------

func TestResponseBodyCapDefault1MiB(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	if c.maxRespBytes != 1<<20 {
		t.Errorf("default maxRespBytes = %d, want 1 MiB", c.maxRespBytes)
	}
}

func TestResponseBodyCapEnforced(t *testing.T) {
	// Server replies with 2× the cap → SDK must reject with TransportError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(make([]byte, 200))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, WithMaxResponseBytes(64))
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err == nil {
		t.Fatal("expected TransportError when body exceeds cap")
	}
	if !strings.Contains(err.Error(), "exceeds max") {
		t.Errorf("err = %v; want 'exceeds max'", err)
	}
}

// ---------- (5) Cloned default transport ----------

func TestDefaultHTTPClientUsesClonedTransport(t *testing.T) {
	c := newDefaultHTTPClient()
	if c.Transport == http.DefaultTransport {
		t.Fatal("default *http.Client.Transport is the global DefaultTransport (must be cloned)")
	}
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.MaxIdleConnsPerHost != 32 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 32", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}

	// Mutating our cloned transport must not touch the global one.
	if base := http.DefaultTransport.(*http.Transport); base.MaxIdleConnsPerHost == 32 {
		// Could conceivably default to 32 in some Go version; just compare
		// pointer identity above is the canonical assertion.
		t.Logf("note: DefaultTransport.MaxIdleConnsPerHost happens to also be 32; relying on pointer-identity check")
		_ = base
	}
}

// ---------- (7) WithBaseURL hygiene ----------

func TestWithBaseURLRejectsBadScheme(t *testing.T) {
	cases := []string{
		"ftp://example.com",
		"file:///etc/passwd",
		"://example.com",
		"https://",       // no host
		"https:/missing", // malformed
	}
	for _, in := range cases {
		cfg := defaultConfig()
		err := WithBaseURL(in)(cfg)
		if err == nil {
			t.Errorf("WithBaseURL(%q) should fail validation", in)
		}
	}
}

func TestWithBaseURLAcceptsHTTPAndHTTPS(t *testing.T) {
	for _, in := range []string{
		"http://localhost:8080",
		"https://api.example.com",
		"https://api.example.com/",
		"https://api.example.com/v1/path",
	} {
		cfg := defaultConfig()
		if err := WithBaseURL(in)(cfg); err != nil {
			t.Errorf("WithBaseURL(%q) failed: %v", in, err)
		}
		if strings.HasSuffix(cfg.baseURL, "/") {
			t.Errorf("WithBaseURL(%q) left trailing slash: %q", in, cfg.baseURL)
		}
	}
}

// ---------- (D2) Client.Close() ----------

func TestClientCloseIsNoOp(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	if err := c.Close(); err != nil {
		t.Errorf("first Close() = %v, want nil", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil (idempotent)", err)
	}
}
