package callback_test

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- Test scaffolding ----------

// callbackHarness ties an Atome-side signer + a partner-side verifier
// for a single callback handler. The signer represents Atome's private
// key signing the body; the verifier holds the Atome public key
// configured on the partner's *Verifier.
type callbackHarness struct {
	t        testing.TB
	priv     *rsa.PrivateKey
	signer   sign.Signer
	verifier *callback.Verifier
}

func newHarness(t testing.TB) *callbackHarness {
	t.Helper()
	priv := mustKey(t)
	v, err := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	return &callbackHarness{t: t, priv: priv, signer: mustSigner(t, priv), verifier: v}
}

// post builds an http.Request that simulates an Atome callback POST:
// signs `body` with the harness's signer and places the signature in
// the Authorization header (default scheme: raw base64).
func (h *callbackHarness) post(body []byte) *http.Request {
	h.t.Helper()
	sig, err := h.signer.Sign(context.Background(), body)
	if err != nil {
		h.t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)
	return r
}

// signedBody returns body + valid sig for a few tests that build their
// own http.Request (e.g. swapping the verb).
func (h *callbackHarness) signedBody(t testing.TB, body []byte) (string, []byte) {
	t.Helper()
	sig, err := h.signer.Sign(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	return sig, body
}

// ---------- Auth handler — happy path ----------

func TestAuthHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1500000,"status":"SUCCESS"}}`)

	var seen *payment.AuthResponse
	handler := callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		seen = e
		return nil
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q", got)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing X-Content-Type-Options nosniff")
	}
	var ack callback.AckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &ack); err != nil {
		t.Fatalf("ack body unmarshal: %v\nbody=%s", err, rec.Body.String())
	}
	if ack.Code != atomefin.CodeSuccess {
		t.Errorf("ack.Code = %q, want SUCCESS", ack.Code)
	}
	if seen == nil {
		t.Fatal("user handler was not invoked")
	}
	if seen.Data == nil || seen.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("event Data.AuthOrderID = %#v", seen.Data)
	}
}

// ---------- Capture handler — happy path ----------

func TestCaptureHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"c-1","orderId":"O-1","currency":"IDR","totalAmount":1500000,"status":"SUCCESS","authOrderId":"AUTH-1"}}`)

	var saw *payment.CaptureResponse
	handler := callback.CaptureHandler(h.verifier, func(ctx context.Context, e *callback.CaptureEvent) error {
		saw = e
		return nil
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if saw == nil || saw.Data == nil || saw.Data.OrderID != "O-1" {
		t.Errorf("capture event = %#v", saw)
	}
}

// ---------- 401 paths ----------

func TestAuthHandler_RejectsMissingSignature(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader(body))
	// no Authorization header
	rec := httptest.NewRecorder()

	handler := callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		t.Error("user handler must NOT be called when signature is missing")
		return nil
	})
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	var ack callback.AckResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ack)
	if ack.Code != atomefin.CodeInvalidSignature {
		t.Errorf("ack.Code = %q, want INVALID_SIGNATURE", ack.Code)
	}
}

func TestAuthHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := h.signedBody(t, body)

	// Send a tampered body but the signature for the original.
	tampered := []byte(`{"code":"FAILED"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		called = true
		return nil
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
	if called {
		t.Error("user handler invoked despite tampered body")
	}
}

// ---------- 400 paths ----------

func TestAuthHandler_RejectsOversizeBody(t *testing.T) {
	priv := mustKey(t)
	v, err := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)},
		callback.WithBodyLimit(64),
	)
	if err != nil {
		t.Fatal(err)
	}

	bigBody := bytes.Repeat([]byte("a"), 200)
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader(bigBody))
	r.Header.Set("Authorization", "AAAA==") // doesn't matter — never reached

	rec := httptest.NewRecorder()
	called := false
	callback.AuthHandler(v, func(ctx context.Context, e *callback.AuthEvent) error {
		called = true
		return nil
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if called {
		t.Error("user handler invoked on oversize body")
	}
}

func TestAuthHandler_RejectsBadJSON(t *testing.T) {
	h := newHarness(t)
	body := []byte(`not json`)
	r := h.post(body) // signed; sig OK; JSON malformed
	rec := httptest.NewRecorder()
	called := false
	callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		called = true
		return nil
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if called {
		t.Error("user handler invoked on undecodable body")
	}
}

// ---------- 500 path ----------

func TestAuthHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"SUCCESS"}}`)

	rec := httptest.NewRecorder()
	callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so Atome retries)", rec.Code)
	}
	var ack callback.AckResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &ack)
	if ack.Code != atomefin.CodeServerError {
		t.Errorf("ack.Code = %q, want SERVER_ERROR", ack.Code)
	}
	if !strings.Contains(ack.Message, "downstream queue full") {
		t.Errorf("ack.Message = %q; want user error reason embedded", ack.Message)
	}
}

