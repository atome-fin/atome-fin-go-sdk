package atomefin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- Test helpers ----------

func mustGenKey(t testing.TB) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	return key
}

func mustPEM(t testing.TB, key *rsa.PrivateKey) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func newTestClient(t testing.TB, srv *httptest.Server, extra ...Option) *Client {
	t.Helper()
	key := mustGenKey(t)
	opts := []Option{
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithBaseURL(srv.URL),
		WithPartnerID("partner-test"),
		WithRetry(transport.RetryPolicy{
			MaxAttempts:           3,
			Base:                  1 * time.Millisecond,
			Cap:                   5 * time.Millisecond,
			Jitter:                0,
			RetryOnStatus:         transport.DefaultRetryOnStatus,
			RetryOnTransportError: transport.DefaultRetryOnTransportError,
		}),
	}
	opts = append(opts, extra...)
	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// ---------- New(...) validation ----------

func TestNewRequiresSigner(t *testing.T) {
	_, err := New(WithBaseURL("http://x"), WithPartnerID("p"))
	if err == nil {
		t.Fatal("expected error when no signer is configured")
	}
	if !strings.Contains(err.Error(), "Signer") {
		t.Errorf("err = %v; expected mention of Signer", err)
	}
}

// Q7 RESOLVED (2026-05-05): partner identity is the dedicated API URL +
// RSA cert exchange, not a header. WithPartnerID stays supported as a
// log-enrichment hook only — Client construction succeeds without it,
// and the SDK never emits an X-Partner-Id / X-Merchant-Id header on
// outbound traffic.
func TestNewWithoutPartnerIDIsAllowed(t *testing.T) {
	var sawPartner string
	var sawMerchant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPartner = r.Header.Get("X-Partner-Id")
		sawMerchant = r.Header.Get("X-Merchant-Id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	key := mustGenKey(t)
	// Construct WITHOUT any WithPartnerID / WithMerchantID — must succeed.
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithBaseURL(srv.URL),
	)
	if err != nil {
		t.Fatalf("Client construction without WithPartnerID must succeed; got %v", err)
	}
	if c.PartnerID() != "" {
		t.Errorf("PartnerID() = %q, want empty", c.PartnerID())
	}
	if c.MerchantID() != "" {
		t.Errorf("MerchantID() = %q, want empty", c.MerchantID())
	}

	// Outbound request must NOT carry the provisional partner-identity
	// headers — even when WithPartnerID IS set later, since Q7 says the
	// SDK never emits them.
	if _, dErr := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`)); dErr != nil {
		t.Fatalf("DoSigned: %v", dErr)
	}
	if sawPartner != "" {
		t.Errorf("X-Partner-Id leaked: %q (Q7 RESOLVED — must NOT be emitted)", sawPartner)
	}
	if sawMerchant != "" {
		t.Errorf("X-Merchant-Id leaked: %q (Q7 RESOLVED — must NOT be emitted)", sawMerchant)
	}

	// Re-confirm with WithPartnerID set: the option populates the
	// log-enrichment accessor but still emits no header.
	c2, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithBaseURL(srv.URL),
		WithPartnerID("partner-foo"),
		WithMerchantID("merchant-bar"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c2.PartnerID() != "partner-foo" {
		t.Errorf("PartnerID accessor lost the value: %q", c2.PartnerID())
	}
	if c2.MerchantID() != "merchant-bar" {
		t.Errorf("MerchantID accessor lost the value: %q", c2.MerchantID())
	}
	sawPartner, sawMerchant = "", ""
	if _, err := c2.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`)); err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if sawPartner != "" || sawMerchant != "" {
		t.Errorf("partner-identity headers leaked even with WithPartnerID set: partner=%q merchant=%q",
			sawPartner, sawMerchant)
	}
}

func TestNewDefaultsBaseURLFromEnvironment(t *testing.T) {
	key := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithEnvironment(EnvTest),
		WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := BaseURL(EnvTest)
	if c.BaseURL() != want {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL(), want)
	}
}

func TestNewBaseURLOverridesEnvironment(t *testing.T) {
	key := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithEnvironment(EnvProd),
		WithBaseURL("https://my-gateway.example.com"),
		WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseURL() != "https://my-gateway.example.com" {
		t.Errorf("BaseURL not overridden: %q", c.BaseURL())
	}
}

func TestNewMutuallyExclusiveSigners(t *testing.T) {
	key := mustGenKey(t)
	signer, _ := sign.NewRSA2Signer(key)
	_, err := New(
		WithSigner(signer),
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithPartnerID("p"),
		WithEnvironment(EnvTest),
	)
	if err == nil {
		t.Fatal("expected error when both WithSigner and WithPrivateKeyPEM are passed")
	}
}

// ---------- DoSigned: happy path ----------

func TestDoSignedHappyPath(t *testing.T) {
	const wantBody = `{"hello":"world"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("server got %s, want POST", r.Method)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("server got empty Authorization")
		}
		// Q7 RESOLVED: SDK must not emit X-Partner-Id / X-Merchant-Id.
		if got := r.Header.Get("X-Partner-Id"); got != "" {
			t.Errorf("X-Partner-Id leaked: %q (Q7 RESOLVED)", got)
		}
		if got := r.Header.Get("X-Merchant-Id"); got != "" {
			t.Errorf("X-Merchant-Id leaked: %q (Q7 RESOLVED)", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != wantBody {
			t.Errorf("body = %q, want %q", string(body), wantBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(wantBody))
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(resp.Body), "SUCCESS") {
		t.Errorf("body = %q", resp.Body)
	}
}

// ---------- DoSigned: signing actually happens ----------

func TestDoSignedSignatureVerifiable(t *testing.T) {
	key := mustGenKey(t)
	verifier, err := sign.NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	const wantBody = `{"requestId":"r-1"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization")
		}
		// Server canonical for POST is the raw body bytes (DESIGN.md §4).
		if vErr := verifier.Verify(r.Context(), body, auth); vErr != nil {
			t.Errorf("signature did not verify: %v", vErr)
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
	if _, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(wantBody)); err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
}

