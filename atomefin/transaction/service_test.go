package transaction_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transaction"
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

// ---------- Transactions happy path ----------

func TestService_Transactions_Success(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","paymentInfo":[{"captureRequestId":"CAP-1","orderId":"ORD-1","totalTenor":3,"createTime":1746084600000}],"paginator":{"start":1,"count":10,"totalCount":1}}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
		ExternalReferenceUID: "user-42",
		StartDate:            "20260501",
		EndDate:              "20260531",
		TransactionType:      transaction.TransactionTypePayment,
	})
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || len(resp.Data.PaymentInfo) != 1 {
		t.Fatalf("Data = %#v", resp.Data)
	}
	if resp.Data.PaymentInfo[0].CaptureRequestID != "CAP-1" {
		t.Errorf("CaptureRequestID = %q", resp.Data.PaymentInfo[0].CaptureRequestID)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/transactions" {
		t.Errorf("path = %q", gotPath)
	}
	wantQuery := "count=10&endDate=20260531&externalReferenceUid=user-42&start=1&startDate=20260501&transactionType=PAYMENT"
	if gotQuery != wantQuery {
		t.Errorf("RawQuery = %q, want %q", gotQuery, wantQuery)
	}
}

// ---------- Multi-param R13 stress ----------

// 5+ params; server reconstructs canonical from r.URL.Query() and
// runs the verifier. Mirrors bill's TestService_Bills_MultiParam_R13_AtScale.
func TestService_Transactions_MultiParam_R13_AtScale(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := sign.NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		canonicalStr, _ := sign.CanonicalQuery(r.URL.Query())
		gotCanonical := []byte(canonicalStr)
		if vErr := verifier.Verify(r.Context(), gotCanonical, r.Header.Get("Authorization")); vErr != nil {
			t.Errorf("R13 multi-param: verify failed.\nwire: %s\ncanonical: %s\nerr: %v",
				r.URL.RawQuery, gotCanonical, vErr)
		}
		if r.URL.RawQuery != string(gotCanonical) {
			t.Errorf("R13 multi-param: wire query != canonical.\nwire: %s\ncanonical: %s",
				r.URL.RawQuery, gotCanonical)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","paymentInfo":[],"paginator":{"start":2,"count":50,"totalCount":0}}}`))
	}))
	defer srv.Close()

	priv := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	c, err := atomefin.New(
		atomefin.WithPrivateKeyPEM(priv),
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
		ExternalReferenceUID: "user-42",
		TransactionType:      transaction.TransactionTypeRefund,
		StartDate:            "20260501",
		EndDate:              "20260531",
		Start:                2,
		Count:                50,
	}); err != nil {
		t.Fatalf("Transactions: %v", err)
	}
}

// ---------- TransactionDetail ----------

func TestService_TransactionDetail_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","paymentInfo":{"captureRequestId":"REQ-1","orderId":"ORD-1","totalTenor":3,"createTime":1746089800000,"principalAmount":1000,"subOrders":[{"subOrderId":"so-1","principalAmount":1000,"billOrderDetails":[{"billId":"202605","billDate":"20260515","dueDate":"20260615","totalAmount":1000,"repaidAmount":0,"principalAmount":1000,"interestAmount":0}]}]}}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := transaction.New(c).TransactionDetail(context.Background(), "REQ-1", "user-42", transaction.TransactionTypePayment)
	if err != nil {
		t.Fatalf("TransactionDetail: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || resp.Data.PaymentInfo == nil || resp.Data.PaymentInfo.CaptureRequestID != "REQ-1" {
		t.Errorf("Data = %#v", resp.Data)
	}
	if resp.Data.PaymentInfo.SubOrders[0].BillOrderDetails[0].BillID != "202605" {
		t.Errorf("BillID = %q", resp.Data.PaymentInfo.SubOrders[0].BillOrderDetails[0].BillID)
	}
	if gotPath != "/transactionDetail" {
		t.Errorf("path = %q", gotPath)
	}
	// Alphabetical canonical:
	// externalReferenceUid < requestId < transactionType.
	wantQ := "externalReferenceUid=user-42&requestId=REQ-1&transactionType=PAYMENT"
	if gotQuery != wantQ {
		t.Errorf("RawQuery = %q, want %q", gotQuery, wantQ)
	}
}

func TestService_TransactionDetail_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := transaction.New(c)

	if _, err := svc.TransactionDetail(context.Background(), "", "u", transaction.TransactionTypePayment); err == nil {
		t.Error("empty requestId must reject")
	} else {
		mustValidationError(t, err, "requestId")
	}
	if _, err := svc.TransactionDetail(context.Background(), "r", "", transaction.TransactionTypePayment); err == nil {
		t.Error("empty externalReferenceUid must reject")
	} else {
		mustValidationError(t, err, "externalReferenceUid")
	}
	if _, err := svc.TransactionDetail(context.Background(), "r", "u", ""); err == nil {
		t.Error("empty transactionType must reject")
	} else {
		mustValidationError(t, err, "transactionType")
	}
}

// ---------- TransactionsAll auto-pagination ----------

