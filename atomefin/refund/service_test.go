package refund_test

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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- Test scaffolding (mirrors payment/service_test.go) ----------

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

// ---------- Refund POST happy path ----------

func TestService_Refund_Success(t *testing.T) {
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
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"refunded","data":{"requestId":"r-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","refundOrderId":"RFD-1","currency":"IDR","refundAmount":1000,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := refund.New(c).Refund(context.Background(), &refund.RefundParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		CaptureRequestID:     "CAP-1",
		RefundAmount:         1000,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 1000},
		},
	})
	if err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if resp.Data.RefundOrderID != "RFD-1" {
		t.Errorf("RefundOrderID = %q", resp.Data.RefundOrderID)
	}
	if gotPath != "/refund" {
		t.Errorf("path = %q, want /refund", gotPath)
	}
	if !strings.Contains(string(gotBody), `"requestId":"r-1"`) {
		t.Errorf("body missing requestId: %s", gotBody)
	}
}

func TestService_Refund_AutoMintsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &refund.RefundParam{
		ExternalReferenceUID: "u-1",
		CaptureRequestID:     "CAP-1",
		RefundAmount:         1,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 1},
		},
	}
	if _, err := refund.New(c).Refund(context.Background(), req); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	if req.RequestID == "" {
		t.Error("RequestID was not auto-minted")
	}
	if len(req.RequestID) > 64 {
		t.Errorf("auto-minted requestId exceeds spec maxlength 64: %d", len(req.RequestID))
	}
}

func TestService_Refund_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_MISSING","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := refund.New(c).Refund(context.Background(), &refund.RefundParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		CaptureRequestID:     "CAP-1",
		RefundAmount:         1,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 1},
		},
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsMissing {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- QueryRefund GET happy path ----------

func TestService_QueryRefund_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","refundOrderId":"RFD-1","currency":"IDR","refundAmount":1000,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := refund.New(c).QueryRefund(context.Background(), "r-1", "u-1")
	if err != nil {
		t.Fatalf("QueryRefund: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if gotPath != "/query-refund" {
		t.Errorf("path = %q, want /query-refund", gotPath)
	}
	// Alphabetical canonical → externalReferenceUid before requestId.
	if gotQuery != "externalReferenceUid=u-1&requestId=r-1" {
		t.Errorf("RawQuery = %q, want externalReferenceUid=u-1&requestId=r-1", gotQuery)
	}
}

func TestService_QueryRefund_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"unknown requestId"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := refund.New(c).QueryRefund(context.Background(), "r-missing", "u-1")
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeNotFound {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- Polling helper ----------

func TestService_RefundPollUntilTerminal_PollsUntilSuccess(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 2 {
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","refundOrderId":"RFD-1","currency":"IDR","refundAmount":1,"status":"PROCESSING"}}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","refundOrderId":"RFD-1","currency":"IDR","refundAmount":1,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := refund.New(c).RefundPollUntilTerminal(context.Background(), &refund.RefundParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		CaptureRequestID:     "CAP-1",
		RefundAmount:         1,
		SubOrders: []refund.SubOrderRefundRequest{
			{SubOrderID: "so-1", Amount: 1},
		},
	}, payment.PollOptions{
		MaxWait:      2 * time.Second,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     5 * time.Millisecond,
		Multiplier:   1.5,
	})
	if err != nil {
		t.Fatalf("RefundPollUntilTerminal: %v", err)
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
	if refund.New(nil) != nil {
		t.Error("refund.New(nil) must return nil")
	}
}

// ---------- Nil-receiver safety ----------

func TestNilService_AllMethodsReturnError(t *testing.T) {
	var svc *refund.Service

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()

	if _, err := svc.Refund(context.Background(), &refund.RefundParam{}); err == nil {
		t.Error("Refund on nil receiver: want error, got nil")
	}
	if _, err := svc.QueryRefund(context.Background(), "r", "u"); err == nil {
		t.Error("QueryRefund on nil receiver: want error, got nil")
	}
	if _, err := svc.RefundPollUntilTerminal(context.Background(), &refund.RefundParam{}, payment.PollOptions{MaxWait: time.Second}); err == nil {
		t.Error("RefundPollUntilTerminal on nil receiver: want error, got nil")
	}
	if got := svc.Client(); got != nil {
		t.Error("Client() on nil receiver must return nil")
	}
}

// ---------- Validation table ----------

func TestRefund_Validate_TableDriven(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := refund.New(c)

	cases := []struct {
		name      string
		req       *refund.RefundParam
		wantField string
	}{
		{"nil-request", nil, "request"},
		{"long-requestId", &refund.RefundParam{
			RequestID:            strings.Repeat("a", 65),
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         1,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 1}},
		}, "requestId"},
		{"missing-externalReferenceUid", &refund.RefundParam{
			RequestID:        "r",
			CaptureRequestID: "C",
			RefundAmount:     1,
			SubOrders:        []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 1}},
		}, "externalReferenceUid"},
		{"missing-captureRequestId", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			RefundAmount:         1,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 1}},
		}, "captureRequestId"},
		{"zero-refundAmount", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         0,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 1}},
		}, "refundAmount"},
		{"empty-subOrders", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         1,
			SubOrders:            []refund.SubOrderRefundRequest{},
		}, "subOrders"},
		{"empty-subOrderId", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         1,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "", Amount: 1}},
		}, "subOrderId"},
		{"zero-sub-amount", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         1,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 0}},
		}, "subOrders[].amount"},
		{"sum-mismatch-Q25", &refund.RefundParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			CaptureRequestID:     "C",
			RefundAmount:         1000,
			SubOrders:            []refund.SubOrderRefundRequest{{SubOrderID: "s", Amount: 999}},
		}, "refundAmount"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Refund(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- QueryRefund validation ----------

func TestQueryRefund_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := refund.New(c)

	if _, err := svc.QueryRefund(context.Background(), "", "u-1"); err == nil {
		t.Error("QueryRefund(\"\", \"u-1\") must reject")
	} else {
		mustValidationError(t, err, "requestId")
	}

	long := strings.Repeat("a", 65)
	if _, err := svc.QueryRefund(context.Background(), long, "u-1"); err == nil {
		t.Error("QueryRefund(>64, \"u-1\") must reject")
	} else {
		mustValidationError(t, err, "requestId")
	}

	// v0.2 fix: externalReferenceUid is spec-required.
	if _, err := svc.QueryRefund(context.Background(), "r-1", ""); err == nil {
		t.Error("QueryRefund(\"r-1\", \"\") must reject")
	} else {
		mustValidationError(t, err, "externalReferenceUid")
	}
}
