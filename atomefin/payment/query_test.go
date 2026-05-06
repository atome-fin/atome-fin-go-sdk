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

// ---------- Happy paths ----------

func TestQueryAuth_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","currency":"IDR","authOrderId":"AUTH-1","totalAmount":1500000,"status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).QueryAuth(context.Background(), "r-1", "u-1")
	if err != nil {
		t.Fatalf("QueryAuth: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal response")
	}
	if resp.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("AuthOrderID = %q", resp.Data.AuthOrderID)
	}
	if gotPath != "/query-auth" {
		t.Errorf("path = %q, want /query-auth", gotPath)
	}
	// Alphabetical canonical → externalReferenceUid before requestId.
	if gotQuery != "externalReferenceUid=u-1&requestId=r-1" {
		t.Errorf("RawQuery = %q, want externalReferenceUid=u-1&requestId=r-1", gotQuery)
	}
}

func TestQueryCapture_Success(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/query-capture" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"c-1","orderId":"O-1","currency":"IDR","totalAmount":1500000,"status":"SUCCESS","authOrderId":"AUTH-1"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).QueryCapture(context.Background(), "c-1", "u-1")
	if err != nil {
		t.Fatalf("QueryCapture: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal response")
	}
	if resp.Data.OrderID != "O-1" {
		t.Errorf("OrderID = %q", resp.Data.OrderID)
	}
	if gotQuery != "externalReferenceUid=u-1&requestId=c-1" {
		t.Errorf("RawQuery = %q, want externalReferenceUid=u-1&requestId=c-1", gotQuery)
	}
}

func TestQueryVoidAuth_Success(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Path != "/query-voidAuth" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"v-1","externalReferenceUid":"u-1","authOrderId":"AUTH-1","status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := payment.New(c).QueryVoidAuth(context.Background(), "v-1", "u-1")
	if err != nil {
		t.Fatalf("QueryVoidAuth: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal response")
	}
	if gotQuery != "externalReferenceUid=u-1&requestId=v-1" {
		t.Errorf("RawQuery = %q, want externalReferenceUid=u-1&requestId=v-1", gotQuery)
	}
}

// ---------- Validation rejections — common to all three Query methods ----------

func TestQuery_RejectsEmptyRequestID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty requestId")
	})))
	svc := payment.New(c)

	for name, fn := range map[string]func() error{
		"QueryAuth":     func() error { _, e := svc.QueryAuth(context.Background(), "", "u-1"); return e },
		"QueryCapture":  func() error { _, e := svc.QueryCapture(context.Background(), "", "u-1"); return e },
		"QueryVoidAuth": func() error { _, e := svc.QueryVoidAuth(context.Background(), "", "u-1"); return e },
	} {
		err := fn()
		var ve *atomefin.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: err = %v; want *ValidationError", name, err)
			continue
		}
		if !strings.Contains(ve.Field, "requestId") {
			t.Errorf("%s: err.Field = %q; want requestId", name, ve.Field)
		}
	}
}

func TestQuery_RejectsLongRequestID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on overlong requestId")
	})))
	svc := payment.New(c)
	longID := strings.Repeat("a", 65)

	for name, fn := range map[string]func() error{
		"QueryAuth":     func() error { _, e := svc.QueryAuth(context.Background(), longID, "u-1"); return e },
		"QueryCapture":  func() error { _, e := svc.QueryCapture(context.Background(), longID, "u-1"); return e },
		"QueryVoidAuth": func() error { _, e := svc.QueryVoidAuth(context.Background(), longID, "u-1"); return e },
	} {
		err := fn()
		var ve *atomefin.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: err = %v; want *ValidationError", name, err)
			continue
		}
		if !strings.Contains(ve.Message, "maxlength") {
			t.Errorf("%s: err.Message = %q; want maxlength rejection", name, ve.Message)
		}
	}
}

// TestQuery_RejectsEmptyExternalReferenceUID exercises the v0.2 fix —
// externalReferenceUid is spec-required on each Query* GET. Passing
// empty must be rejected before the network round-trip.
func TestQuery_RejectsEmptyExternalReferenceUID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty externalReferenceUid")
	})))
	svc := payment.New(c)

	for name, fn := range map[string]func() error{
		"QueryAuth":     func() error { _, e := svc.QueryAuth(context.Background(), "r-1", ""); return e },
		"QueryCapture":  func() error { _, e := svc.QueryCapture(context.Background(), "r-1", ""); return e },
		"QueryVoidAuth": func() error { _, e := svc.QueryVoidAuth(context.Background(), "r-1", ""); return e },
	} {
		err := fn()
		var ve *atomefin.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("%s: err = %v; want *ValidationError", name, err)
			continue
		}
		if !strings.Contains(ve.Field, "externalReferenceUid") {
			t.Errorf("%s: err.Field = %q; want externalReferenceUid", name, ve.Field)
		}
	}
}

// ---------- Nil-service safety ----------

func TestQuery_NilService(t *testing.T) {
	var svc *payment.Service
	if _, err := svc.QueryAuth(context.Background(), "r", "u"); err == nil {
		t.Error("QueryAuth on nil service must error")
	}
	if _, err := svc.QueryCapture(context.Background(), "r", "u"); err == nil {
		t.Error("QueryCapture on nil service must error")
	}
	if _, err := svc.QueryVoidAuth(context.Background(), "r", "u"); err == nil {
		t.Error("QueryVoidAuth on nil service must error")
	}
}

// ---------- 4xx surfaces APIError ----------

func TestQueryAuth_4xxBecomesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"unknown requestId"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	_, err := payment.New(c).QueryAuth(context.Background(), "r-missing", "u-1")
	var ae *atomefin.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("err = %v; want *APIError", err)
	}
	if ae.Code != atomefin.CodeNotFound {
		t.Errorf("Code = %q, want NOT_FOUND", ae.Code)
	}
}
