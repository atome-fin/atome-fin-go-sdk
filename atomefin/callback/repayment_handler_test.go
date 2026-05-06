package callback_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- Repayment handler — happy path ----------

func TestRepaymentHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS","message":"repayment terminal","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1500000,"event":"NORMAL"}}`)

	var seen *repayment.RepaymentResponse
	handler := callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
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
	if seen.Data == nil || seen.Data.RepaymentID != "RPM-1" {
		t.Errorf("event Data.RepaymentID = %#v", seen.Data)
	}
}

// ---------- 401 paths ----------

func TestRepaymentHandler_RejectsMissingSignature(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader(body))
	// no Authorization header
	rec := httptest.NewRecorder()

	handler := callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

func TestRepaymentHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := h.signedBody(t, body)

	tampered := []byte(`{"code":"FAILED"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

func TestRepaymentHandler_RejectsOversizeBody(t *testing.T) {
	priv := mustKey(t)
	v, err := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)},
		callback.WithBodyLimit(64),
	)
	if err != nil {
		t.Fatal(err)
	}

	bigBody := bytes.Repeat([]byte("a"), 200)
	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader(bigBody))
	r.Header.Set("Authorization", "AAAA==") // never reached

	rec := httptest.NewRecorder()
	called := false
	callback.RepaymentHandler(v, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

func TestRepaymentHandler_RejectsBadJSON(t *testing.T) {
	h := newHarness(t)
	body := []byte(`not json`)
	r := h.post(body) // signed; sig OK; JSON malformed
	rec := httptest.NewRecorder()
	called := false
	callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

// ---------- Multi-cert end-to-end ----------

func TestRepaymentHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
	oldKey := mustKey(t)
	newKey := mustKey(t)
	signer := mustSigner(t, oldKey) // atome-fin still signs with old key during overlap

	v, err := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &oldKey.PublicKey),
		mustVerifierFromKey(t, &newKey.PublicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"code":"SUCCESS","message":"repayment terminal","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1,"event":"NORMAL"}}`)
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.RepaymentHandler(v, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

// ---------- Replay: handler invoked once per delivery ----------

func TestRepaymentHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS","message":"repayment terminal","data":{"requestId":"r-dup","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1,"event":"ATOME_REPAYMENT"}}`)

	var calls int32
	handler := callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
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

// ---------- 500 path ----------

func TestRepaymentHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS","message":"repayment terminal","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1,"event":"NORMAL"}}`)

	rec := httptest.NewRecorder()
	callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so atome-fin retries)", rec.Code)
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

func TestRepaymentHandler_405OnGet(t *testing.T) {
	h := newHarness(t)
	r := httptest.NewRequest(http.MethodGet, "/atome/repayment", nil)

	rec := httptest.NewRecorder()
	callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
		t.Error("user handler must NOT be called on GET")
		return nil
	}).ServeHTTP(rec, r)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// ---------- Defensive: nil verifier / nil userFn ----------

func TestRepaymentHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader([]byte(`{}`)))
	callback.RepaymentHandler(nil, func(ctx context.Context, e *callback.RepaymentEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500 with config error", rec.Code)
	}
}

func TestRepaymentHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/repayment", bytes.NewReader([]byte(`{}`)))
	callback.RepaymentHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500 with config error", rec.Code)
	}
}

// ---------- Fixture decode ----------

func TestRepaymentHandler_DecodesFixture(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_repayment_terminal_success.json")
	rec := httptest.NewRecorder()
	callback.RepaymentHandler(h.verifier, func(ctx context.Context, e *callback.RepaymentEvent) error {
		return nil
	}).ServeHTTP(rec, h.post(body))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
