package mock_test

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// mustVerifier builds a callback.Verifier from the bundled mock
// signing PUBLIC key — the matching half of MockSigningPrivKeyPEM
// that the Fire*Callback helpers sign with by default.
func mustVerifier(t *testing.T) *callback.Verifier {
	t.Helper()
	pub, err := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	if err != nil {
		t.Fatalf("LoadPublicCertPEM: %v", err)
	}
	v, err := sign.NewRSA2Verifier(pub)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	cb, err := callback.NewVerifier([]sign.Verifier{v})
	if err != nil {
		t.Fatalf("callback.NewVerifier: %v", err)
	}
	return cb
}

// ---------- FireAuthCallback ----------

func TestFireAuthCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var got *callback.AuthEvent
	h := callback.AuthHandler(v, func(_ context.Context, e *callback.AuthEvent) error {
		got = e
		return nil
	})

	mock.FireAuthCallback(t, h, &callback.AuthEvent{
		Code: "SUCCESS", Message: "ok",
		Data: &payment.AuthorizationData{
			RequestID:   "r-1",
			AuthOrderID: "AUTH-1",
			Currency:    "IDR",
			TotalAmount: 1500000,
			Status:      "SUCCESS",
		},
	}, mock.WithFireMockKey())

	if got == nil {
		t.Fatal("user fn was not invoked")
	}
	if got.Data == nil || got.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("decoded event = %#v", got)
	}
}

// ---------- FireCaptureCallback ----------

func TestFireCaptureCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.CaptureHandler(v, func(_ context.Context, e *callback.CaptureEvent) error {
		hit = true
		return nil
	})

	mock.FireCaptureCallback(t, h, &callback.CaptureEvent{
		Code: "SUCCESS", Message: "ok",
		Data: &payment.CaptureResultData{
			AuthOrderID: "AUTH-1",
		},
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- FireRefundCallback ----------

func TestFireRefundCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.RefundHandler(v, func(_ context.Context, e *callback.RefundEvent) error {
		hit = true
		return nil
	})

	mock.FireRefundCallback(t, h, &callback.RefundEvent{
		RequestID:        "r-1",
		CaptureRequestID: "c-1",
		RefundID:         "RFD-1",
		Currency:         "IDR",
		RefundAmount:     1000,
		Status:           "SUCCESS",
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- FireRepaymentCallback ----------

func TestFireRepaymentCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.RepaymentHandler(v, func(_ context.Context, e *callback.RepaymentEvent) error {
		hit = true
		return nil
	})

	mock.FireRepaymentCallback(t, h, &callback.RepaymentEvent{
		RequestID:       "r-1",
		RepaymentID:     "RPY-1",
		Currency:        "IDR",
		RepaymentAmount: 100000,
		Status:          "SUCCESS",
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- FireAccountChangeCallback ----------

func TestFireAccountChangeCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.AccountChangeHandler(v, func(_ context.Context, e *callback.AccountChangeEvent) error {
		hit = true
		return nil
	})

	mock.FireAccountChangeCallback(t, h, &callback.AccountChangeEvent{
		CallbackRequestID:    "ev-1",
		ExternalReferenceUID: "user-1",
		Event:                "ATOME_CONTROL",
		Scene:                "ACCOUNT_STATUS_CHANGE",
		Currency:             "IDR",
		CreditInfo: &callback.AccountChangeCreditInfo{
			TotalCredit: 1, AvailableCredit: 1, UsedCredit: 0, UserStatus: "NORMAL",
		},
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- FireCreditApplicationCallback ----------

func TestFireCreditApplicationCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.CreditApplicationHandler(v, func(_ context.Context, e *callback.CreditApplicationEvent) error {
		hit = true
		return nil
	})

	mock.FireCreditApplicationCallback(t, h, &callback.CreditApplicationEvent{
		ExternalReferenceUID: "user-1",
		Status:               credit.CreditStatus("SUCCESS"),
		Currency:             "IDR",
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- FireCreditInformationCallback ----------

func TestFireCreditInformationCallback_HappyPath(t *testing.T) {
	v := mustVerifier(t)
	var hit bool
	h := callback.CreditInformationHandler(v, func(_ context.Context, e *callback.CreditInformationEvent) error {
		hit = true
		return nil
	})

	mock.FireCreditInformationCallback(t, h, &callback.CreditInformationEvent{
		Code: "SUCCESS", Message: "ok",
		Data: &credit.CreditApplicationCollectQueryResult{
			RequestID:            "info-1",
			ExternalReferenceUID: "user-1",
			Status:               credit.CreditStatus("SUCCESS"),
		},
	}, mock.WithFireMockKey())

	if !hit {
		t.Error("user fn was not invoked")
	}
}

// ---------- Error / corner paths ----------

func TestFireAuthCallback_RequiresKeySource(t *testing.T) {
	// Without WithFireMockKey or WithFireSignerKeyPEM, the helper
	// should fail loud rather than silently use a default.
	ftb := newFakeTB()
	mock.FireAuthCallback(ftb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		&callback.AuthEvent{Code: "SUCCESS"},
	)
	if len(ftb.fatal) == 0 {
		t.Error("expected t.Fatalf when no key source supplied")
	}
}

func TestFireAuthCallback_RejectsTamperedSignature(t *testing.T) {
	v := mustVerifier(t)
	h := callback.AuthHandler(v, func(_ context.Context, e *callback.AuthEvent) error {
		t.Error("user fn must NOT be invoked on tampered signature")
		return nil
	})

	// Fire with the encrypt PRIVATE key (a fresh 2048-bit RSA key
	// not in the verifier's accepted set). The verifier should
	// reject; with WithFireSkipResponseCheck the helper returns
	// the rejection response instead of failing the test.
	resp := mock.FireAuthCallback(t, h,
		&callback.AuthEvent{Code: "SUCCESS"},
		mock.WithFireSignerKeyPEM(mock.MockEncryptPrivKeyPEM()),
		mock.WithFireSkipResponseCheck(),
	)
	if resp.StatusCode == http.StatusOK {
		t.Errorf("status = %d, want non-200 (signature should have been rejected)", resp.StatusCode)
	}
}

// TestFireAuthCallback_VerifyCount pins the
// WithFireVerifyCount option: when set, the helper asserts that
// the dispatched-to handler ran the user fn exactly the expected
// number of times.
func TestFireAuthCallback_VerifyCount(t *testing.T) {
	v := mustVerifier(t)
	var calls int32
	h := callback.AuthHandler(v, func(_ context.Context, e *callback.AuthEvent) error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	mock.FireAuthCallback(t, h,
		&callback.AuthEvent{Code: "SUCCESS"},
		mock.WithFireMockKey(),
		mock.WithFireVerifyCount(1),
	)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("user fn calls = %d, want 1", got)
	}
}

// TestFireAuthCallback_SkipResponseCheck lets partners drive
// failure paths (e.g. 500 when the user fn errors) without the
// helper itself failing the test.
func TestFireAuthCallback_SkipResponseCheck(t *testing.T) {
	v := mustVerifier(t)
	h := callback.AuthHandler(v, func(_ context.Context, e *callback.AuthEvent) error {
		return errors.New("boom")
	})

	resp := mock.FireAuthCallback(t, h,
		&callback.AuthEvent{Code: "SUCCESS"},
		mock.WithFireMockKey(),
		mock.WithFireSkipResponseCheck(),
	)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
}

func TestFire_RejectsNilEvent(t *testing.T) {
	cases := []struct {
		name string
		fn   func(testing.TB, http.Handler)
	}{
		{"FireAuthCallback", func(tb testing.TB, h http.Handler) { mock.FireAuthCallback(tb, h, nil) }},
		{"FireCaptureCallback", func(tb testing.TB, h http.Handler) { mock.FireCaptureCallback(tb, h, nil) }},
		{"FireRefundCallback", func(tb testing.TB, h http.Handler) { mock.FireRefundCallback(tb, h, nil) }},
		{"FireRepaymentCallback", func(tb testing.TB, h http.Handler) { mock.FireRepaymentCallback(tb, h, nil) }},
		{"FireAccountChangeCallback", func(tb testing.TB, h http.Handler) { mock.FireAccountChangeCallback(tb, h, nil) }},
		{"FireCreditApplicationCallback", func(tb testing.TB, h http.Handler) { mock.FireCreditApplicationCallback(tb, h, nil) }},
		{"FireCreditInformationCallback", func(tb testing.TB, h http.Handler) { mock.FireCreditInformationCallback(tb, h, nil) }},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ftb := newFakeTB()
			tc.fn(ftb, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			if len(ftb.fatal) == 0 {
				t.Error("expected t.Fatalf for nil event")
			}
		})
	}
}

func newFakeTB() *fakeTB { return &fakeTB{} }
