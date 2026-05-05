package payment_test

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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- Test scaffolding ----------

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

// ---------- Auth ----------

func TestService_Auth_Success(t *testing.T) {
	var gotPath, gotSession string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSession = r.Header.Get("sessionid")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1000,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	svc := payment.New(c)

	req := &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1000, Quantity: 1}},
		Sessionid:            "session-token-abc",
	}
	resp, err := svc.Auth(context.Background(), req)
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if !resp.IsTerminal() {
		t.Errorf("expected terminal response, got %#v", resp)
	}
	if resp.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("AuthOrderID = %q", resp.Data.AuthOrderID)
	}
	if gotPath != "/auth" {
		t.Errorf("server path = %q", gotPath)
	}
	if gotSession != "session-token-abc" {
		t.Errorf("sessionid header = %q", gotSession)
	}
	// Body must contain the requestId verbatim (signing canonical).
	if !strings.Contains(string(gotBody), `"requestId":"r-1"`) {
		t.Errorf("body missing requestId: %s", gotBody)
	}
	// And the Sessionid must NOT appear in the JSON body — it travels
	// in the header, json:"-".
	if strings.Contains(string(gotBody), "sessionid") || strings.Contains(string(gotBody), "session-token-abc") {
		t.Errorf("sessionid leaked into JSON body: %s", gotBody)
	}
}

func TestService_Auth_AutoMintsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &payment.AuthRequest{
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1, Quantity: 1}},
		Sessionid:            "s",
	}
	if _, err := payment.New(c).Auth(context.Background(), req); err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if req.RequestID == "" {
		t.Error("RequestID was not auto-minted on the request")
	}
	if len(req.RequestID) > 64 {
		t.Errorf("auto-minted requestId exceeds spec maxlength 64: %d", len(req.RequestID))
	}
}

func TestService_Auth_ValidationRejectsZeroSum(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	svc := payment.New(c)

	req := &payment.AuthRequest{
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 999, Quantity: 1}},
		Sessionid:            "s",
	}
	_, err := svc.Auth(context.Background(), req)
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want ValidationError", err)
	}
	if !strings.Contains(ve.Error(), "totalAmount") {
		t.Errorf("expected totalAmount mismatch error, got %v", ve)
	}
}

func TestService_Auth_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_MISSING","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1, Quantity: 1}},
		Sessionid:            "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want APIError", err)
	}
	if ae.Code != atomefin.CodeParamsMissing {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- Capture ----------

func TestService_Capture_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"c-1","orderId":"O-1","currency":"IDR","totalAmount":1000,"status":"SUCCESS","authOrderId":"AUTH-1"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).Capture(context.Background(), &payment.CaptureRequest{
		RequestID:   "c-1",
		AuthOrderID: "AUTH-1",
		TotalAmount: 1000,
		PeriodType:  1,
		SubOrders:   []payment.SubOrder{{SubOrderID: "so-1", Amount: 1000, Quantity: 1}},
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if resp.Data.OrderID != "O-1" {
		t.Errorf("OrderID = %q", resp.Data.OrderID)
	}
	if resp.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("AuthOrderID = %q", resp.Data.AuthOrderID)
	}
}

// ---------- VoidAuth ----------

func TestService_VoidAuth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// VoidAuthRequest must NOT carry extendInfo (architect re-read).
		if strings.Contains(string(body), "extendInfo") {
			t.Errorf("/voidAuth body must not contain extendInfo: %s", body)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"v-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).VoidAuth(context.Background(), &payment.VoidAuthRequest{
		RequestID:            "v-1",
		ExternalReferenceUID: "u-1",
		AuthOrderID:          "AUTH-1",
	})
	if err != nil {
		t.Fatalf("VoidAuth: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
}

// ---------- PollUntilTerminal ----------

func TestService_AuthPollUntilTerminal_PollsUntilSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"PROCESSING"}}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	svc := payment.New(c)

	req := &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1, Quantity: 1}},
		Sessionid:            "s",
	}
	resp, err := svc.AuthPollUntilTerminal(context.Background(), req, payment.PollOptions{
		MaxWait:      2 * time.Second,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   1.5,
	})
	if err != nil {
		t.Fatalf("AuthPollUntilTerminal: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if got := atomic.LoadInt32(&hits); got < 3 {
		t.Errorf("hits = %d, want >= 3", got)
	}
}

func TestService_AuthPollUntilTerminal_RespectsMaxWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1,"status":"PROCESSING"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	svc := payment.New(c)
	start := time.Now()
	_, err := svc.AuthPollUntilTerminal(context.Background(), &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1, Quantity: 1}},
		Sessionid:            "s",
	}, payment.PollOptions{MaxWait: 100 * time.Millisecond, InitialDelay: 1 * time.Millisecond, MaxDelay: 5 * time.Millisecond, Multiplier: 1.1})
	dur := time.Since(start)

	if err == nil {
		t.Fatal("expected MaxWait to expire")
	}
	if dur > 1*time.Second {
		t.Errorf("MaxWait not honoured; ran for %v", dur)
	}
}

// ---------- CapturePollUntilTerminal mirrors Auth's poll wrapper ----------

func TestService_CapturePollUntilTerminal_PollsUntilSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"c-1","orderId":"O-1","currency":"IDR","totalAmount":1,"status":"PROCESSING","authOrderId":"AUTH-1"}}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"c-1","orderId":"O-1","currency":"IDR","totalAmount":1,"status":"SUCCESS","authOrderId":"AUTH-1"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).CapturePollUntilTerminal(context.Background(), &payment.CaptureRequest{
		RequestID:   "c-1",
		AuthOrderID: "AUTH-1",
		TotalAmount: 1,
		PeriodType:  1,
		SubOrders:   []payment.SubOrder{{SubOrderID: "so-1", Amount: 1, Quantity: 1}},
	}, payment.PollOptions{
		MaxWait:      2 * time.Second,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   1.5,
	})
	if err != nil {
		t.Fatalf("CapturePollUntilTerminal: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Errorf("hits = %d, want >= 2", got)
	}
}

// ---------- New(nil) returns nil ----------

func TestNewNilClient(t *testing.T) {
	if payment.New(nil) != nil {
		t.Error("payment.New(nil) must return nil so the caller fails fast")
	}
}
