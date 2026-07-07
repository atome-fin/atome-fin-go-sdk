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

// Sample callback body — re-used across happy-path / replay / 500
// tests to keep noise low. Mirrors callback_credit_application_terminal_success.json.
func creditApplicationCallbackBody() []byte {
	return []byte(`{"externalReferenceUid":"user-42","status":"SUCCESS","currency":"IDR","creditInfo":{"totalCredit":30000000,"availableCredit":30000000,"usedCredit":0,"userStatus":"NORMAL","version":1715000000000},"loanCreditInfo":{"loanStatus":"NORMAL"},"billDay":1,"payDay":25}`)
}

// ---------- Credit-application handler — happy path ----------

func TestCreditApplicationHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := creditApplicationCallbackBody()

	var seen *credit.CreditApplicationResult
	handler := callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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
	if seen.Status != credit.CreditStatusSuccess {
		t.Errorf("event Status = %#v", seen.Status)
	}
	if seen.CreditInfo == nil || seen.CreditInfo.TotalCredit != 30000000 {
		t.Errorf("event CreditInfo = %#v", seen.CreditInfo)
	}
}

// ---------- 401 paths ----------

func TestCreditApplicationHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := creditApplicationCallbackBody()
	sig, _ := h.signedBody(t, body)

	tampered := []byte(`{"externalReferenceUid":"u","status":"FAILED","currency":"IDR"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

func TestCreditApplicationHandler_RejectsMissingSignature(t *testing.T) {
	h := newHarness(t)
	body := creditApplicationCallbackBody()
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader(body))
	// no Authorization header
	rec := httptest.NewRecorder()

	handler := callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

// ---------- Multi-cert end-to-end ----------

func TestCreditApplicationHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
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

	body := creditApplicationCallbackBody()
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.CreditApplicationHandler(v, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

func TestCreditApplicationHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := creditApplicationCallbackBody()

	var calls int32
	handler := callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

func TestCreditApplicationHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := creditApplicationCallbackBody()

	rec := httptest.NewRecorder()
	callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so atome-fin retries)", rec.Code)
	}
}

// ---------- Bad JSON ----------

func TestCreditApplicationHandler_RejectsBadJSON(t *testing.T) {
	h := newHarness(t)
	body := []byte(`not json`)
	r := h.post(body) // signed; sig OK; JSON malformed
	rec := httptest.NewRecorder()
	called := false
	callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

func TestCreditApplicationHandler_RejectsOversizeBody(t *testing.T) {
	priv := mustKey(t)
	v, err := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)},
		callback.WithBodyLimit(64),
	)
	if err != nil {
		t.Fatal(err)
	}

	bigBody := bytes.Repeat([]byte("a"), 200)
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader(bigBody))
	r.Header.Set("Authorization", "AAAA==") // unreachable — body gate fires first

	rec := httptest.NewRecorder()
	called := false
	callback.CreditApplicationHandler(v, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
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

func TestCreditApplicationHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader([]byte(`{}`)))
	callback.CreditApplicationHandler(nil, func(ctx context.Context, e *callback.CreditApplicationEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500 with config error", rec.Code)
	}
}

func TestCreditApplicationHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/credit-application", bytes.NewReader([]byte(`{}`)))
	callback.CreditApplicationHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500 with config error", rec.Code)
	}
}

// ---------- Fixture decode ----------

func TestCreditApplicationHandler_DecodesFixture(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_credit_application_terminal_success.json")
	rec := httptest.NewRecorder()
	callback.CreditApplicationHandler(h.verifier, func(ctx context.Context, e *callback.CreditApplicationEvent) error {
		return nil
	}).ServeHTTP(rec, h.post(body))
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
