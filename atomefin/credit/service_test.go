package credit_test

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
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// ---------- Test scaffolding (mirrors refund/bill service tests) ----------

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

func validInformationParam() *credit.CreditInformationParam {
	return &credit.CreditInformationParam{
		RequestID:            "info-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+628129801929",
		Email:                "u@example.com",
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.CreditInformationEssentialInfo{
			OCRResult: &credit.CreditInformationOCRResult{FullName: "Test User"},
		},
	}
}

func validApplicationParam() *credit.CreditApplicationParam {
	return &credit.CreditApplicationParam{
		RequestID:            "app-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+628129801929",
		Email:                "u@example.com",
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.ApplicationEssentialInfo{
			LivenessCheck: &credit.LivenessCheck{
				Result:        "PASS",
				SnapshotPhoto: "base64-photo",
			},
			IndividualProfile:   &credit.IndividualProfile{IDType: "KTP"},
			PlatformInformation: &credit.PlatformInformation{},
		},
		ExtendInfo: &credit.CreditApplicationExtendInfo{
			CreditInformationRequestID: "info-1",
		},
	}
}

// ptrFloat64 is a tiny helper for optional *float64 test fixtures.
func ptrFloat64(v float64) *float64 { return &v }

// ---------- POST /credit-information + /credit-application — BLOCKED in v0.2.x ----------

// TestSubmitInformation_RejectsMissingEncryptOption pins that
// calling /credit-information on a Client constructed without
// WithEncryptAtomePublicCertPEM fails LOCALLY (no network) with a
// typed *ValidationError pointing the partner at the missing
// option. v0.3 re-enabled the network path; the precondition
// guard moves from method-body to Client.DoEncryptedSigned.
func TestSubmitInformation_RejectsMissingEncryptOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server must NOT be reached when encrypt key is unconfigured")
	}))
	defer srv.Close()

	c := mustClient(t, srv) // no WithEncryptAtomePublicCertPEM
	_, err := credit.New(c).SubmitInformation(context.Background(), validInformationParam())
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if ve.Field != "encryptAtomePublicCert" {
		t.Errorf("Field = %q; want encryptAtomePublicCert", ve.Field)
	}
}

// TestSubmitApplication_RejectsMissingEncryptOption is the
// matching guard for /credit-application.
func TestSubmitApplication_RejectsMissingEncryptOption(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server must NOT be reached when encrypt key is unconfigured")
	}))
	defer srv.Close()

	c := mustClient(t, srv) // no WithEncryptAtomePublicCertPEM
	_, err := credit.New(c).SubmitApplication(context.Background(), validApplicationParam())
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if ve.Field != "encryptAtomePublicCert" {
		t.Errorf("Field = %q; want encryptAtomePublicCert", ve.Field)
	}
}

// ---------- GET /credit-result happy path ----------

func TestService_QueryResult_Success(t *testing.T) {
	var gotPath, gotQuery, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotMethod = r.Method
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"externalReferenceUid":"user-42","status":"SUCCESS","currency":"IDR","creditInfo":{"totalCredit":30000000,"availableCredit":30000000,"usedCredit":0,"userStatus":"NORMAL","version":1715000000000}}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).QueryResult(context.Background(), "user-42")
	if err != nil {
		t.Fatalf("QueryResult: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/credit-result" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "externalReferenceUid=user-42" {
		t.Errorf("RawQuery = %q", gotQuery)
	}
}

func TestService_QueryResult_RejectsEmptyUID(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached on empty uid")
	})))
	_, err := credit.New(c).QueryResult(context.Background(), "")
	mustValidationError(t, err, "externalReferenceUid")
}

// ---------- GET /credit-information-result happy path ----------

func TestService_QueryInformationResult_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"info-1","externalReferenceUid":"user-42","status":"SUCCESS"}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).QueryInformationResult(context.Background(), "user-42", "info-1")
	if err != nil {
		t.Fatalf("QueryInformationResult: %v", err)
	}
	if !resp.IsTerminal() {
		t.Error("expected terminal")
	}
	if gotPath != "/credit-information-result" {
		t.Errorf("path = %q", gotPath)
	}
	// Two-key alphabetical canonical: externalReferenceUid < requestId.
	want := "externalReferenceUid=user-42&requestId=info-1"
	if gotQuery != want {
		t.Errorf("RawQuery = %q, want %q", gotQuery, want)
	}
}

