// Package docs hosts CI-tested examples that mirror the patterns
// documented in MOCK_MODE.md verbatim. Compiled and run on every
// `make test`, so doc drift is caught at the test gate.
//
// These are NOT user-facing examples in the godoc sense (the
// package name is `docs_test`, deliberately not the SDK package);
// they exist so the snippets in MOCK_MODE.md stay green.
package docs_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// freshTestKeyPEM mints a one-shot RSA-2048 key in PKCS#1 PEM
// form. Mirrors what partners do once at the top of their test
// file. Returns it as []byte so it slots straight into
// WithPrivateKeyPEM.
func freshTestKeyPEM(tb testing.TB) []byte {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("rsa.GenerateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// freshAuthRequest is a small canonical /auth request used by
// both worked examples below. Real partners would build this
// from their cart state.
func freshAuthRequest() *payment.AuthRequest {
	return &payment.AuthRequest{
		RequestID:            "r-mock-1",
		ExternalReferenceUID: "u-mock-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders: []payment.SubOrder{
			{
				SubOrderID: "so-1", MerchantID: "merchant-1",
				MerchantName: "KFC Sudirman", MerchantCategory: "FAST_FOOD",
				Amount: 1500000,
			},
		},
		ExtendInfo: specExtendInfo(),
		Sessionid:  "session-mock",
	}
}

// specExtendInfo returns the minimal spec-valid RequestExtendInfo for
// GRAB_* auth/capture on the pinned 2026-08-14 swagger.
func specExtendInfo() *payment.RequestExtendInfo {
	return &payment.RequestExtendInfo{
		OrderType:     payment.OrderTypeGrabFood,
		CreditProfile: payment.CreditProfile(`{"score":720}`),
		DeviceInfo: &payment.DeviceInfo{
			Platform: payment.PlatformAndroid,
			Device: &payment.DeviceProfile{
				DeviceID:            "dev-1",
				GoogleAdvertisingID: "gaid-1",
				UTDID:               "utdid-1",
				Build: &payment.BuildInfo{
					Board: "sdm845", Brand: "xiaomi", Device: "dipper",
					Manufacturer: "Xiaomi", Model: "MI 8", Product: "dipper",
				},
			},
			WifiList:  []payment.WifiAP{{SSID: "ssid-demo"}},
			IPAddress: &payment.IPAddress{EthIP: "10.0.0.2", TrueIP: "203.0.113.7"},
		},
		Address: &payment.Address{
			ShippingAddress: &payment.ShippingAddress{City: "Jakarta", Address1: "Jl. Sudirman No. 1"},
			ShippingName:    "Budi Santoso",
			ShippingPhoneNo: "+6281234567890",
		},
		MainOrderExtendInfos: []payment.MainOrderExtendInfo{
			{MerchantID: "merchant-1", SkuInfos: []payment.SkuInfo{{SkuID: "sku-1", Amount: 1500000}}},
		},
	}
}

// ---------- Pattern A — WithHTTPClient + RoundTripper ----------

// canned200 is a tiny http.RoundTripper that replies SUCCESS to
// every request. Adapt body / status per test as needed.
type canned200 struct{ body string }

func (rt canned200) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(bytes.NewBufferString(rt.body)),
	}, nil
}

// TestMockMode_Pattern_WithHTTPClient mirrors the first worked
// example in MOCK_MODE.md verbatim. Catches drift between the
// doc snippet and the live SDK surface.
func TestMockMode_Pattern_WithHTTPClient(t *testing.T) {
	c, err := atomefin.New(
		atomefin.WithBaseURL("https://atome-fin.test"), // any URL — never dialled
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
		atomefin.WithHTTPClient(&http.Client{
			Transport: canned200{body: `{"code":"SUCCESS","message":"ok","data":{"requestId":"r-mock-1","authOrderId":"AUTH-1","status":"SUCCESS","totalAmount":1500000,"currency":"IDR"}}`},
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := payment.New(c).Auth(context.Background(), freshAuthRequest())
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if !resp.IsTerminal() {
		t.Errorf("expected terminal response, got %#v", resp)
	}
	if resp.Data == nil || resp.Data.AuthOrderID != "AUTH-1" {
		t.Errorf("Data = %#v", resp.Data)
	}
}

// ---------- Pattern B — httptest.NewServer ----------

// TestMockMode_Pattern_HttptestNewServer mirrors the second
// worked example in MOCK_MODE.md.
func TestMockMode_Pattern_HttptestNewServer(t *testing.T) {
	var gotPath string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := payment.New(c).Auth(context.Background(), freshAuthRequest()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/auth" {
		t.Errorf("path = %q", gotPath)
	}
	if len(gotBody) == 0 {
		t.Error("body was empty")
	}
}

// Compile-only check that `WithHTTPClient` is a stable public
// option — pin so MOCK_MODE.md's snippet doesn't silently break
// if the option is renamed.
var _ = func() atomefin.Option { return atomefin.WithHTTPClient(http.DefaultClient) }

// Sanity: the example helper compiles and produces the expected
// shape. Catches accidental signature drift early.
func TestMockMode_FreshAuthRequest_HasRequiredFields(t *testing.T) {
	req := freshAuthRequest()
	if req.RequestID == "" || req.ExternalReferenceUID == "" || len(req.SubOrders) == 0 {
		t.Errorf("freshAuthRequest missing required fields: %#v", req)
	}
	if req.TotalAmount <= 0 {
		t.Errorf("TotalAmount = %d", req.TotalAmount)
	}
	_ = fmt.Sprintf // imports keep the compiler happy if helpers shrink
}
