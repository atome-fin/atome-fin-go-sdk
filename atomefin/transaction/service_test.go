package transaction_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
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
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":1,"items":[{"tradeId":"TRD-1","tradeType":"AUTH","authOrderId":"AUTH-1","currency":"IDR","amount":1000,"tradeStatus":"SUCCESS","tradeTime":1746084600000}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
		ExternalReferenceUID: "user-42",
		PageNumber:           1,
		PageSize:             20,
	})
	if err != nil {
		t.Fatalf("Transactions: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || len(resp.Data.Items) != 1 {
		t.Fatalf("Data = %#v", resp.Data)
	}
	if resp.Data.Items[0].TradeType != transaction.TradeTypeAuth {
		t.Errorf("TradeType = %q", resp.Data.Items[0].TradeType)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/transactions" {
		t.Errorf("path = %q", gotPath)
	}
	wantQuery := "externalReferenceUid=user-42&pageNumber=1&pageSize=20"
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
		gotCanonical := []byte(sign.CanonicalQuery(r.URL.Query()))
		if vErr := verifier.Verify(r.Context(), gotCanonical, r.Header.Get("Authorization")); vErr != nil {
			t.Errorf("R13 multi-param: verify failed.\nwire: %s\ncanonical: %s\nerr: %v",
				r.URL.RawQuery, gotCanonical, vErr)
		}
		if r.URL.RawQuery != string(gotCanonical) {
			t.Errorf("R13 multi-param: wire query != canonical.\nwire: %s\ncanonical: %s",
				r.URL.RawQuery, gotCanonical)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":2,"pageSize":50,"total":0,"items":[]}}`))
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

	// 7-param query: pageNumber, pageSize, externalReferenceUid,
	// authOrderId, tradeType, startDate, endDate. Alphabetical sort
	// shuffles them: authOrderId < endDate < externalReferenceUid <
	// pageNumber < pageSize < startDate < tradeType.
	if _, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
		PageNumber:           2,
		PageSize:             50,
		ExternalReferenceUID: "user-42",
		AuthOrderID:          "AUTH-2026-0501-0001",
		TradeType:            transaction.TradeTypeRefund,
		StartDate:            "2026-05-01",
		EndDate:              "2026-05-31",
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
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"tradeId":"TRD-1","tradeType":"CAPTURE","authOrderId":"AUTH-1","orderId":"ORD-1","currency":"IDR","amount":1000,"tradeStatus":"SUCCESS","tradeTime":1746089800000,"billId":"202605","notes":"x"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := transaction.New(c).TransactionDetail(context.Background(), "TRD-1")
	if err != nil {
		t.Fatalf("TransactionDetail: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || resp.Data.TradeID != "TRD-1" {
		t.Errorf("Data = %#v", resp.Data)
	}
	if resp.Data.BillID != "202605" {
		t.Errorf("BillID = %q", resp.Data.BillID)
	}
	if gotPath != "/transactionDetail" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "tradeId=TRD-1" {
		t.Errorf("RawQuery = %q", gotQuery)
	}
}

func TestService_TransactionDetail_RejectsEmptyTradeID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty tradeID")
	})))
	_, err := transaction.New(c).TransactionDetail(context.Background(), "")
	mustValidationError(t, err, "tradeId")
}

// ---------- TransactionsAll auto-pagination ----------

func TestService_TransactionsAll_AutoPaginates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("pageNumber")
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":2,"total":3,"items":[{"tradeId":"TRD-1","tradeType":"AUTH","authOrderId":"A","currency":"IDR","amount":1,"tradeStatus":"SUCCESS","tradeTime":1},{"tradeId":"TRD-2","tradeType":"CAPTURE","authOrderId":"A","currency":"IDR","amount":1,"tradeStatus":"SUCCESS","tradeTime":2}]}}`))
		case "2":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":2,"pageSize":2,"total":3,"items":[{"tradeId":"TRD-3","tradeType":"REFUND","authOrderId":"A","currency":"IDR","amount":1,"tradeStatus":"SUCCESS","tradeTime":3}]}}`))
		default:
			t.Errorf("unexpected pageNumber=%q", page)
		}
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	all, err := transaction.New(c).TransactionsAll(context.Background(), &transaction.TransactionsParams{PageSize: 2})
	if err != nil {
		t.Fatalf("TransactionsAll: %v", err)
	}
	if got := len(all); got != 3 {
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
	_, err := transaction.New(c).Transactions(context.Background(), nil)
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
	if _, err := svc.TransactionDetail(context.Background(), "TRD-1"); err == nil {
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
		{"negative-pageNumber", &transaction.TransactionsParams{PageNumber: -1}, "pageNumber"},
		{"negative-pageSize", &transaction.TransactionsParams{PageSize: -1}, "pageSize"},
		{"oversize-pageSize", &transaction.TransactionsParams{PageSize: 1001}, "pageSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Transactions(context.Background(), tc.p)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- TradeType enum ----------

func TestTradeType_IsValid(t *testing.T) {
	for _, v := range []transaction.TradeType{
		transaction.TradeTypeAuth,
		transaction.TradeTypeCapture,
		transaction.TradeTypeVoid,
		transaction.TradeTypeRefund,
	} {
		if !v.IsValid() {
			t.Errorf("TradeType(%q).IsValid() = false; want true", v)
		}
	}
	for _, v := range []transaction.TradeType{"", "auth", "REPAYMENT", "FOO"} {
		if v.IsValid() {
			t.Errorf("TradeType(%q).IsValid() = true; want false", v)
		}
	}
}

func TestTradeType_StringIsWireLiteral(t *testing.T) {
	if got := transaction.TradeTypeRefund.String(); got != "REFUND" {
		t.Errorf("TradeTypeRefund.String() = %q", got)
	}
}

// ---------- Default-pagination expansion ----------

func TestTransactions_NilParamsUsesDefaults(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":0,"items":[]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	if _, err := transaction.New(c).Transactions(context.Background(), nil); err != nil {
		t.Fatalf("Transactions(nil): %v", err)
	}
	want := fmt.Sprintf("pageNumber=%d&pageSize=%d", transaction.DefaultPageNumber, transaction.DefaultPageSize)
	if gotQuery != want {
		t.Errorf("RawQuery = %q, want %q", gotQuery, want)
	}
}