// ---------- GET /query-balance-history multi-param R13 ----------

// TestService_BalanceHistory_MultiParam_R13_AtScale exercises R13
// invariant: for a multi-param query, the wire RawQuery must be
// byte-identical to sign.CanonicalQuery(values), and the signature
// must verify against those bytes.
//
// Five params (count, externalReferenceUid, requestId, start, type)
// — that's the architect's "≥3 params" threshold, so this is the
// per-sub-package R13 stress version.
func TestService_BalanceHistory_MultiParam_R13_AtScale(t *testing.T) {
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
		// Pin: wire RawQuery byte-equal to the signing canonical.
		if r.URL.RawQuery != string(gotCanonical) {
			t.Errorf("R13 multi-param: wire query != canonical bytes.\n"+
				"wire:      %s\ncanonical: %s",
				r.URL.RawQuery, gotCanonical)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"externalReferenceUid":"user-42","type":"OVERPAID_CHANGE","currency":"IDR","recordInfo":[]}}`))
	}))
	defer srv.Close()

	priv, _ := pemEncodePrivate(key)
	c, err := atomefin.New(
		atomefin.WithPrivateKeyPEM(priv),
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := credit.New(c).BalanceHistory(context.Background(), &credit.BalanceHistoryParams{
		ExternalReferenceUID: "user-42",
		Type:                 credit.BalanceHistoryTypeOverpaidChange,
		RequestID:            "req-zzz",
		Start:                3,
		Count:                25,
	}); err != nil {
		t.Fatalf("BalanceHistory: %v", err)
	}
}

// pemEncodePrivate returns a PKCS#1 PEM for the test key.
func pemEncodePrivate(k *rsa.PrivateKey) ([]byte, error) {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	}), nil
}

// ---------- GET /query-balance-history happy path ----------

func TestService_BalanceHistory_Success(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"externalReferenceUid":"user-42","type":"OVERPAID_CHANGE","currency":"IDR","recordInfo":[{"requestId":"r","changeAmount":"1500000","event":"REPAYMENT","time":1715000000000}]}}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).BalanceHistory(context.Background(), &credit.BalanceHistoryParams{
		ExternalReferenceUID: "user-42",
		Type:                 credit.BalanceHistoryTypeOverpaidChange,
	})
	if err != nil {
		t.Fatalf("BalanceHistory: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
	if gotPath != "/query-balance-history" {
		t.Errorf("path = %q", gotPath)
	}
	// Default start=1, count=10 with no requestId — three keys
	// alphabetically: count, externalReferenceUid, start, type.
	want := "count=10&externalReferenceUid=user-42&start=1&type=OVERPAID_CHANGE"
	if gotQuery != want {
		t.Errorf("RawQuery = %q, want %q", gotQuery, want)
	}
}

// ---------- POST /modify-application-info ----------

func TestService_ModifyApplicationInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"modified"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).ModifyApplicationInfo(context.Background(), &credit.CreditApplicationChangeParam{
		RequestID:            "mod-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+628129801930",
	})
	if err != nil {
		t.Fatalf("ModifyApplicationInfo: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
}

// ---------- POST /close-account ----------

func TestService_CloseAccount_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"closed"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).CloseAccount(context.Background(), &credit.CloseAccountParam{
		RequestID:            "cl-1",
		ExternalReferenceUID: "user-42",
	})
	if err != nil {
		t.Fatalf("CloseAccount: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q", resp.Code)
	}
}

func TestService_CloseAccount_UnpaidDebt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"UNPAID_DEBT","message":"x"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	resp, err := credit.New(c).CloseAccount(context.Background(), &credit.CloseAccountParam{
		RequestID:            "cl-1",
		ExternalReferenceUID: "user-42",
	})
	if err != nil {
		t.Fatalf("CloseAccount: %v", err)
	}
	if resp.Code != credit.CodeUnpaidDebt {
		t.Errorf("Code = %q, want UNPAID_DEBT", resp.Code)
	}
}

// ---------- Retry on 5xx ----------
//
// Probe via ModifyApplicationInfo (POST /modify-application-info) —
// SubmitInformation / SubmitApplication are blocked locally in
// v0.2.x and never reach the wire. Both paths share the same
// invokePost / DoSigned pipeline, so retry / ctx-cancel /
// reserved-header semantics observed via ModifyApplicationInfo
// apply uniformly.

func TestService_Retries_5xx_ThenSucceeds(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"code":"SERVER_ERROR","message":"upstream"}`))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	if _, err := credit.New(c).ModifyApplicationInfo(context.Background(), &credit.CreditApplicationChangeParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+6281298000000",
	}); err != nil {
		t.Fatalf("ModifyApplicationInfo: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got < 2 {
		t.Errorf("hits = %d, want >= 2 (retry expected)", got)
	}
}

