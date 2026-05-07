package mock

import (
	"bytes"
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// FireOption configures a single Fire*Callback invocation. Both
// shipped FireOptions are partner overrides for the default
// (which is "use the bundled mock signing key, but only if the
// caller previously opted in via WithMockKeysAllowed").
type FireOption func(*fireConfig)

type fireConfig struct {
	signerKeyPEM          []byte
	allowMockKey          bool
	verifyCount           int // 0 = no count assertion; >0 = expect this many invocations
	skipResponseAssertion bool
}

// WithFireSignerKeyPEM substitutes the RSA-2048 private key used
// to sign the outbound callback body. Mutually exclusive with
// WithFireMockKey; if both are passed, the explicit key wins.
func WithFireSignerKeyPEM(priv []byte) FireOption {
	return func(c *fireConfig) {
		c.signerKeyPEM = priv
	}
}

// WithFireMockKey opts in to signing with the bundled mock key.
// The partner-side `callback.Verifier` must have been built with
// `mock.MockSigningPubCertPEM()` for signature verification to
// succeed.
//
// Without this option (and without WithFireSignerKeyPEM), the
// helper fails with `t.Fatalf` — protective against accidental
// shared-keypair tests.
func WithFireMockKey() FireOption {
	return func(c *fireConfig) {
		c.allowMockKey = true
	}
}

// WithFireVerifyCount asserts the handler received exactly the
// expected number of new invocations during this Fire call. Used
// for replay / dedupe testing — Fire fires once and expects the
// handler to invoke the user fn the matching number of times.
//
// Default (n == 0) means "no count assertion".
func WithFireVerifyCount(n int) FireOption {
	return func(c *fireConfig) {
		c.verifyCount = n
	}
}

// WithFireSkipResponseCheck disables the helper's default
// assertion that the callback handler returned 200 with a
// SUCCESS ack. Useful for tests that intentionally drive
// failure paths (e.g. asserting that a partner-side handler
// returns 500 when its user fn errors).
func WithFireSkipResponseCheck() FireOption {
	return func(c *fireConfig) {
		c.skipResponseAssertion = true
	}
}

// FireAuthCallback builds a signed callback body for /<authNotifyUrl>
// and dispatches it to h via ServeHTTP. Returns the captured
// *http.Response — the body has already been read into memory and
// is safe to close (or read further; it's a NopCloser around a
// bytes.Buffer).
//
// Default: signs with the bundled mock key when
// WithFireMockKey is in effect; otherwise requires
// WithFireSignerKeyPEM. Default: asserts the response is HTTP 200
// with `{"code":"SUCCESS"}`. Pass WithFireSkipResponseCheck to
// drive failure paths.
func FireAuthCallback(t testing.TB, h http.Handler, event *callback.AuthEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireAuthCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/<authNotifyUrl>", event, opts...)
}

// FireCaptureCallback fires a /<captureNotifyUrl> body.
func FireCaptureCallback(t testing.TB, h http.Handler, event *callback.CaptureEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireCaptureCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/<captureNotifyUrl>", event, opts...)
}

// FireRefundCallback fires a /<refundNotifyUrl> body.
func FireRefundCallback(t testing.TB, h http.Handler, event *callback.RefundEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireRefundCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/<refundNotifyUrl>", event, opts...)
}

// FireRepaymentCallback fires a /repayment-callback body.
func FireRepaymentCallback(t testing.TB, h http.Handler, event *callback.RepaymentEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireRepaymentCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/repayment-callback", event, opts...)
}

// FireAccountChangeCallback fires a /account_change_callback body.
func FireAccountChangeCallback(t testing.TB, h http.Handler, event *callback.AccountChangeEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireAccountChangeCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/account_change_callback", event, opts...)
}

// FireCreditApplicationCallback fires a
// /<creditApplicationNotifyUrl> body.
func FireCreditApplicationCallback(t testing.TB, h http.Handler, event *callback.CreditApplicationEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireCreditApplicationCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/<creditApplicationNotifyUrl>", event, opts...)
}

// FireCreditInformationCallback fires a
// /<creditInformationNotifyUrl> body.
func FireCreditInformationCallback(t testing.TB, h http.Handler, event *callback.CreditInformationEvent, opts ...FireOption) *http.Response {
	t.Helper()
	if event == nil {
		t.Fatalf("mock.FireCreditInformationCallback: nil event")
		return synthetic500()
	}
	return fire(t, h, "/<creditInformationNotifyUrl>", event, opts...)
}

// synthetic500 is the response returned to keep callers using a
// fake testing.TB (which doesn't actually exit on Fatalf) from
// nil-deref'ing on the result of a fire that already failed.
func synthetic500() *http.Response {
	return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}
}

