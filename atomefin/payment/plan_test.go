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

// ---------- /payment-plan happy path ----------

func TestService_PaymentPlan_Success(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","plans":[{"periodType":3,"currency":"IDR","totalAmount":1545000,"totalFee":45000},{"periodType":6,"currency":"IDR","totalAmount":1590000,"totalFee":90000}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPlan(context.Background(), &payment.PaymentPlanRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PlanSubOrder{
			{SubOrderID: "so-1", Amount: 1500000, Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("PaymentPlan: %v", err)
	}
	if resp.Code != atomefin.CodeSuccess {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || len(resp.Data.Plans) != 2 {
		t.Fatalf("expected 2 plans, got %#v", resp.Data)
	}
	if resp.Data.Plans[0].PeriodType != 3 || resp.Data.Plans[1].PeriodType != 6 {
		t.Errorf("PeriodType ordering = %d, %d", resp.Data.Plans[0].PeriodType, resp.Data.Plans[1].PeriodType)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/payment-plan" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.Contains(string(gotBody), `"requestId":"r-1"`) {
		t.Errorf("body missing requestId: %s", gotBody)
	}
}

func TestService_PaymentPlan_AutoMintsRequestID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	req := &payment.PaymentPlanRequest{
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PlanSubOrder{
			{SubOrderID: "so-1", Amount: 1, Quantity: 1},
		},
	}
	if _, err := payment.New(c).PaymentPlan(context.Background(), req); err != nil {
		t.Fatalf("PaymentPlan: %v", err)
	}
	if req.RequestID == "" {
		t.Error("RequestID was not auto-minted")
	}
}

func TestService_PaymentPlan_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_WRONG","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := payment.New(c).PaymentPlan(context.Background(), &payment.PaymentPlanRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		Currency:             atomefin.CurrencyIDR,
		SubOrders: []payment.PlanSubOrder{
			{SubOrderID: "so-1", Amount: 1, Quantity: 1},
		},
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsWrong {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- Validation table ----------

func TestPaymentPlan_Validate_TableDriven(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := payment.New(c)

	cases := []struct {
		name      string
		req       *payment.PaymentPlanRequest
		wantField string
	}{
		{"nil-request", nil, "request"},
		{"missing-externalReferenceUid", &payment.PaymentPlanRequest{
			RequestID:   "r",
			TotalAmount: 1,
			Currency:    atomefin.CurrencyIDR,
			SubOrders:   []payment.PlanSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "externalReferenceUid"},
		{"zero-totalAmount", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          0,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PlanSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "totalAmount"},
		{"non-IDR-currency", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             "EUR",
			SubOrders:            []payment.PlanSubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		}, "currency"},
		{"empty-subOrders", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PlanSubOrder{},
		}, "subOrders"},
		{"sum-mismatch", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1000,
			Currency:             atomefin.CurrencyIDR,
			SubOrders:            []payment.PlanSubOrder{{SubOrderID: "s", Amount: 999, Quantity: 1}},
		}, "totalAmount"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PaymentPlan(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- Marshal harness — R10/R11/R12 ----------

func TestPaymentPlanRequest_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPlanRequest](t, fixtureRoot+"payment_plan_request.json")
}

func TestPaymentPlanResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPlanResponse](t, fixtureRoot+"payment_plan_response.json")
}

// Empty-plans round-trip pin — the paginated-list pattern from chunk #3
// applies to PaymentPlanData.Plans (bare json:"plans", no omitempty)
// so a 0-tenor response round-trips as `"plans":[]` rather than
// disappearing.
func TestPaymentPlanResponse_Roundtrip_Empty(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPlanResponse](t, fixtureRoot+"payment_plan_response_empty.json")
}

func TestR10_PaymentPlanRequest_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.PaymentPlanRequest](t, func(v int64) payment.PaymentPlanRequest {
		return payment.PaymentPlanRequest{
			RequestID:            "r-1",
			ExternalReferenceUID: "u-1",
			TotalAmount:          v,
			Currency:             atomefin.CurrencyIDR,
			SubOrders: []payment.PlanSubOrder{
				{SubOrderID: "so-1", Amount: v, Quantity: 1},
			},
		}
	})
}

func TestR10_CommerceInstallmentDetail_AllAmounts(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.CommerceInstallmentDetail](t, func(v int64) payment.CommerceInstallmentDetail {
		return payment.CommerceInstallmentDetail{
			InstallmentID: "INS-1",
			DueDate:       "2026-06-15",
			Principal:     v,
			Fee:           v,
			Interest:      v,
			Amount:        v,
		}
	})
}

func TestR11_PaymentPlan_RejectsFractional(t *testing.T) {
	body := []byte(`{"requestId":"r","externalReferenceUid":"u","totalAmount":1.5,"currency":"IDR","subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.PaymentPlanRequest](t, body)
}

func TestR12_CommerceInstallmentPlan_IntegerLiterals(t *testing.T) {
	in := payment.CommerceInstallmentPlan{
		PeriodType:  3,
		Currency:    atomefin.CurrencyIDR,
		TotalAmount: 1545000,
		TotalFee:    45000,
		Installments: []payment.CommerceInstallmentDetail{
			{InstallmentID: "INS-1", DueDate: "2026-06-15", Principal: 500000, Fee: 15000, Interest: 0, Amount: 515000},
		},
	}
	marshal.AssertAmountKeysAreInteger[payment.CommerceInstallmentPlan](t, in,
		"totalAmount", "totalFee", "principal", "fee", "interest", "amount",
	)
}