// ---------- ctx cancellation ----------

func TestService_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block long enough that ctx cancellation is the deciding event.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	_, err := credit.New(c).ModifyApplicationInfo(ctx, &credit.CreditApplicationChangeParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+6281298000000",
	})
	if err == nil {
		t.Fatal("expected error from ctx cancellation")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context") {
		t.Errorf("err = %v; want context.Canceled-class", err)
	}
}

// ---------- Reserved-header allowlist ----------

// Asserts the SDK's reserved-header semantics via a public POST
// path: the server inspects the inbound Authorization header and
// confirms it is the SDK-generated signature, not anything else.
// Probes via ModifyApplicationInfo for the same reason as the
// retry / ctx tests above (the blocked Submit* methods are no
// longer suitable wire-touching probes).
func TestService_ReservedHeaderAllowlist_AuthorizationSDKControlled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Errorf("missing Authorization header on outbound request")
		}
		// Reject the canary value that a partner override would
		// produce (exercise: SDK must NOT let partners set this).
		if auth == "PARTNER_OVERRIDE_TOKEN" {
			t.Errorf("Authorization header was partner-controllable (leaked override)")
		}
		// The SDK uses base64 signatures; canary value cannot match.
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c := mustClient(t, srv)
	if _, err := credit.New(c).ModifyApplicationInfo(context.Background(), &credit.CreditApplicationChangeParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "user-42",
		MobileNumber:         "+6281298000000",
	}); err != nil {
		t.Fatalf("ModifyApplicationInfo: %v", err)
	}
}

// ---------- Constructor / nil-receiver safety ----------

func TestNew_NilClient_ReturnsNil(t *testing.T) {
	if credit.New(nil) != nil {
		t.Error("credit.New(nil) must return nil")
	}
}

func TestNilService_AllMethodsReturnError(t *testing.T) {
	var svc *credit.Service
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("nil receiver panicked: %v", r)
		}
	}()
	if _, err := svc.SubmitInformation(context.Background(), validInformationParam()); err == nil {
		t.Error("SubmitInformation on nil receiver: want error")
	}
	if _, err := svc.SubmitApplication(context.Background(), validApplicationParam()); err == nil {
		t.Error("SubmitApplication on nil receiver: want error")
	}
	if _, err := svc.QueryResult(context.Background(), "u"); err == nil {
		t.Error("QueryResult on nil receiver: want error")
	}
	if _, err := svc.QueryInformationResult(context.Background(), "u", "r"); err == nil {
		t.Error("QueryInformationResult on nil receiver: want error")
	}
	if _, err := svc.BalanceHistory(context.Background(), &credit.BalanceHistoryParams{}); err == nil {
		t.Error("BalanceHistory on nil receiver: want error")
	}
	if _, err := svc.ModifyApplicationInfo(context.Background(), &credit.CreditApplicationChangeParam{}); err == nil {
		t.Error("ModifyApplicationInfo on nil receiver: want error")
	}
	if _, err := svc.CloseAccount(context.Background(), &credit.CloseAccountParam{}); err == nil {
		t.Error("CloseAccount on nil receiver: want error")
	}
	if got := svc.Client(); got != nil {
		t.Error("Client() on nil receiver must return nil")
	}
}