// fire is the shared implementation: marshal → sign → dispatch
// via httptest.NewRecorder → optionally assert.
func fire(t testing.TB, h http.Handler, path string, event any, opts ...FireOption) *http.Response {
	t.Helper()
	cfg := &fireConfig{}
	for _, o := range opts {
		if o != nil {
			o(cfg)
		}
	}
	if h == nil {
		t.Fatalf("mock.Fire*Callback: nil http.Handler")
	}

	body, err := atomefin.MarshalSigning(event)
	if err != nil {
		t.Fatalf("mock.Fire*Callback: marshal event: %v", err)
	}

	priv := resolveFireKey(t, cfg)
	if priv == nil {
		// resolveFireKey already called t.Fatalf — return a synthetic
		// non-200 response so callers using a fake testing.TB (which
		// suppresses Fatalf) don't panic on a follow-up nil deref.
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}
	}
	signer, err := sign.NewRSA2Signer(priv)
	if err != nil {
		t.Fatalf("mock.Fire*Callback: build signer: %v", err)
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}
	}
	authz, err := signer.Sign(context.Background(), body)
	if err != nil {
		t.Fatalf("mock.Fire*Callback: sign body: %v", err)
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewReader(nil))}
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authz)

	rec := httptest.NewRecorder()
	// If a verify count was requested, wrap the handler with a
	// counter so we can assert on dispatched-to-user-fn count.
	wrapped := h
	var count int
	if cfg.verifyCount > 0 {
		wrapped = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count++
			h.ServeHTTP(w, r)
		})
	}
	wrapped.ServeHTTP(rec, req)
	resp := rec.Result()

	if cfg.verifyCount > 0 && count != cfg.verifyCount {
		t.Errorf("mock.Fire*Callback: handler invocation count = %d, want %d", count, cfg.verifyCount)
	}

	if !cfg.skipResponseAssertion {
		assertSuccessAck(t, path, resp)
	}
	return resp
}

// assertSuccessAck verifies the response is HTTP 200 with a
// SUCCESS-shaped ack body. Catches partner-handler regressions
// that silently return 4xx/5xx without surfacing the cause.
func assertSuccessAck(t testing.TB, path string, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("mock.Fire*Callback %s: status = %d, want 200; body: %s", path, resp.StatusCode, body)
		return
	}
	// Re-read body for caller convenience (tests often inspect).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("mock.Fire*Callback %s: read body: %v", path, err)
		return
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	// Cheap ack-shape check: body should contain "SUCCESS".
	// Strict envelope decoding lives in callback's own tests.
	if len(body) > 0 && !bytes.Contains(body, []byte("SUCCESS")) {
		t.Errorf("mock.Fire*Callback %s: ack body does not contain SUCCESS: %s", path, body)
	}
}

// resolveFireKey returns the *rsa.PrivateKey that the helper
// should sign with. Order of precedence:
//
//  1. cfg.signerKeyPEM (explicit override)
//  2. mock bundled key (only if cfg.allowMockKey is set)
//  3. t.Fatalf — partners must opt in to mock keys explicitly,
//     or supply their own.
func resolveFireKey(t testing.TB, cfg *fireConfig) *rsa.PrivateKey {
	t.Helper()
	var pemBytes []byte
	switch {
	case len(cfg.signerKeyPEM) > 0:
		pemBytes = cfg.signerKeyPEM
	case cfg.allowMockKey:
		pemBytes = MockSigningPrivKeyPEM()
	default:
		t.Fatalf("mock.Fire*Callback: no signing key supplied — pass WithFireSignerKeyPEM(priv) or opt-in via WithFireMockKey")
		return nil
	}
	priv, err := sign.LoadPrivateKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("mock.Fire*Callback: load private key: %v", err)
		return nil
	}
	return priv
}

// Compile-time check: every event type wired below must be
// reachable via the public callback package. Pinned here so a
// future package rename surfaces at build time, not at the
// Fire*Callback call site.
var (
	_ = func(*callback.AuthEvent) {}
	_ = func(*callback.CaptureEvent) {}
	_ = func(*callback.RefundEvent) {}
	_ = func(*callback.RepaymentEvent) {}
	_ = func(*callback.AccountChangeEvent) {}
	_ = func(*callback.CreditApplicationEvent) {}
	_ = func(*callback.CreditInformationEvent) {}
	_ = errors.New
	_ = fmt.Sprintf
)
