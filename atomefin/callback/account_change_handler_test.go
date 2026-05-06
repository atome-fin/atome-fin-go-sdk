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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- AccountChange handler — happy path ----------

func TestAccountChangeHandler_HappyPath(t *testing.T) {
	h := newHarness(t)

	body := []byte(`{"code":"SUCCESS","message":"account state changed","data":{"eventId":"ACE-1","externalReferenceUid":"user-42","eventTime":1746748800000,"accountChanges":{"version":1746748800000,"externalReferenceUid":"user-42","previousStatus":"NORMAL","currentStatus":"NORMAL","totalCreditChange":5000000,"usedCreditChange":0,"frozenCreditChange":0,"availableCreditChange":5000000,"overpaidAmountChange":0,"lateFeeAmountChange":0,"interestAmountChange":0}}}`)

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
	if seen.Data == nil || seen.Data.EventID != "ACE-1" {
		t.Errorf("event Data = %#v", seen.Data)
	}
	if seen.Data.AccountChanges == nil {
		t.Fatal("AccountChanges nil — credit-change vector should round-trip")
	}
	if seen.Data.AccountChanges.AvailableCreditChange != 5000000 {
		t.Errorf("AvailableCreditChange = %d", seen.Data.AccountChanges.AvailableCreditChange)
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

	body := []byte(`{"code":"SUCCESS","message":"account state changed","data":{"eventId":"ACE-1","externalReferenceUid":"user-42","eventTime":1,"accountChanges":{"version":1,"externalReferenceUid":"user-42","previousStatus":"NORMAL","currentStatus":"NORMAL","totalCreditChange":0,"usedCreditChange":0,"frozenCreditChange":0,"availableCreditChange":0,"overpaidAmountChange":0,"lateFeeAmountChange":0,"interestAmountChange":0}}}`)
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
	body := []byte(`{"code":"SUCCESS","message":"x","data":{"eventId":"ACE-DUP","externalReferenceUid":"u","eventTime":1,"accountChanges":{"version":1,"externalReferenceUid":"u","previousStatus":"NORMAL","currentStatus":"NORMAL","totalCreditChange":0,"usedCreditChange":0,"frozenCreditChange":0,"availableCreditChange":0,"overpaidAmountChange":0,"lateFeeAmountChange":0,"interestAmountChange":0}}}`)

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
	body := []byte(`{"code":"SUCCESS","message":"x","data":{"eventId":"ACE-1","externalReferenceUid":"u","eventTime":1,"accountChanges":{"version":1,"externalReferenceUid":"u","previousStatus":"NORMAL","currentStatus":"NORMAL","totalCreditChange":0,"usedCreditChange":0,"frozenCreditChange":0,"availableCreditChange":0,"overpaidAmountChange":0,"lateFeeAmountChange":0,"interestAmountChange":0}}}`)

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
	if seen == nil || seen.Data == nil {
		t.Fatal("event data nil")
	}
	if seen.Data.AccountChanges.TotalCreditChange != 5000000 {
		t.Errorf("TotalCreditChange = %d", seen.Data.AccountChanges.TotalCreditChange)
	}
	if seen.Data.ExtendInfo == nil || seen.Data.ExtendInfo.Reason != "credit-limit-increase" {
		t.Errorf("ExtendInfo = %#v", seen.Data.ExtendInfo)
	}
}

// Q24 position-scoped enum: currentStatus = ACCOUNT_CLOSED is valid;
// previousStatus = ACCOUNT_CLOSED would be invalid (caught by
// payment.IsValidPreviousStatus). The status-close fixture pairs
// previousStatus=ACCOUNT_BLOCKED_OVERDUE with currentStatus=
// ACCOUNT_CLOSED, which is the canonical close transition.
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
	if seen == nil || seen.Data == nil || seen.Data.AccountChanges == nil {
		t.Fatal("event data nil")
	}
	ac := seen.Data.AccountChanges

	// Sanity: this is the spec-canonical close transition.
	if ac.PreviousStatus != atomefin.AccountStatusBlockedOverdue {
		t.Errorf("previousStatus = %q, want ACCOUNT_BLOCKED_OVERDUE", ac.PreviousStatus)
	}
	if ac.CurrentStatus != atomefin.AccountStatusClosed {
		t.Errorf("currentStatus = %q, want ACCOUNT_CLOSED", ac.CurrentStatus)
	}

	// Q24 position-scoped rule still holds:
	if !payment.IsValidPreviousStatus(ac.PreviousStatus) {
		t.Error("previousStatus must validate for the close transition")
	}
	if !payment.IsValidCurrentStatus(ac.CurrentStatus) {
		t.Error("currentStatus must validate (ACCOUNT_CLOSED is allowed only on currentStatus)")
	}
	// Negative-pin: swapping ACCOUNT_CLOSED into the previousStatus
	// slot would fail validation.
	if payment.IsValidPreviousStatus(atomefin.AccountStatusClosed) {
		t.Error("Q24 invariant broken: ACCOUNT_CLOSED must NOT be valid for previousStatus")
	}
}

// ---------- AccountChangeEvent.IsSuccess helper ----------

func TestAccountChangeEvent_IsSuccess(t *testing.T) {
	if (&callback.AccountChangeEvent{Code: atomefin.CodeSuccess}).IsSuccess() != true {
		t.Error("SUCCESS envelope must report IsSuccess = true")
	}
	if (&callback.AccountChangeEvent{Code: atomefin.CodeServerError}).IsSuccess() != false {
		t.Error("SERVER_ERROR envelope must report IsSuccess = false")
	}
	var nilEvent *callback.AccountChangeEvent
	if nilEvent.IsSuccess() != false {
		t.Error("nil receiver must report IsSuccess = false")
	}
}
