package atomefin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- R13 — wire query == signing canonical ----------

// TestDoSignedGET_R13_WireEqualsCanonical pins the GET-path invariant
// added in v0.2: the bytes the SDK signs (sign.CanonicalQuery output)
// MUST equal the bytes that travel on the wire (req.URL.RawQuery).
// The server reconstructs the canonical from r.URL.Query() and runs
// the verifier — if the bytes drift (e.g., url.Values.Encode()'s "+"
// for space leaks in), this fails loudly.
func TestDoSignedGET_R13_WireEqualsCanonical(t *testing.T) {
	key := mustGenKey(t)
	verifier, err := sign.NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	// Multi-key query that exercises sorting + percent-encoding (space,
	// ampersand, plus). The expected canonical is keys sorted alphabetically:
	//   externalReferenceUid=user-42&note=Foo%20%26%20Co&periodType=3&requestId=01HABC1234567890ABCDEFGHJK
	want := url.Values{
		"requestId":            []string{"01HABC1234567890ABCDEFGHJK"},
		"externalReferenceUid": []string{"user-42"},
		"note":                 []string{"Foo & Co"}, // contains space + ampersand
		"periodType":           []string{"3"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Server-side: rebuild canonical from r.URL.Query() and verify.
		// This is the round-trip property the spec demands — partner
		// signs canonical, server parses + re-canonicalises, signature
		// verifies against the rebuilt canonical.
		gotCanonical := []byte(sign.CanonicalQuery(r.URL.Query()))
		auth := r.Header.Get("Authorization")
		if vErr := verifier.Verify(r.Context(), gotCanonical, auth); vErr != nil {
			// Diagnostic: dump the wire query and the rebuilt canonical
			// so a regression points at the byte that drifted.
			t.Errorf("R13: server-side verify failed.\n"+
				"raw wire query:    %s\nrebuilt canonical: %s\nerr: %v",
				r.URL.RawQuery, gotCanonical, vErr)
		}
		// Stronger pin: the WIRE query bytes should equal the canonical
		// bytes verbatim. If we ever switch to url.Values.Encode() the
		// raw query would have "+" instead of "%20" and the rebuild-
		// then-verify above might still pass (Go's parser handles both),
		// but the wire-≡-canonical contract would be broken. This catches
		// that drift.
		if r.URL.RawQuery != string(gotCanonical) {
			t.Errorf("R13: wire query != canonical bytes.\n"+
				"wire:      %s\ncanonical: %s",
				r.URL.RawQuery, gotCanonical)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(
		WithSigner(mustSigner(t, key)),
		WithBaseURL(srv.URL),
		WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.DoSignedGET(context.Background(), "/query-auth", want); err != nil {
		t.Fatalf("DoSignedGET: %v", err)
	}
}

// ---------- 4xx becomes APIError, 5xx retries ----------

func TestDoSignedGET_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"unknown requestId"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSignedGET(context.Background(), "/query-auth", url.Values{"requestId": []string{"r-1"}})
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != CodeNotFound {
		t.Errorf("Code = %q, want NOT_FOUND", ae.Code)
	}
	if ae.HTTPStatus != 400 {
		t.Errorf("HTTPStatus = %d", ae.HTTPStatus)
	}
}

func TestDoSignedGET_5xxRetriesThenFails(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"code":"SERVER_ERROR","message":"oops"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSignedGET(context.Background(), "/query-auth", url.Values{"requestId": []string{"r-1"}})
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (default MaxAttempts)", got)
	}
}

// ---------- Retries reuse the same canonical bytes ----------

// Idempotency parity with DoSigned: every retry hits the server with
// the SAME RawQuery (and therefore the SAME signing canonical, and —
// for deterministic PKCS#1 v1.5 — the SAME Authorization header).
func TestDoSignedGET_RetriesUseSameCanonical(t *testing.T) {
	var queries []string
	var auths []string
	mu := newGuard()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.lock()
		queries = append(queries, r.URL.RawQuery)
		auths = append(auths, r.Header.Get("Authorization"))
		mu.unlock()
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, _ = c.DoSignedGET(context.Background(), "/query-auth",
		url.Values{"requestId": []string{"r-1"}, "extra": []string{"v"}})

	if len(queries) < 2 {
		t.Fatalf("want >= 2 attempts, got %d", len(queries))
	}
	for i := 1; i < len(queries); i++ {
		if queries[i] != queries[0] {
			t.Errorf("attempt %d wire query differs from attempt 0", i)
		}
		if auths[i] != auths[0] {
			t.Errorf("attempt %d Authorization differs from attempt 0 (deterministic PKCS#1-v1.5 expected)", i)
		}
	}
}

// ---------- ctx cancellation aborts retries DURING sleep ----------

func TestDoSignedGET_CtxCancelStopsRetries(t *testing.T) {
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
	_, err := c.DoSignedGET(ctx, "/query-auth", url.Values{"requestId": []string{"r-1"}})
	dur := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if dur > 400*time.Millisecond {
		t.Errorf("DoSignedGET ran for %v; ctx-cancel-during-sleep should have aborted earlier", dur)
	}
}

// ---------- Reserved-header allowlist (mirrors DoSigned) ----------

func TestDoSignedGET_ReservedHeadersCannotBeOverridden(t *testing.T) {
	var sawAuth, sawCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		sawCT = r.Header.Get("Content-Type")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSignedGET(context.Background(), "/query-auth",
		url.Values{"requestId": []string{"r-1"}},
		WithRequestHeader("Authorization", "tampered-sig"),
		WithRequestHeader("Content-Type", "text/evil"),
	)
	if err != nil {
		t.Fatalf("DoSignedGET: %v", err)
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

// ---------- Partner can inject custom headers ----------

func TestDoSignedGET_CustomHeaderPassesThrough(t *testing.T) {
	var sawTrace string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawTrace = r.Header.Get("X-Atome-Trace-Id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSignedGET(context.Background(), "/query-auth",
		url.Values{"requestId": []string{"r-1"}},
		WithRequestHeader("X-Atome-Trace-Id", "trace-123"),
	)
	if err != nil {
		t.Fatalf("DoSignedGET: %v", err)
	}
	if sawTrace != "trace-123" {
		t.Errorf("X-Atome-Trace-Id = %q, want trace-123", sawTrace)
	}
}

// ---------- Validation rejections ----------

func TestDoSignedGET_RejectsBadPath(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := c.DoSignedGET(context.Background(), "query-auth", nil)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
}

func TestDoSignedGET_NilClient(t *testing.T) {
	var c *Client
	_, err := c.DoSignedGET(context.Background(), "/query-auth", nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("err = %v; want nil-client error", err)
	}
}

// ---------- Empty query is permitted (canonical = "") ----------

func TestDoSignedGET_EmptyQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected empty RawQuery, got %q", r.URL.RawQuery)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	if _, err := c.DoSignedGET(context.Background(), "/health", nil); err != nil {
		t.Fatalf("DoSignedGET(nil query): %v", err)
	}
}
