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
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","subOrderInstallmentPlans":[{"subOrderId":"so-1","orderAmount":1500000,"installmentPlans":[{"totalTenor":3,"orderRepayAmount":1545000,"orderInterestAmount":45000,"orderDiscountAmount":0,"installmentDetails":[{"subOrderId":"so-1","totalTenor":3,"currentTenor":1,"repayAmount":515000,"principalAmount":500000,"interestAmount":15000,"billId":"202606","billDate":"2026-06-15","dueDate":"2026-06-15"}]},{"totalTenor":6,"orderRepayAmount":1590000,"orderInterestAmount":90000,"orderDiscountAmount":0,"installmentDetails":[{"subOrderId":"so-1","totalTenor":6,"currentTenor":1,"repayAmount":265000,"principalAmount":250000,"interestAmount":15000,"billId":"202606","billDate":"2026-06-15","dueDate":"2026-06-15"}]}]}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).PaymentPlan(context.Background(), &payment.PaymentPlanRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		SubOrders: []payment.PlanSubOrder{
			specSamplePlanSubOrder(1500000),
		},
		ExtendInfo: &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
		Sessionid:  "session-plan",
	})
	if err != nil {
		t.Fatalf("PaymentPlan: %v", err)
	}
	if resp.Code != atomefin.CodeSuccess {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || len(resp.Data.SubOrderInstallmentPlans) != 1 {
		t.Fatalf("expected 1 sub-order plan group, got %#v", resp.Data)
	}
	plans := resp.Data.SubOrderInstallmentPlans[0].InstallmentPlans
	if len(plans) != 2 || plans[0].TotalTenor != 3 || plans[1].TotalTenor != 6 {
		t.Errorf("TotalTenor ordering = %#v", plans)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/payment-plan" {
		t.Errorf("path = %q", gotPath)
	}
	if strings.Contains(string(gotBody), "requestId") {
		t.Errorf("body must not contain requestId per spec: %s", gotBody)
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
		SubOrders: []payment.PlanSubOrder{
			specSamplePlanSubOrder(1),
		},
		ExtendInfo: &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
		Sessionid:  "session-plan",
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
		SubOrders: []payment.PlanSubOrder{
			specSamplePlanSubOrder(1),
		},
		ExtendInfo: &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
		Sessionid:  "session-plan",
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
			SubOrders:   []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:  &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
			Sessionid:   "s",
		}, "externalReferenceUid"},
		{"zero-totalAmount", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          0,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:           &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
			Sessionid:            "s",
		}, "totalAmount"},
		{"empty-subOrders", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{},
			ExtendInfo:           &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
			Sessionid:            "s",
		}, "subOrders"},
		{"sum-mismatch", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1000,
			SubOrders: func() []payment.PlanSubOrder {
				so := specSamplePlanSubOrder(999)
				return []payment.PlanSubOrder{so}
			}(),
			ExtendInfo: &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
			Sessionid:  "s",
		}, "totalAmount"},
		{"missing-extendInfo", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			Sessionid:            "s",
		}, "extendInfo"},
		{"missing-orderType", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo:           &payment.CheckoutExtendInfo{},
			Sessionid:            "s",
		}, "extendInfo.orderType"},
		{"mart-missing-skuInfos", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo: &payment.CheckoutExtendInfo{
				OrderType: payment.OrderTypeGrabMart,
				MainOrderExtendInfos: []payment.MainOrderExtendInfo{{
					MerchantID: "merchant-1",
				}},
			},
		}, "skuInfos"},
		{"mart-missing-skuId", &payment.PaymentPlanRequest{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			TotalAmount:          1,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(1)},
			ExtendInfo: &payment.CheckoutExtendInfo{
				OrderType: payment.OrderTypeGrabMart,
				MainOrderExtendInfos: []payment.MainOrderExtendInfo{{
					MerchantID: "merchant-1",
					SkuInfos:   []payment.SkuInfo{{Amount: 1}},
				}},
			},
		}, "skuId"},
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

// Empty sub-order plan round-trip pin.
func TestPaymentPlanResponse_Roundtrip_Empty(t *testing.T) {
	marshal.GoldenRoundTrip[payment.PaymentPlanResponse](t, fixtureRoot+"payment_plan_response_empty.json")
}

func TestR10_PaymentPlanRequest_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.PaymentPlanRequest](t, func(v int64) payment.PaymentPlanRequest {
		return payment.PaymentPlanRequest{
			ExternalReferenceUID: "u-1",
			TotalAmount:          v,
			SubOrders:            []payment.PlanSubOrder{specSamplePlanSubOrder(v)},
			ExtendInfo:           &payment.CheckoutExtendInfo{OrderType: payment.OrderTypeGrabFood},
		}
	})
}

func TestR10_CommerceInstallmentDetail_AllAmounts(t *testing.T) {
	marshal.AssertAmountRoundtrip[payment.CommerceInstallmentDetail](t, func(v int64) payment.CommerceInstallmentDetail {
		return payment.CommerceInstallmentDetail{
			SubOrderID:      "so-1",
			TotalTenor:      3,
			CurrentTenor:    1,
			RepayAmount:     v,
			PrincipalAmount: v,
			InterestAmount:  v,
			DiscountAmount:  v,
			BillID:          "202606",
			BillDate:        "2026-06-15",
			DueDate:         "2026-06-15",
		}
	})
}

func TestR11_PaymentPlan_RejectsFractional(t *testing.T) {
	body := []byte(`{"externalReferenceUid":"u","totalAmount":1.5,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[payment.PaymentPlanRequest](t, body)
}

func TestR12_CommerceInstallmentPlan_IntegerLiterals(t *testing.T) {
	in := payment.CommerceInstallmentPlan{
		TotalTenor:          3,
		OrderRepayAmount:    1545000,
		OrderInterestAmount: 45000,
		OrderDiscountAmount: 0,
		InstallmentDetails: []payment.CommerceInstallmentDetail{
			{SubOrderID: "so-1", TotalTenor: 3, CurrentTenor: 1, RepayAmount: 515000, PrincipalAmount: 500000, InterestAmount: 15000, BillID: "202606", BillDate: "2026-06-15", DueDate: "2026-06-15"},
		},
	}
	marshal.AssertAmountKeysAreInteger[payment.CommerceInstallmentPlan](t, in,
		"orderRepayAmount", "orderInterestAmount", "orderDiscountAmount", "repayAmount", "principalAmount", "interestAmount",
	)
}
