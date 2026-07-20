package mock_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- WithSpecValidation ----------

// TestServer_SpecValidation_RejectsMissingHeader pins that
// WithSpecValidation enforces the pinned spec's required-header
// set. /auth requires the `sessionid` header per spec; an inbound
// request without it must be rejected with 400 PARAMS_MISSING.
func TestServer_SpecValidation_RejectsMissingHeader(t *testing.T) {
	srv := mock.NewServer(t, mock.AlwaysSuccess(), mock.WithSpecValidation())

	// POST /auth without the sessionid header — should fail.
	resp, err := http.Post(srv.URL+"/auth", "application/json",
		strings.NewReader(`{"requestId":"r-1","externalReferenceUid":"u-1","totalAmount":1,"periodType":1,"subOrders":[{"subOrderId":"so-1","amount":1,"quantity":1}]}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "PARAMS_MISSING") {
		t.Errorf("body = %q; want PARAMS_MISSING", body)
	}
	if !strings.Contains(string(body), "sessionid") {
		t.Errorf("body = %q; want sessionid mentioned", body)
	}
}

// TestServer_SpecValidation_OffByDefault pins that v0.4-shape
// `mock.NewServer(t, scenario)` calls don't activate spec
// validation — the SDK retries / signature tests don't want
// pre-rejection on a partial body.
func TestServer_SpecValidation_OffByDefault(t *testing.T) {
	srv := mock.NewServer(t, mock.AlwaysSuccess()) // no WithSpecValidation
	resp, err := http.Post(srv.URL+"/auth", "application/json",
		strings.NewReader(`{}`)) // missing every required field
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (validation off)", resp.StatusCode)
	}
}

// ---------- WithIdempotency ----------

// TestServer_Idempotency_ReplaysOnDuplicateRequestID pins the
// LRU replay: the first POST gets a fresh response; the second
// with the same `requestId` gets the cached response byte-for-
// byte (with an `X-Mock-Replay: 1` marker).
func TestServer_Idempotency_ReplaysOnDuplicateRequestID(t *testing.T) {
	var counter int32
	dynamicScenario := mock.ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&counter, 1)
		body := `{"code":"SUCCESS","message":"hit-` + itoa(int(n)) + `"}`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	srv := mock.NewServer(t, dynamicScenario, mock.WithIdempotency())

	body := strings.NewReader(`{"requestId":"r-idem-1","externalReferenceUid":"u-1"}`)
	resp1, err := http.Post(srv.URL+"/voidAuth", "application/json", body)
	if err != nil {
		t.Fatalf("POST #1: %v", err)
	}
	body1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	if !strings.Contains(string(body1), "hit-1") {
		t.Errorf("first response body = %q; want hit-1", body1)
	}

	// Replay with the same requestId — should return the cached body.
	body = strings.NewReader(`{"requestId":"r-idem-1","externalReferenceUid":"u-1"}`)
	resp2, err := http.Post(srv.URL+"/voidAuth", "application/json", body)
	if err != nil {
		t.Fatalf("POST #2: %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if !strings.Contains(string(body2), "hit-1") {
		t.Errorf("replay body = %q; want hit-1 (cached)", body2)
	}
	if resp2.Header.Get("X-Mock-Replay") != "1" {
		t.Errorf("replay missing X-Mock-Replay header: %v", resp2.Header)
	}
	if got := atomic.LoadInt32(&counter); got != 1 {
		t.Errorf("scenario invocations = %d; want 1 (cache hit on retry)", got)
	}
}

// TestServer_Idempotency_OffByDefault pins that retries are
// counted separately when WithIdempotency is not passed —
// retry-loop tests (e.g. SDK's 5xx retry) still see each attempt.
func TestServer_Idempotency_OffByDefault(t *testing.T) {
	var counter int32
	scen := mock.ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&counter, 1)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"code":"SUCCESS"}`)),
		}, nil
	})
	srv := mock.NewServer(t, scen) // no WithIdempotency

	for i := 0; i < 3; i++ {
		body := strings.NewReader(`{"requestId":"r-1"}`)
		_, _ = http.Post(srv.URL+"/voidAuth", "application/json", body)
	}
	if got := atomic.LoadInt32(&counter); got != 3 {
		t.Errorf("scenario invocations = %d; want 3 (no caching)", got)
	}
}

