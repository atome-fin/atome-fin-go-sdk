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
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"availableCredit":5000000}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPreCheck(context.Background(), &payment.PaymentPreCheckRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1000)},
		ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
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
	if !strings.Contains(string(gotBody), `"externalReferenceUid":"u-1"`) {
		t.Errorf("body missing externalReferenceUid: %s", gotBody)
	}
}

func TestService_PaymentPreCheck_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"RISK_REJECT","message":"risk declined","data":{"extendInfo":{"reapplyTime":1715000000000}}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPreCheck(context.Background(), &payment.PaymentPreCheckRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1000,
		SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1000)},
		ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
	})
	if err != nil {
		t.Fatalf("PaymentPreCheck: %v", err)
	}
	if resp.IsEligible() {
		t.Error("expected Eligible=false")
	}
	if resp.Code != atomefin.CodeRiskReject {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || resp.Data.ExtendInfo == nil || resp.Data.ExtendInfo.ReapplyTime != 1715000000000 {
		t.Errorf("ReapplyTime missing: %#v", resp.Data)
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
		SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
		ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
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
		SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
		ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
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
		{"missing-externalReferenceUid", &payment.PaymentPreCheckRequest{
			TotalAmount: 1,
			SubOrders:   []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:  &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "externalReferenceUid"},
		{"zero-totalAmount", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          0,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "totalAmount"},
		{"empty-subOrders", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{},
			ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "subOrders"},
		{"empty-subOrderId", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{{Amount: 1, Quantity: 1, SkuID: "sku", CategoryID: "c", CategoryOneName: "n", MerchantID: "m"}},
			ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "subOrderId"},
		{"zero-quantity", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders: func() []payment.PlanSubOrder {
				so := specSamplePlanSubOrder(1)
				so.Quantity = 0
				return []payment.PlanSubOrder{so}
			}(),
			ExtendInfo: &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "quantity"},
		{"sum-mismatch", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1000,
			SubOrders: func() []payment.PlanSubOrder {
				so := specSamplePlanSubOrder(999)
				return []payment.PlanSubOrder{so}
			}(),
			ExtendInfo: &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}, "totalAmount"},
		{"missing-extendInfo", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
		}, "extendInfo"},
		{"missing-orderType", &payment.PaymentPreCheckRequest{
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:           &payment.PreCheckExtendInfo{},
		}, "extendInfo.orderType"},
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
			ExternalReferenceUID: "u-1",
			TotalAmount:          v,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(v)},
			ExtendInfo:           &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}
	})
}

func TestR11_PaymentPreCheck_RejectsFractional(t *testing.T) {
	body := []byte(`{"externalReferenceUid":"u","totalAmount":1.5,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.PaymentPreCheckRequest](t, body)
}

func TestR12_PaymentPreCheckRequest_IntegerLiterals(t *testing.T) {
	in := payment.PaymentPreCheckRequest{
		ExternalReferenceUID: "u",
		TotalAmount:          1500000,
		SubOrders: []payment.PlanSubOrder{
			specSamplePlanSubOrder(1000000),
			func() payment.PlanSubOrder {
				so := specSamplePlanSubOrder(500000)
				so.SubOrderID = "so-2"
				so.Quantity = 2
				return so
			}(),
		},
		ExtendInfo: &payment.PreCheckExtendInfo{OrderType: payment.OrderTypeGrabFood},
	}
	marshal.AssertAmountKeysAreInteger[payment.PaymentPreCheckRequest](t, in,
		"totalAmount", "amount",
	)
}