// ---------- Method check ----------

func TestAuthHandler_405OnGet(t *testing.T) {
	h := newHarness(t)
	r := httptest.NewRequest(http.MethodGet, "/atome/auth", nil)

	rec := httptest.NewRecorder()
	callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		t.Error("user handler must NOT be called on GET")
		return nil
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// ---------- Idempotency: replays invoke the user fn each time ----------

// The package contract is that the handler invokes the user function
// exactly ONCE per HTTP call. Atome may deliver duplicates — dedupe is
// the partner's responsibility. This test asserts the contract by
// replaying the same signed body and counting invocations.
func TestAuthHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-dup","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"SUCCESS"}}`)

	var calls int32
	handler := callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, h.post(body))
		if rec.Code != http.StatusOK {
			t.Fatalf("replay %d: status = %d", i, rec.Code)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("user fn invoked %d times across 2 replays; partner is responsible for dedupe (R-doc)", got)
	}
}

// ---------- Multi-cert end-to-end ----------

func TestAuthHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
	oldKey := mustKey(t)
	newKey := mustKey(t)
	// Atome still signs with old key (simulating a callback in flight
	// during cert rotation overlap).
	signer := mustSigner(t, oldKey)

	v, err := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &oldKey.PublicKey),
		mustVerifierFromKey(t, &newKey.PublicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"SUCCESS"}}`)
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.AuthHandler(v, func(ctx context.Context, e *callback.AuthEvent) error {
		called = true
		return nil
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 during rotation overlap", rec.Code)
	}
	if !called {
		t.Error("user handler not invoked despite valid signature on old key")
	}
}

// ---------- Defensive: nil verifier / nil fn ----------

func TestAuthHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader([]byte(`{}`)))
	callback.AuthHandler(nil, func(ctx context.Context, e *callback.AuthEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500 with config error", rec.Code)
	}
}

func TestAuthHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/auth", bytes.NewReader([]byte(`{}`)))
	callback.AuthHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500 with config error", rec.Code)
	}
}

// ---------- Fixtures: callback bodies decode through our types ----------

func TestAuthHandler_DecodesFixtures(t *testing.T) {
	h := newHarness(t)
	for _, fixture := range []string{
		"../../qa/testdata/callback_auth_terminal_success.json",
		"../../qa/testdata/callback_auth_terminal_failed.json",
	} {
		fixture := fixture
		t.Run(fixture, func(t *testing.T) {
			body := readFile(t, fixture)
			rec := httptest.NewRecorder()
			callback.AuthHandler(h.verifier, func(ctx context.Context, e *callback.AuthEvent) error {
				return nil
			}).ServeHTTP(rec, h.post(body))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCaptureHandler_DecodesFixture(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_capture_terminal_success.json")
	rec := httptest.NewRecorder()
	callback.CaptureHandler(h.verifier, func(ctx context.Context, e *callback.CaptureEvent) error {
		return nil
	}).ServeHTTP(rec, h.post(body))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// ---------- Helpers ----------

func readFile(t testing.TB, path string) []byte {
	t.Helper()
	r, err := openFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	// Trim the trailing newline that text-editor-saved fixtures
	// typically have so the signature matches the on-the-wire bytes.
	return bytes.TrimSpace(body)
}
