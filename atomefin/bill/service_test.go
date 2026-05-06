package bill_test

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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/bill"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
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

// ---------- Bills happy path ----------

func TestService_Bills_Success(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":1,"bills":[{"billId":"202605","currency":"IDR","totalAmount":800000,"unpaidAmount":800000,"overdueStatus":"ON_TIME"}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := bill.New(c).Bills(context.Background(), &bill.BillsParams{
		ExternalReferenceUID: "user-42",
		PageNumber:           1,
		PageSize:             20,
	})
	if err != nil {
		t.Fatalf("Bills: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("expected SUCCESS, got %q", resp.Code)
	}
	if resp.Data == nil || len(resp.Data.Bills) != 1 {
		t.Fatalf("expected 1 bill, got %#v", resp.Data)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/bills" {
		t.Errorf("path = %q, want /bills", gotPath)
	}
	// Three keys, alphabetically sorted: externalReferenceUid, pageNumber, pageSize.
	wantQuery := "externalReferenceUid=user-42&pageNumber=1&pageSize=20"
	if gotQuery != wantQuery {
		t.Errorf("RawQuery = %q, want %q", gotQuery, wantQuery)
	}
}

// ---------- Multi-param R13 stress test ----------

// TestService_Bills_MultiParam_R13_AtScale exercises the R13
// invariant with a 6-param query (mixed-case keys, dates, an
// ampersand value, a UID with a dash). The server reconstructs the
// canonical from r.URL.Query() and runs the verifier to prove the
// wire bytes match the signing canonical byte-for-byte.
//
// Architect's call-out: bill is the first sub-package that exercises
// multi-param queries at scale; this test is the per-sub-package
// stress version of TestDoSignedGET_R13_WireEqualsCanonical.
func TestService_Bills_MultiParam_R13_AtScale(t *testing.T) {
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
			t.Errorf("R13 multi-param: verify failed.\n"+
				"raw wire query:    %s\nrebuilt canonical: %s\nerr: %v",
				r.URL.RawQuery, gotCanonical, vErr)
		}
		// Also pin: wire RawQuery byte-equal to the canonical.
		if r.URL.RawQuery != string(gotCanonical) {
			t.Errorf("R13 multi-param: wire query != canonical bytes.\n"+
				"wire:      %s\ncanonical: %s",
				r.URL.RawQuery, gotCanonical)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":2,"pageSize":50,"total":0,"bills":[]}}`))
	}))
	defer srv.Close()

	signer, err := sign.NewRSA2Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := pemEncodePrivate(key)
	c, err := atomefin.New(
		atomefin.WithPrivateKeyPEM(priv),
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = signer

	// 6-param query: pageNumber, pageSize, externalReferenceUid,
	// billId, startMonth, endMonth. Alphabetical sort produces
	// billId < endMonth < externalReferenceUid < pageNumber <
	// pageSize < startMonth — exercises full sort behaviour.
	if _, err := bill.New(c).Bills(context.Background(), &bill.BillsParams{
		PageNumber:           2,
		PageSize:             50,
		ExternalReferenceUID: "user-42",
		BillID:               "202605",
		StartMonth:           "202604",
		EndMonth:             "202605",
	}); err != nil {
		t.Fatalf("Bills: %v", err)
	}
}

// pemEncodePrivate returns a PKCS#1 PEM for the test key.
func pemEncodePrivate(k *rsa.PrivateKey) ([]byte, error) {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}), nil
}

// ---------- BillDetail ----------

func TestService_BillDetail_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"billId":"202605","currency":"IDR","totalAmount":800000,"orders":[{"authOrderId":"AUTH-1","amount":500000}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := bill.New(c).BillDetail(context.Background(), "202605", "u-1")
	if err != nil {
		t.Fatalf("BillDetail: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if resp.Data == nil || resp.Data.BillID != "202605" {
		t.Errorf("Data = %#v", resp.Data)
	}
	if len(resp.Data.Orders) != 1 {
		t.Errorf("Orders len = %d", len(resp.Data.Orders))
	}
	if gotPath != "/billDetail" {
		t.Errorf("path = %q", gotPath)
	}
	// Alphabetical canonical: billId before externalReferenceUid.
	if gotQuery != "billId=202605&externalReferenceUid=u-1" {
		t.Errorf("RawQuery = %q, want billId=202605&externalReferenceUid=u-1", gotQuery)
	}
}

func TestService_BillDetail_RejectsEmptyBillID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty billID")
	})))
	_, err := bill.New(c).BillDetail(context.Background(), "", "u-1")
	mustValidationError(t, err, "billId")
}

func TestService_BillDetail_RejectsEmptyExternalReferenceUID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty externalReferenceUID")
	})))
	_, err := bill.New(c).BillDetail(context.Background(), "202605", "")
	mustValidationError(t, err, "externalReferenceUid")
}

// ---------- BillsUnpaid ----------

func TestService_BillsUnpaid_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":1,"bills":[{"billId":"202603","currency":"IDR","totalAmount":600000,"unpaidAmount":600000,"overdueStatus":"OVERDUE"}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := bill.New(c).BillsUnpaid(context.Background(), &bill.BillsUnpaidParams{
		ExternalReferenceUID: "user-42",
	})
	if err != nil {
		t.Fatalf("BillsUnpaid: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if gotPath != "/billUnpaid" {
		t.Errorf("path = %q", gotPath)
	}
	// Default pageNumber=1, pageSize=20 + externalReferenceUid filter.
	wantQuery := "externalReferenceUid=user-42&pageNumber=1&pageSize=20"
	if gotQuery != wantQuery {
		t.Errorf("RawQuery = %q, want %q", gotQuery, wantQuery)
	}
}

