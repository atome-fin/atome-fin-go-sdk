package payment_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// Validation tests focused on the rejection paths in auth.go / capture.go /
// void.go — these all run client-side (no httptest server) and lift
// payment-package coverage substantially.

// ---------- Auth rejections ----------

func TestAuth_Validate_NilRequest(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := payment.New(c).Auth(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "nil") {
		t.Errorf("err = %v; want nil-request validation", err)
	}
}

func TestAuth_Validate_RejectsLongRequestID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:            strings.Repeat("a", 65),
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{specSampleSubOrder(1)},
		ExtendInfo:           specSampleRequestExtendInfo(),
		Sessionid:            "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "requestId")
}

func TestAuth_Validate_RejectsEmptyExternalReferenceUID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:   "r",
		TotalAmount: 1,
		PeriodType:  1,
		SubOrders:   []payment.SubOrder{specSampleSubOrder(1)},
		ExtendInfo:  specSampleRequestExtendInfo(),
		Sessionid:   "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "externalReferenceUid")
}

func TestAuth_Validate_RejectsEmptySubOrders(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{},
		ExtendInfo:           specSampleRequestExtendInfo(),
		Sessionid:            "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "subOrders")
}

func TestAuth_Validate_RejectsEmptySubOrderID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "", Amount: 1, Quantity: 1, SkuID: "sku", CategoryID: "c", CategoryOneName: "n", MerchantID: "m"}},
		ExtendInfo:           specSampleRequestExtendInfo(),
		Sessionid:            "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "subOrderId")
}

func TestAuth_Validate_RejectsZeroSubOrderAmount(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders: func() []payment.SubOrder {
			so := specSampleSubOrder(1)
			so.Amount = 0
			return []payment.SubOrder{so}
		}(),
		ExtendInfo: specSampleRequestExtendInfo(),
		Sessionid:  "s",
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "amount")
}

func TestAuth_Validate_RejectsLongSessionid(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{specSampleSubOrder(1)},
		ExtendInfo:           specSampleRequestExtendInfo(),
		Sessionid:            strings.Repeat("a", 65),
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "sessionid")
}

func TestAuth_Validate_RejectsBadCreditScore(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	bad := 1.5
	req := &payment.AuthRequest{
		RequestID:            "r",
		ExternalReferenceUID: "u",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{specSampleSubOrder(1)},
		Sessionid:            "s",
		ExtendInfo: &payment.RequestExtendInfo{
			OrderType:       payment.OrderTypeGrabFood,
			UserCreditScore: &bad,
		},
	}
	_, err := payment.New(c).Auth(context.Background(), req)
	mustValidationError(t, err, "userCreditScore")
}

// ---------- Capture rejections ----------

func TestCapture_Validate_NilRequest(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := payment.New(c).Capture(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCapture_Validate_RejectsEmptyExternalReferenceUID(t *testing.T) {
	// v0.1.1 fix: capture now requires externalReferenceUid (latent
	// non-compliance in v0.1.0). The validator must reject before the
	// network round-trip.
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when externalReferenceUid is empty")
	})))
	req := &payment.CaptureRequest{
		RequestID:   "c-1",
		AuthOrderID: "AUTH-1",
		TotalAmount: 1,
		PeriodType:  1,
		SubOrders:   []payment.SubOrder{specSampleSubOrder(1)},
		ExtendInfo:  specSampleRequestExtendInfo(),
	}
	_, err := payment.New(c).Capture(context.Background(), req)
	mustValidationError(t, err, "externalReferenceUid")
}

func TestCapture_Validate_MissingAuthOrderID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.CaptureRequest{
		RequestID:            "c-1",
		ExternalReferenceUID: "user-42",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{specSampleSubOrder(1)},
		ExtendInfo:           specSampleRequestExtendInfo(),
	}
	_, err := payment.New(c).Capture(context.Background(), req)
	mustValidationError(t, err, "authOrderId")
}

func TestCapture_Validate_SumMismatch(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	req := &payment.CaptureRequest{
		RequestID:            "c-1",
		ExternalReferenceUID: "user-42",
		AuthOrderID:          "AUTH-1",
		TotalAmount:          1000,
		PeriodType:           1,
		SubOrders: func() []payment.SubOrder {
			so := specSampleSubOrder(999)
			return []payment.SubOrder{so}
		}(),
		ExtendInfo: specSampleRequestExtendInfo(),
	}
	_, err := payment.New(c).Capture(context.Background(), req)
	mustValidationError(t, err, "totalAmount")
}

// ---------- VoidAuth rejections ----------

func TestVoidAuth_Validate_NilRequest(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	_, err := payment.New(c).VoidAuth(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVoidAuth_Validate_AllRequiredFields(t *testing.T) {
	cases := []struct {
		req     *payment.VoidAuthRequest
		wantSub string
	}{
		// Empty RequestID is auto-minted by VoidAuth(), so it never
		// reaches validateVoidAuthRequest's empty-string branch — drop
		// that case. Length and missing-field paths still apply.
		{&payment.VoidAuthRequest{RequestID: "r", AuthOrderID: "A"}, "externalReferenceUid"},
		{&payment.VoidAuthRequest{RequestID: "r", ExternalReferenceUID: "u"}, "authOrderId"},
		{&payment.VoidAuthRequest{RequestID: strings.Repeat("a", 65), ExternalReferenceUID: "u", AuthOrderID: "A"}, "requestId"},
	}
	for _, tc := range cases {
		_, err := payment.New(mustClientFromCase(t)).VoidAuth(context.Background(), tc.req)
		mustValidationError(t, err, tc.wantSub)
	}
}

// ---------- Service.Client + nil-receiver ----------

func TestService_ClientAccessor(t *testing.T) {
	cl := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	svc := payment.New(cl)
	if svc.Client() != cl {
		t.Error("Service.Client() did not return the wrapped *atomefin.Client")
	}
}

// ---------- Helpers ----------

func mustClientFromCase(t *testing.T) *atomefin.Client {
	t.Helper()
	return mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
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
