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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- Refund handler — happy path ----------

func TestRefundHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"requestId":"r-1","captureRequestId":"CAP-1","refundId":"RFD-1","currency":"IDR","refundAmount":1500000,"status":"SUCCESS"}`)

	var seen *refund.RefundResult
	handler := callback.RefundHandler(h.verifier, func(ctx context.Context, e *callback.RefundEvent) error {
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
	if seen.RefundID != "RFD-1" {
		t.Errorf("event RefundID = %q", seen.RefundID)
	}
}

// ---------- 401 paths ----------

func TestRefundHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := h.signedBody(t, body)

	tampered := []byte(`{"code":"FAILED"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/refund", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.RefundHandler(h.verifier, func(ctx context.Context, e *callback.RefundEvent) error {
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

// ---------- Multi-cert end-to-end ----------

func TestRefundHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
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

	body := []byte(`{"requestId":"r-1","captureRequestId":"CAP-1","refundId":"RFD-1","currency":"IDR","refundAmount":1,"status":"SUCCESS"}`)
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/refund", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.RefundHandler(v, func(ctx context.Context, e *callback.RefundEvent) error {
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

func TestRefundHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"requestId":"r-dup","captureRequestId":"CAP-1","refundId":"RFD-1","currency":"IDR","refundAmount":1,"status":"SUCCESS"}`)

	var calls int32
	handler := callback.RefundHandler(h.verifier, func(ctx context.Context, e *callback.RefundEvent) error {
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

func TestRefundHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"requestId":"r-1","captureRequestId":"CAP-1","refundId":"RFD-1","currency":"IDR","refundAmount":1,"status":"SUCCESS"}`)

	rec := httptest.NewRecorder()
	callback.RefundHandler(h.verifier, func(ctx context.Context, e *callback.RefundEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so atome-fin retries)", rec.Code)
	}
}

// ---------- Defensive: nil verifier / nil userFn ----------

func TestRefundHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/refund", bytes.NewReader([]byte(`{}`)))
	callback.RefundHandler(nil, func(ctx context.Context, e *callback.RefundEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500 with config error", rec.Code)
	}
}

func TestRefundHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/refund", bytes.NewReader([]byte(`{}`)))
	callback.RefundHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500 with config error", rec.Code)
	}
}

// ---------- Fixture decode ----------

func TestRefundHandler_DecodesFixture(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_refund_terminal_success.json")
	rec := httptest.NewRecorder()
	callback.RefundHandler(h.verifier, func(ctx context.Context, e *callback.RefundEvent) error {
		return nil
	}).ServeHTTP(rec, h.post(body))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
