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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// Sample callback body — re-used across the credit-information
// handler tests. Mirrors callback_credit_information_terminal_success.json.
func creditInformationCallbackBody() []byte {
	return []byte(`{"code":"SUCCESS","message":"credit information collect terminal","data":{"requestId":"info-1","externalReferenceUid":"user-42","status":"SUCCESS"}}`)
}

// ---------- Credit-information handler — happy path ----------

func TestCreditInformationHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := creditInformationCallbackBody()

	var seen *credit.CreditInformationCollectResponse
	handler := callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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
	if seen.Data == nil || seen.Data.Status != credit.CreditStatusSuccess {
		t.Errorf("event Data.Status = %#v", seen.Data)
	}
	if seen.Data.RequestID != "info-1" {
		t.Errorf("event Data.RequestID = %q", seen.Data.RequestID)
	}
}

// ---------- 401 paths ----------

func TestCreditInformationHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := creditInformationCallbackBody()
	sig, _ := h.signedBody(t, body)

	tampered := []byte(`{"code":"SUCCESS","data":{"requestId":"info-2","externalReferenceUid":"user-99","status":"FAILED"}}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-information", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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

func TestCreditInformationHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
	oldKey := mustKey(t)
	newKey := mustKey(t)
	signer := mustSigner(t, oldKey)

	v, err := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &oldKey.PublicKey),
		mustVerifierFromKey(t, &newKey.PublicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := creditInformationCallbackBody()
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/credit-information", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.CreditInformationHandler(v, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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

// ---------- Replay ----------

func TestCreditInformationHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := creditInformationCallbackBody()

	var calls int32
	handler := callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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
		t.Errorf("user fn invoked %d times across 2 replays", got)
	}
}

// ---------- 500 path ----------

func TestCreditInformationHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := creditInformationCallbackBody()

	rec := httptest.NewRecorder()
	callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so atome-fin retries)", rec.Code)
	}
}

// ---------- Bad JSON ----------

func TestCreditInformationHandler_RejectsBadJSON(t *testing.T) {
	h := newHarness(t)
	body := []byte(`not json`)
	r := h.post(body)
	rec := httptest.NewRecorder()
	called := false
	callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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

// ---------- Oversize body ----------

func TestCreditInformationHandler_RejectsOversizeBody(t *testing.T) {
	priv := mustKey(t)
	v, err := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)},
		callback.WithBodyLimit(64),
	)
	if err != nil {
		t.Fatal(err)
	}

	bigBody := bytes.Repeat([]byte("a"), 200)
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-information", bytes.NewReader(bigBody))
	r.Header.Set("Authorization", "AAAA==")

	rec := httptest.NewRecorder()
	called := false
	callback.CreditInformationHandler(v, func(ctx context.Context, e *callback.CreditInformationEvent) error {
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

// ---------- Defensive: nil verifier / nil userFn ----------

func TestCreditInformationHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-information", bytes.NewReader([]byte(`{}`)))
	callback.CreditInformationHandler(nil, func(ctx context.Context, e *callback.CreditInformationEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500", rec.Code)
	}
}

func TestCreditInformationHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-information", bytes.NewReader([]byte(`{}`)))
	callback.CreditInformationHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500", rec.Code)
	}
}

// ---------- Fixture decode ----------

func TestCreditInformationHandler_DecodesFixture(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_credit_information_terminal_success.json")
	rec := httptest.NewRecorder()
	callback.CreditInformationHandler(h.verifier, func(ctx context.Context, e *callback.CreditInformationEvent) error {
		return nil
	}).ServeHTTP(rec, h.post(body))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