func TestService_TransactionsAll_AutoPaginates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("start")
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
		switch start {
		case "1":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","paymentInfo":[{"captureRequestId":"CAP-1","orderId":"ORD-1","totalTenor":3,"createTime":1},{"captureRequestId":"CAP-2","orderId":"ORD-2","totalTenor":3,"createTime":2}],"paginator":{"start":1,"count":2,"totalCount":3}}}`))
		case "3":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"currency":"IDR","paymentInfo":[{"captureRequestId":"CAP-3","orderId":"ORD-3","totalTenor":3,"createTime":3}],"paginator":{"start":3,"count":2,"totalCount":3}}}`))
		default:
			t.Errorf("unexpected start=%q", start)
		}
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	all, err := transaction.New(c).TransactionsAll(context.Background(), &transaction.TransactionsParams{
		ExternalReferenceUID: "u",
		StartDate:            "20260501",
		EndDate:              "20260531",
		TransactionType:      transaction.TransactionTypePayment,
		Count:                2,
	})
	if err != nil {
		t.Fatalf("TransactionsAll: %v", err)
	}
	if got := len(all.PaymentInfo); got != 3 {
		t.Errorf("len = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("hits = %d, want 2", got)
	}
}

// ---------- 4xx ----------

func TestService_Transactions_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_WRONG","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
		ExternalReferenceUID: "u",
		StartDate:            "20260501",
		EndDate:              "20260531",
		TransactionType:      transaction.TransactionTypePayment,
	})
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeParamsWrong {
		t.Errorf("Code = %q", ae.Code)
	}
}

// ---------- Constructor / nil safety ----------

func TestNew_NilClient_ReturnsNil(t *testing.T) {
	if transaction.New(nil) != nil {
		t.Error("transaction.New(nil) must return nil")
	}
}

func TestNilService_AllMethodsReturnError(t *testing.T) {
	var svc *transaction.Service
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()
	if _, err := svc.Transactions(context.Background(), nil); err == nil {
		t.Error("Transactions on nil receiver must error")
	}
	if _, err := svc.TransactionDetail(context.Background(), "REQ-1", "u", transaction.TransactionTypePayment); err == nil {
		t.Error("TransactionDetail on nil receiver must error")
	}
	if _, err := svc.TransactionsAll(context.Background(), nil); err == nil {
		t.Error("TransactionsAll on nil receiver must error")
	}
	if got := svc.Client(); got != nil {
		t.Error("Client() on nil receiver must return nil")
	}
}

// ---------- Validation ----------

func TestTransactions_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := transaction.New(c)

	cases := []struct {
		name      string
		p         *transaction.TransactionsParams
		wantField string
	}{
		{"missing-externalReferenceUid", &transaction.TransactionsParams{StartDate: "20260501", EndDate: "20260531", TransactionType: transaction.TransactionTypePayment}, "externalReferenceUid"},
		{"missing-startDate", &transaction.TransactionsParams{ExternalReferenceUID: "u", EndDate: "20260531", TransactionType: transaction.TransactionTypePayment}, "startDate"},
		{"missing-endDate", &transaction.TransactionsParams{ExternalReferenceUID: "u", StartDate: "20260501", TransactionType: transaction.TransactionTypePayment}, "endDate"},
		{"missing-transactionType", &transaction.TransactionsParams{ExternalReferenceUID: "u", StartDate: "20260501", EndDate: "20260531"}, "transactionType"},
		{"negative-start", &transaction.TransactionsParams{ExternalReferenceUID: "u", StartDate: "20260501", EndDate: "20260531", TransactionType: transaction.TransactionTypePayment, Start: -1}, "start"},
		{"negative-count", &transaction.TransactionsParams{ExternalReferenceUID: "u", StartDate: "20260501", EndDate: "20260531", TransactionType: transaction.TransactionTypePayment, Count: -1}, "count"},
		{"oversize-count", &transaction.TransactionsParams{ExternalReferenceUID: "u", StartDate: "20260501", EndDate: "20260531", TransactionType: transaction.TransactionTypePayment, Count: 51}, "count"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Transactions(context.Background(), tc.p)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- TransactionType enum ----------

func TestTransactionType_IsValid(t *testing.T) {
	for _, v := range []transaction.TransactionType{
		transaction.TransactionTypePayment,
		transaction.TransactionTypeRefund,
		transaction.TransactionTypeRepayment,
	} {
		if !v.IsValid() {
			t.Errorf("TransactionType(%q).IsValid() = false; want true", v)
		}
	}
	for _, v := range []transaction.TransactionType{"", "payment", "AUTH", "FOO"} {
		if v.IsValid() {
			t.Errorf("TransactionType(%q).IsValid() = true; want false", v)
		}
	}
}

func TestTransactionType_StringIsWireLiteral(t *testing.T) {
	if got := transaction.TransactionTypeRefund.String(); got != "REFUND" {
		t.Errorf("TransactionTypeRefund.String() = %q", got)
	}
}

// ---------- Required query params ----------

func TestTransactions_NilParamsRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on nil params")
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := transaction.New(c).Transactions(context.Background(), nil)
	mustValidationError(t, err, "externalReferenceUid")
}
