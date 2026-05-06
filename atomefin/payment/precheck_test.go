package payment_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

// ---------- /payment-precheck happy path ----------

func TestService_PaymentPreCheck_Eligible(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","externalReferenceUid":"u-1","eligible":true,"availableCredit":5000000}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPreCheck(context.Background(), &payment.PaymentPreCheckRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1000, Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("PaymentPreCheck: %v", err)
	}
	if !resp.IsEligible() {
		t.Errorf("expected Eligible=true, got %#v", resp.Data)
	}
	if resp.Data.AvailableCredit != 5000000 {
		t.Errorf("AvailableCredit = %d", resp.Data.AvailableCredit)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/payment-precheck" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), `"requestId":"r-1"`) {
		t.Errorf("body missing requestId: %s", gotBody)
	}
}

func TestService_PaymentPreCheck_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","eligible":false,"deniedReason":"credit-limit-insufficient"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPreCheck(context.Background(), &payment.PaymentPreCheckRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1000, Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("PaymentPreCheck: %v", err)
	}
	if resp.IsEligible() {
		t.Error("expected Eligible=false")
	}
	if resp.Data.DeniedReason != "credit-limit-insufficient" {
		t.Errorf("DeniedReason = %q", resp.Data.DeniedReason)
	}
}

func TestService_PaymentPreCheck_AutoMintsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &payment.PaymentPreCheckRequest{
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1, Quantity: 1},
		},
	}
	if _, err := payment.New(c).PaymentPreCheck(context.Background(), req); err != nil {
		t.Fatalf("PaymentPreCheck: %v", err)
	}
	if req.RequestID == "" {
		t.Error("RequestID was not auto-minted")
	}
	if len(req.RequestID) > 64 {
		t.Errorf("auto-minted requestId exceeds spec maxlength 64: %d", len(req.RequestID))
	}
}

func TestService_PaymentPreCheck_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_MISSING","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := payment.New(c).PaymentPreCheck(context.Background(), &payment.PaymentPreCheckRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1, Quantity: 1},
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

// ---------- Validation table ----------

func TestPaymentPreCheck_Validate_TableDriven(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := payment.New(c)

	cases := []struct {
		name      string
		req       *payment.PaymentPreCheckRequest
		wantField string
	}{
		{"nil-request", nil, "request"},
		{"long-requestId", &payment.PaymentPreCheckRequest{
			RequestID:            strings.Repeat("a", 65),
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "requestId"},
		{"missing-externalReferenceUid", &payment.PaymentPreCheckRequest{
			RequestID:   "r",
			TotalAmount: 1,
			Currency:    atomefin.CurrencyIDR,
			SubOrders:   []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "externalReferenceUid"},
		{"zero-totalAmount", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          0,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "totalAmount"},
		{"missing-currency", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "currency"},
		{"non-IDR-currency", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             "USD",
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "currency"},
		{"empty-subOrders", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{},
		}, "subOrders"},
		{"empty-subOrderId", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "", Amount: 1, Quantity: 1}},
		}, "subOrderId"},
		{"zero-quantity", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 0}},
		}, "quantity"},
		{"sum-mismatch", &payment.PaymentPreCheckRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1000,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PaymentPreCheckSubOrder{{SubOrderID: "s", Amount: 999, Quantity: 1}},
		}, "totalAmount"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PaymentPreCheck(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- Marshal harness — R10/R11/R12 ----------

func TestPaymentPreCheckRequest_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPreCheckRequest](t, fixtureRoot+"payment_precheck_request.json")
}

func TestPaymentPreCheckResponse_Roundtrip_Eligible(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPreCheckResponse](t, fixtureRoot+"payment_precheck_response_eligible.json")
}

func TestPaymentPreCheckResponse_Roundtrip_Denied(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPreCheckResponse](t, fixtureRoot+"payment_precheck_response_denied.json")
}

func TestR10_PaymentPreCheckRequest_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.PaymentPreCheckRequest](t, func(v int64) payment.PaymentPreCheckRequest {
		return payment.PaymentPreCheckRequest{
			RequestID:            "r-1",
			ExternalReferenceUID: "u-1",
			TotalAmount:          v,
			Currency:             atomefin.CurrencyIDR,
			SubOrders: []payment.PaymentPreCheckSubOrder{
				{SubOrderID: "so-1", Amount: v, Quantity: 1},
			},
		}
	})
}

func TestR11_PaymentPreCheck_RejectsFractional(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","totalAmount":1.5,"currency":"IDR","subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.PaymentPreCheckRequest](t, body)
}

func TestR12_PaymentPreCheckRequest_IntegerLiterals(t *testing.T) {
	in := payment.PaymentPreCheckRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1500000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PaymentPreCheckSubOrder{
			{SubOrderID: "so-1", Amount: 1000000, Quantity: 1},
			{SubOrderID: "so-2", Amount: 500000, Quantity: 2},
		},
	}
	marshal.AssertAmountKeysAreInteger[payment.PaymentPreCheckRequest](t, in,
		"totalAmount", "amount",
	)
}
