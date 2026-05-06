package repayment_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- Test scaffolding (mirrors refund/service_test.go) ----------

func mustClient(t testing.TB, srv *httptest.Server, extra ...atomefin.Option) *atomefin.Client {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	opts := []atomefin.Option{
		atomefin.WithPrivateKeyPEM(pemBytes),
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPartnerID("partner-test"),
		atomefin.WithRetry(transport.RetryPolicy{
			MaxAttempts:           3,
			Base:                  1 * time.Millisecond,
			Cap:                   5 * time.Millisecond,
			Jitter:                0,
			RetryOnStatus:         transport.DefaultRetryOnStatus,
			RetryOnTransportError: transport.DefaultRetryOnTransportError,
		}),
	}
	opts = append(opts, extra...)
	c, err := atomefin.New(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func mustValidationError(t *testing.T, err error, wantField string) {
	t.Helper()
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if !strings.Contains(ve.Error(), wantField) {
		t.Errorf("err = %v; want mention of %q", ve, wantField)
	}
}

// ---------- Repayment POST happy path ----------

func TestService_Repayment_Success(t *testing.T) {
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"repaid","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1000,"event":"NORMAL"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := repayment.New(c).Repayment(context.Background(), &repayment.RepaymentParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1000,
		RepaymentApplyTime:   1746662400000,
	})
	if err != nil {
		t.Fatalf("Repayment: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if resp.Data.RepaymentID != "RPM-1" {
		t.Errorf("RepaymentID = %q", resp.Data.RepaymentID)
	}
	if gotPath != "/repayment-request" {
		t.Errorf("path = %q, want /repayment-request", gotPath)
	}
	if !strings.Contains(string(gotBody), `"requestId":"r-1"`) {
		t.Errorf("body missing requestId: %s", gotBody)
	}
	if !strings.Contains(string(gotBody), `"repaymentApplyTime":1746662400000`) {
		t.Errorf("body missing repaymentApplyTime: %s", gotBody)
	}
}

func TestService_Repayment_AutoMintsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &repayment.RepaymentParam{
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1,
		RepaymentApplyTime:   1746662400000,
	}
	if _, err := repayment.New(c).Repayment(context.Background(), req); err != nil {
		t.Fatalf("Repayment: %v", err)
	}
	if req.RequestID == "" {
		t.Error("RequestID was not auto-minted")
	}
	if len(req.RequestID) > 64 {
		t.Errorf("auto-minted requestId exceeds spec maxlength 64: %d", len(req.RequestID))
	}
}

func TestService_Repayment_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_MISSING","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := repayment.New(c).Repayment(context.Background(), &repayment.RepaymentParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1,
		RepaymentApplyTime:   1746662400000,
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsMissing {
		t.Errorf("Code = %q", ae.Code)
	}
}

// 5xx → retried by transport.RetryPolicy → eventual success on attempt 2
func TestService_Repayment_RetriesOn5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"code":"SERVER_ERROR","message":"transient"}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := repayment.New(c).Repayment(context.Background(), &repayment.RepaymentParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1,
		RepaymentApplyTime:   1746662400000,
	})
	if err != nil {
		t.Fatalf("Repayment: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal after retry")
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Errorf("hits = %d, want >= 2 (retry policy must engage on 5xx)", got)
	}
}

func TestService_Repayment_ContextCancelled(t *testing.T) {
	// Server delays response long enough that the client's ctx
	// deadline fires first; the handler returns when its own request
	// context is cancelled by the client side, so srv.Close() can
	// drain cleanly.
	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-done:
		}
	}))
	defer func() {
		close(done)
		srv.Close()
	}()

	c := mustClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := repayment.New(c).Repayment(ctx, &repayment.RepaymentParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1,
		RepaymentApplyTime:   1746662400000,
	})
	if err == nil {
		t.Error("ctx cancel: want error, got nil")
	}
}

// ---------- QueryRepayment GET happy path ----------

func TestService_QueryRepayment_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1000,"event":"NORMAL"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := repayment.New(c).QueryRepayment(context.Background(), "r-1", "u-1")
	if err != nil {
		t.Fatalf("QueryRepayment: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if gotPath != "/repayment-result" {
		t.Errorf("path = %q, want /repayment-result", gotPath)
	}
	// CanonicalQuery sorts keys alphabetically: externalReferenceUid before requestId.
	if gotQuery != "externalReferenceUid=u-1&requestId=r-1" {
		t.Errorf("RawQuery = %q, want externalReferenceUid=u-1&requestId=r-1", gotQuery)
	}
}

func TestService_QueryRepayment_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"unknown requestId"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := repayment.New(c).QueryRepayment(context.Background(), "r-missing", "u-1")
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeNotFound {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- Polling helper ----------

func TestService_RepaymentPollUntilTerminal_PollsUntilSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"PROCESSING","currency":"IDR","repaymentAmount":1}}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","repaymentId":"RPM-1","status":"SUCCESS","currency":"IDR","repaymentAmount":1}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := repayment.New(c).RepaymentPollUntilTerminal(context.Background(), &repayment.RepaymentParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		RepaymentAmount:      1,
		RepaymentApplyTime:   1746662400000,
	}, payment.PollOptions{
		MaxWait:      2 * time.Second,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   1.5,
	})
	if err != nil {
		t.Fatalf("RepaymentPollUntilTerminal: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Errorf("hits = %d, want >= 2", got)
	}
}

// ---------- Constructor ----------

func TestNew_NilClient_ReturnsNil(t *testing.T) {
	if repayment.New(nil) != nil {
		t.Error("repayment.New(nil) must return nil")
	}
}

// ---------- Nil-receiver safety ----------

func TestNilService_AllMethodsReturnError(t *testing.T) {
	var svc *repayment.Service

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()

	if _, err := svc.Repayment(context.Background(), &repayment.RepaymentParam{}); err == nil {
		t.Error("Repayment on nil receiver: want error, got nil")
	}
	if _, err := svc.QueryRepayment(context.Background(), "r", "u"); err == nil {
		t.Error("QueryRepayment on nil receiver: want error, got nil")
	}
	if _, err := svc.RepaymentPollUntilTerminal(context.Background(), &repayment.RepaymentParam{}, payment.PollOptions{MaxWait: time.Second}); err == nil {
		t.Error("RepaymentPollUntilTerminal on nil receiver: want error, got nil")
	}
	if got := svc.Client(); got != nil {
		t.Error("Client() on nil receiver must return nil")
	}
}

// ---------- IsProcessing / IsTerminal nil-safety ----------

func TestRepaymentResponse_NilSafe(t *testing.T) {
	var nilResp *repayment.RepaymentResponse
	if nilResp.IsTerminal() {
		t.Error("nil response should not be terminal")
	}
	if nilResp.IsProcessing() {
		t.Error("nil response should not be processing")
	}
	emptyData := &repayment.RepaymentResponse{Code: atomefin.CodeSuccess, Message: "ok"}
	if emptyData.IsTerminal() {
		t.Error("nil .Data should not be terminal")
	}
	if emptyData.IsProcessing() {
		t.Error("nil .Data should not be processing")
	}
}