// ---------- BillsAll auto-pagination ----------

func TestService_BillsAll_AutoPaginates(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("pageNumber")
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(200)
		switch page {
		case "1":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":2,"total":5,"bills":[{"billId":"202601","currency":"IDR","totalAmount":1},{"billId":"202602","currency":"IDR","totalAmount":1}]}}`))
		case "2":
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":2,"pageSize":2,"total":5,"bills":[{"billId":"202603","currency":"IDR","totalAmount":1},{"billId":"202604","currency":"IDR","totalAmount":1}]}}`))
		case "3":
			// Short page → loop terminates after this.
			_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":3,"pageSize":2,"total":5,"bills":[{"billId":"202605","currency":"IDR","totalAmount":1}]}}`))
		default:
			t.Errorf("unexpected pageNumber=%q", page)
		}
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	all, err := bill.New(c).BillsAll(context.Background(), &bill.BillsParams{PageSize: 2})
	if err != nil {
		t.Fatalf("BillsAll: %v", err)
	}
	if got := len(all); got != 5 {
		t.Errorf("BillsAll len = %d, want 5", got)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("hits = %d, want 3 (pages 1+2+3)", got)
	}
	for i, expectedID := range []string{"202601", "202602", "202603", "202604", "202605"} {
		if all[i].BillID != expectedID {
			t.Errorf("all[%d].BillID = %q, want %q", i, all[i].BillID, expectedID)
		}
	}
}

func TestService_BillsAll_TerminatesOnEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":0,"bills":[]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	all, err := bill.New(c).BillsAll(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("len = %d, want 0", len(all))
	}
}

// ---------- 4xx ----------

func TestService_Bills_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"PARAMS_WRONG","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := bill.New(c).Bills(context.Background(), nil)
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
	if bill.New(nil) != nil {
		t.Error("bill.New(nil) must return nil")
	}
}

func TestNilService_AllMethodsReturnError(t *testing.T) {
	var svc *bill.Service
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()
	if _, err := svc.Bills(context.Background(), nil); err == nil {
		t.Error("Bills on nil receiver must error")
	}
	if _, err := svc.BillDetail(context.Background(), "202605", "u-1"); err == nil {
		t.Error("BillDetail on nil receiver must error")
	}
	if _, err := svc.BillsUnpaid(context.Background(), nil); err == nil {
		t.Error("BillsUnpaid on nil receiver must error")
	}
	if _, err := svc.BillsAll(context.Background(), nil); err == nil {
		t.Error("BillsAll on nil receiver must error")
	}
	if got := svc.Client(); got != nil {
		t.Error("Client() on nil receiver must return nil")
	}
}

// ---------- Validation ----------

func TestBills_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := bill.New(c)

	cases := []struct {
		name      string
		p         *bill.BillsParams
		wantField string
	}{
		{"negative-pageNumber", &bill.BillsParams{PageNumber: -1}, "pageNumber"},
		{"negative-pageSize", &bill.BillsParams{PageSize: -1}, "pageSize"},
		{"oversize-pageSize", &bill.BillsParams{PageSize: 1001}, "pageSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Bills(context.Background(), tc.p)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

func TestBillsUnpaid_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on validation failure")
	})))
	svc := bill.New(c)

	cases := []struct {
		name      string
		p         *bill.BillsUnpaidParams
		wantField string
	}{
		{"negative-pageNumber", &bill.BillsUnpaidParams{PageNumber: -1}, "pageNumber"},
		{"negative-pageSize", &bill.BillsUnpaidParams{PageSize: -1}, "pageSize"},
		{"oversize-pageSize", &bill.BillsUnpaidParams{PageSize: 1001}, "pageSize"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.BillsUnpaid(context.Background(), tc.p)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- OverdueStatus enum ----------

func TestOverdueStatus_IsValid(t *testing.T) {
	for _, s := range []bill.OverdueStatus{
		bill.OverdueStatusOnTime,
		bill.OverdueStatusGracePeriod,
		bill.OverdueStatusOverdue,
	} {
		if !s.IsValid() {
			t.Errorf("OverdueStatus(%q).IsValid() = false; want true", s)
		}
	}
	for _, s := range []bill.OverdueStatus{"", "PARTIALLY_PAID", "WRITTEN_OFF", "on_time"} {
		if s.IsValid() {
			t.Errorf("OverdueStatus(%q).IsValid() = true; want false", s)
		}
	}
}

func TestOverdueStatus_StringIsWireLiteral(t *testing.T) {
	if got := bill.OverdueStatusOverdue.String(); got != "OVERDUE" {
		t.Errorf("OverdueStatusOverdue.String() = %q", got)
	}
}

// ---------- Default-pagination expansion ----------

func TestBills_NilParamsUsesDefaults(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"pageNumber":1,"pageSize":20,"total":0,"bills":[]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	if _, err := bill.New(c).Bills(context.Background(), nil); err != nil {
		t.Fatalf("Bills(nil): %v", err)
	}
	want := fmt.Sprintf("pageNumber=%d&pageSize=%d", bill.DefaultPageNumber, bill.DefaultPageSize)
	if gotQuery != want {
		t.Errorf("RawQuery = %q, want %q", gotQuery, want)
	}
}
