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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- AccountChange handler — happy path ----------

func TestAccountChangeHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"callbackRequestId":"ACE-1","externalReferenceUid":"user-42","externalRequestId":"REQ-1","event":"FIXED_CREDIT_LIMIT_BOOST","scene":"CREDIT_LIMIT_ADJUSTMENT","previousStatus":"NORMAL","currentStatus":"NORMAL","currency":"IDR","amountChange":5000000,"version":1746748800000,"creditInfo":{"totalCredit":35000000,"availableCredit":35000000,"usedCredit":0,"userStatus":"NORMAL"}}`)

	var seen *callback.AccountChangeEvent
	handler := callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
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
	if seen.CallbackRequestID != "ACE-1" {
		t.Errorf("CallbackRequestID = %q", seen.CallbackRequestID)
	}
	if seen.CreditInfo == nil {
		t.Fatal("CreditInfo nil")
	}
	if seen.AmountChange != 5000000 {
		t.Errorf("AmountChange = %d", seen.AmountChange)
	}
}

// ---------- 401 paths ----------

func TestAccountChangeHandler_RejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := h.signedBody(t, body)

	tampered := []byte(`{"code":"FAILED"}`)
	r := httptest.NewRequest(http.MethodPost, "/atome/account-change", bytes.NewReader(tampered))
	r.Header.Set("Authorization", sig)

	called := false
	handler := callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
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

func TestAccountChangeHandler_MultiCert_OldKeyStillVerifies(t *testing.T) {
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

	body := []byte(`{"callbackRequestId":"ACE-1","externalReferenceUid":"user-42","event":"ATOME_CONTROL","scene":"ACCOUNT_STATUS_CHANGE","currency":"IDR","version":1,"creditInfo":{"totalCredit":1,"availableCredit":1,"usedCredit":0,"userStatus":"NORMAL"}}`)
	sig, _ := signer.Sign(context.Background(), body)

	r := httptest.NewRequest(http.MethodPost, "/atome/account-change", bytes.NewReader(body))
	r.Header.Set("Authorization", sig)

	rec := httptest.NewRecorder()
	called := false
	callback.AccountChangeHandler(v, func(ctx context.Context, e *callback.AccountChangeEvent) error {
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

func TestAccountChangeHandler_ReplayInvokesUserFnTwice(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"callbackRequestId":"ACE-DUP","externalReferenceUid":"u","event":"ATOME_CONTROL","scene":"ACCOUNT_STATUS_CHANGE","currency":"IDR","version":1,"creditInfo":{"totalCredit":1,"availableCredit":1,"usedCredit":0,"userStatus":"NORMAL"}}`)

	var calls int32
	handler := callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
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
		t.Errorf("user fn invoked %d times across 2 replays; partner is responsible for dedupe (R-doc — typical pattern: dedupe on EventID)", got)
	}
}

// ---------- 500 path ----------

func TestAccountChangeHandler_500OnUserError(t *testing.T) {
	h := newHarness(t)
	body := []byte(`{"callbackRequestId":"ACE-1","externalReferenceUid":"u","event":"ATOME_CONTROL","scene":"ACCOUNT_STATUS_CHANGE","currency":"IDR","version":1,"creditInfo":{"totalCredit":1,"availableCredit":1,"usedCredit":0,"userStatus":"NORMAL"}}`)

	rec := httptest.NewRecorder()
	callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
		return errors.New("downstream queue full")
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (so atome-fin retries)", rec.Code)
	}
}

// ---------- Defensive: nil verifier / nil userFn ----------

func TestAccountChangeHandler_NilVerifier(t *testing.T) {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/account-change", bytes.NewReader([]byte(`{}`)))
	callback.AccountChangeHandler(nil, func(ctx context.Context, e *callback.AccountChangeEvent) error { return nil }).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil verifier: status = %d, want 500 with config error", rec.Code)
	}
}

func TestAccountChangeHandler_NilUserFn(t *testing.T) {
	h := newHarness(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/atome/account-change", bytes.NewReader([]byte(`{}`)))
	callback.AccountChangeHandler(h.verifier, nil).ServeHTTP(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("nil user fn: status = %d, want 500 with config error", rec.Code)
	}
}

// ---------- Fixture decode ----------

func TestAccountChangeHandler_DecodesFixture_BalanceIncrease(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_account_change_balance_increase.json")

	var seen *callback.AccountChangeEvent
	rec := httptest.NewRecorder()
	callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
		seen = e
		return nil
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if seen == nil {
		t.Fatal("event data nil")
	}
	if seen.AmountChange != 5000000 {
		t.Errorf("AmountChange = %d", seen.AmountChange)
	}
	if seen.ExtendInfo == nil || seen.ExtendInfo.CreditLimitAdjustmentReason != "PERMANENT_CREDIT_INCREASE" {
		t.Errorf("ExtendInfo = %#v", seen.ExtendInfo)
	}
}

func TestAccountChangeHandler_DecodesFixture_StatusClose_Q24(t *testing.T) {
	h := newHarness(t)
	body := readFile(t, "../../qa/testdata/callback_account_change_status_close.json")

	var seen *callback.AccountChangeEvent
	rec := httptest.NewRecorder()
	callback.AccountChangeHandler(h.verifier, func(ctx context.Context, e *callback.AccountChangeEvent) error {
		seen = e
		return nil
	}).ServeHTTP(rec, h.post(body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if seen == nil {
		t.Fatal("event data nil")
	}
	// Sanity: this is the spec-canonical close transition.
	if seen.PreviousStatus != atomefin.AccountStatusBlockedOverdue {
		t.Errorf("previousStatus = %q, want ACCOUNT_BLOCKED_OVERDUE", seen.PreviousStatus)
	}
	if seen.CurrentStatus != atomefin.AccountStatusClosed {
		t.Errorf("currentStatus = %q, want ACCOUNT_CLOSED", seen.CurrentStatus)
	}
}

// ---------- AccountChangeEvent.IsSuccess helper ----------

func TestAccountChangeEvent_IsSuccess(t *testing.T) {
	if (&callback.AccountChangeEvent{CallbackRequestID: "cb-1"}).IsSuccess() != true {
		t.Error("non-empty callbackRequestId must report IsSuccess = true")
	}
	if (&callback.AccountChangeEvent{}).IsSuccess() != false {
		t.Error("empty event must report IsSuccess = false")
	}
	var nilEvent *callback.AccountChangeEvent
	if nilEvent.IsSuccess() != false {
		t.Error("nil receiver must report IsSuccess = false")
	}
}