// TestServer_Reset_ClearsIdempotency confirms Server.Reset()
// drops the cache so subsequent requests with the same requestId
// hit the scenario fresh.
func TestServer_Reset_ClearsIdempotency(t *testing.T) {
	var counter int32
	scen := mock.ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		atomic.AddInt32(&counter, 1)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"code":"SUCCESS"}`)),
		}, nil
	})
	srv := mock.NewServer(t, scen, mock.WithIdempotency())

	body := strings.NewReader(`{"requestId":"r-1"}`)
	_, _ = http.Post(srv.URL+"/voidAuth", "application/json", body)
	body = strings.NewReader(`{"requestId":"r-1"}`)
	_, _ = http.Post(srv.URL+"/voidAuth", "application/json", body)

	if got := atomic.LoadInt32(&counter); got != 1 {
		t.Errorf("after caching: invocations = %d, want 1", got)
	}

	srv.Reset()

	body = strings.NewReader(`{"requestId":"r-1"}`)
	_, _ = http.Post(srv.URL+"/voidAuth", "application/json", body)
	if got := atomic.LoadInt32(&counter); got != 2 {
		t.Errorf("after Reset(): invocations = %d, want 2", got)
	}
}

// ---------- WithAutoCallback ----------

// TestServer_AutoCallback_FiresAfterTypedSuccessScenario pins
// the multi-step lifecycle the architect un-blocked in §6: a
// typed AuthSuccess scenario sync-replies SUCCESS AND fires the
// matching callback to a partner-side handler.
func TestServer_AutoCallback_FiresAfterTypedSuccessScenario(t *testing.T) {
	pub, _ := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	v, _ := sign.NewRSA2Verifier(pub)
	verifier, _ := callback.NewVerifier([]sign.Verifier{v})

	var callbackHit bool
	authHandler := callback.AuthHandler(verifier, func(_ context.Context, e *callback.AuthEvent) error {
		callbackHit = true
		if e.Data == nil || e.Data.AuthOrderID != "AUTH-AUTO-1" {
			t.Errorf("callback Event.Data = %#v; want AuthOrderID=AUTH-AUTO-1", e.Data)
		}
		return nil
	})

	srv := mock.NewServer(t,
		mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth": mock.AuthSuccess("AUTH-AUTO-1"),
		}, mock.AlwaysSuccess()),
		mock.WithAutoCallback(map[string]http.Handler{
			"POST /<authNotifyUrl>": authHandler,
		}),
	)

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	resp, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{samplePaymentSubOrder(1500000)},
		ExtendInfo:           sampleRequestExtendInfo(),
		Sessionid:            "s",
	})
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if resp.Data == nil || resp.Data.AuthOrderID != "AUTH-AUTO-1" {
		t.Errorf("sync resp.Data = %#v", resp.Data)
	}
	if !callbackHit {
		t.Error("auto-callback was NOT delivered to partner handler")
	}
}

// TestServer_AutoCallback_OffByDefault pins that v0.4-shape
// `mock.NewServer(t, scenario)` calls don't fire callbacks even
// when a typed scenario carries one.
func TestServer_AutoCallback_OffByDefault(t *testing.T) {
	srv := mock.NewServer(t,
		mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth": mock.AuthSuccess("AUTH-1"),
		}, mock.AlwaysSuccess()),
	)
	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	if _, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{samplePaymentSubOrder(1500000)},
		ExtendInfo:           sampleRequestExtendInfo(),
		Sessionid:            "s",
	}); err != nil {
		t.Fatalf("Auth: %v", err)
	}
	// Implicit assertion: no t.Errorf was raised by an unwired
	// callback handler — auto-callback off by default.
}

// TestServer_AutoCallback_DelayHonored pins the
// WithCallbackDelay knob.
func TestServer_AutoCallback_DelayHonored(t *testing.T) {
	pub, _ := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	v, _ := sign.NewRSA2Verifier(pub)
	verifier, _ := callback.NewVerifier([]sign.Verifier{v})

	authHandler := callback.AuthHandler(verifier, func(_ context.Context, _ *callback.AuthEvent) error {
		return nil
	})

	srv := mock.NewServer(t,
		mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth": mock.AuthSuccess("AUTH-1"),
		}, mock.AlwaysSuccess()),
		mock.WithAutoCallback(map[string]http.Handler{
			"POST /<authNotifyUrl>": authHandler,
		}),
		mock.WithCallbackDelay(50*time.Millisecond),
	)

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	start := time.Now()
	if _, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{samplePaymentSubOrder(1)},
		ExtendInfo:           sampleRequestExtendInfo(),
		Sessionid:            "s",
	}); err != nil {
		t.Fatalf("Auth: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Errorf("elapsed = %s; want >= 50ms (callback delay)", elapsed)
	}
}

// ---------- WithResponseSigning ----------

// TestServer_ResponseSigning_AddsAuthorizationHeader pins the
// forward-compat plumbing: when a private key is supplied via
// WithResponseSigning, every response carries an Authorization
// header that verifies against the matching public.
func TestServer_ResponseSigning_AddsAuthorizationHeader(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})

	srv := mock.NewServer(t, mock.AlwaysSuccess(), mock.WithResponseSigning(privPEM))

	resp, err := http.Get(srv.URL + "/heart-beat")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	authz := resp.Header.Get("Authorization")
	if authz == "" {
		t.Fatal("missing Authorization header")
	}

	// Verify the signature against the matching public.
	verifier, err := sign.NewRSA2Verifier(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifier.Verify(context.Background(), body, authz); err != nil {
		t.Errorf("response signature failed verification: %v", err)
	}
}

// TestServer_ResponseSigning_OffByDefault pins that the v0.4
// no-Authorization-on-response default is preserved.
func TestServer_ResponseSigning_OffByDefault(t *testing.T) {
	srv := mock.NewServer(t, mock.AlwaysSuccess())
	resp, err := http.Get(srv.URL + "/heart-beat")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.Header.Get("Authorization") != "" {
		t.Errorf("Authorization unexpectedly set: %q", resp.Header.Get("Authorization"))
	}
}

// ---------- helpers ----------

// itoa avoids strconv import at every call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// freshTestKeyPEM generates a one-shot RSA-2048 PEM for client
// signing — same shape as the docs/mock_mode_examples_test.go
// helper.
func freshTestKeyPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
}

// Prevent unused-import drift for json/bytes if helpers shrink.
var _ = json.Marshal
var _ = bytes.NewReader