func mustSigner(t testing.TB, key *rsa.PrivateKey) sign.Signer {
	t.Helper()
	s, err := sign.NewRSA2Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// ---------- DoSigned: 4xx APIError ----------

func TestDoSigned4xxReturnsAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_MISSING","message":"requestId required","data":{"requestId":"r-1"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err == nil {
		t.Fatal("expected APIError")
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != CodeParamsMissing {
		t.Errorf("Code = %q", ae.Code)
	}
	if ae.HTTPStatus != 400 {
		t.Errorf("HTTPStatus = %d", ae.HTTPStatus)
	}
	if ae.RequestID != "r-1" {
		t.Errorf("RequestID = %q", ae.RequestID)
	}
}

// ---------- DoSigned: 5xx retries then surfaces APIError ----------

func TestDoSigned5xxRetriesThenFails(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"code":"SERVER_ERROR","message":"oops"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("server hits = %d, want 3 (default MaxAttempts)", got)
	}
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want APIError", err)
	}
	if !ae.Temporary() {
		t.Error("500 APIError should report Temporary() == true")
	}
}

// ---------- DoSigned: 5xx then 200 — succeeds, no error ----------

func TestDoSigned5xxThen200Succeeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	resp, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2", got)
	}
}

// ---------- DoSigned: 4xx never retries ----------

func TestDoSigned4xxNeverRetries(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"code":"INVALID_SIGNATURE","message":"bad"}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err == nil {
		t.Fatal("expected APIError")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("hits = %d, want 1 (no retry on 4xx)", got)
	}
	var ae *APIError
	if !errors.As(err, &ae) || !ae.IsSignature() {
		t.Errorf("expected 401 INVALID_SIGNATURE, got %v", err)
	}
}

// ---------- DoSigned: same body / signature across retries ----------

func TestDoSignedRetriesUseSameBody(t *testing.T) {
	var bodies []string
	var auths []string
	var mu = newGuard()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.lock()
		bodies = append(bodies, string(body))
		auths = append(auths, r.Header.Get("Authorization"))
		mu.unlock()
		w.WriteHeader(500)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	const sent = `{"requestId":"r-1","amount":1000}`
	_, _ = c.DoSigned(context.Background(), "POST", "/auth", []byte(sent))

	if len(bodies) < 2 {
		t.Fatalf("want >= 2 attempts, got %d", len(bodies))
	}
	for i := range bodies {
		if bodies[i] != sent {
			t.Errorf("attempt %d body = %q, want %q", i, bodies[i], sent)
		}
	}
	// PKCS#1-v1.5 is deterministic, so identical body → identical signature.
	for i := 1; i < len(auths); i++ {
		if auths[i] != auths[0] {
			t.Errorf("attempt %d signature differs from attempt 0 (deterministic PKCS#1-v1.5 expected)", i)
		}
	}
}

type guard struct{ ch chan struct{} }

func newGuard() *guard   { return &guard{ch: make(chan struct{}, 1)} }
func (g *guard) lock()   { g.ch <- struct{}{} }
func (g *guard) unlock() { <-g.ch }

// ---------- DoSigned: validation ----------

func TestDoSignedRejectsNonPOST(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := c.DoSigned(context.Background(), "GET", "/auth", nil)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
}

func TestDoSignedRejectsBadPath(t *testing.T) {
	c := newTestClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := c.DoSigned(context.Background(), "POST", "auth", nil)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
}

// ---------- DoSigned: context cancellation aborts retries ----------

func TestDoSignedCtxCancelStopsRetries(t *testing.T) {
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
	_, err := c.DoSigned(ctx, "POST", "/auth", []byte(`{}`))
	dur := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	// Should bail roughly when the ctx expires, well before MaxAttempts*Base.
	if dur > 400*time.Millisecond {
		t.Errorf("DoSigned ran for %v; should have aborted on ctx deadline", dur)
	}
}

// ---------- Authorization scheme override ----------

func TestAuthorizationSchemeOverride(t *testing.T) {
	var sawHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawHeader = r.Header.Get("Authorization")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv,
		WithKeyID("k1"),
		WithAuthorizationScheme(SchemeAtomeKeyed),
	)
	_, err := c.DoSigned(context.Background(), "POST", "/auth", []byte(`{}`))
	if err != nil {
		t.Fatalf("DoSigned: %v", err)
	}
	if !strings.HasPrefix(sawHeader, "Algorithm=RSA2") {
		t.Errorf("Authorization = %q; want SchemeAtomeKeyed prefix", sawHeader)
	}
}

// ---------- Atome verifier wiring ----------

func TestWithAtomePublicKey(t *testing.T) {
	atomeKey := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, mustGenKey(t))),
		WithBaseURL("http://x"),
		WithPartnerID("p"),
		WithAtomePublicKey(&atomeKey.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.Verifier() == nil {
		t.Error("Verifier() = nil after WithAtomePublicKey")
	}
}

// ---------- NewRequestID is wired into the client ----------

func TestNewRequestIDOverride(t *testing.T) {
	key := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithBaseURL("http://x"),
		WithPartnerID("p"),
		WithRequestIDGenerator(func() string { return "fixed-id" }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.NewRequestID(); got != "fixed-id" {
		t.Errorf("NewRequestID = %q, want fixed-id", got)
	}
}
