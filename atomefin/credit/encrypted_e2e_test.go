package credit_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// TestE2E_SubmitInformation_DecryptingServer drives a full v0.3
// hybrid-encrypt flow end-to-end against an httptest.Server that
// owns Atome's encrypt PRIVATE key. The server unwraps the
// per-request AES key from the Encrypt header, decrypts the body,
// asserts the SDK sent a well-formed CreditInformationParam, and
// returns a fixture 200.
//
// Confirms (in one test):
//   - SDK marshals the request struct correctly
//   - Encrypt header carries a valid wrapped key
//   - The wire body is the AES ciphertext (signed canonical)
//   - Server-side decryption recovers the original plaintext
//   - SDK decodes the plaintext response normally
func TestE2E_SubmitInformation_DecryptingServer(t *testing.T) {
	atomeEncryptKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey atome: %v", err)
	}
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey sign: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/credit-information" {
			t.Errorf("path = %q, want /credit-information", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		encHeader := r.Header.Get(encrypt.EncryptHeaderName)
		if encHeader == "" {
			t.Errorf("missing Encrypt header")
			http.Error(w, `{"code":"PARAMS_MISSING","message":"missing Encrypt"}`, http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", r.Header.Get("Content-Type"))
		}

		body, rerr := io.ReadAll(r.Body)
		if rerr != nil {
			t.Errorf("read body: %v", rerr)
			return
		}
		// Decrypt the inbound body via the partner-protocol's
		// hybrid envelope (we own the Atome encrypt private key).
		plain, derr := encrypt.Unmarshal(encHeader, string(body), atomeEncryptKey)
		if derr != nil {
			t.Errorf("Unmarshal: %v", derr)
			http.Error(w, `{"code":"INVALID_ENCRYPTION","message":""}`, http.StatusBadRequest)
			return
		}

		// Decode the plaintext into the spec-defined shape and
		// assert the SDK sent the right fields.
		var got credit.CreditInformationParam
		if jerr := json.Unmarshal(plain, &got); jerr != nil {
			t.Errorf("decode plaintext as CreditInformationParam: %v", jerr)
			return
		}
		if got.RequestID == "" {
			t.Error("decoded RequestID empty (auto-mint failed?)")
		}
		if got.ExternalReferenceUID != "user-42" {
			t.Errorf("decoded ExternalReferenceUID = %q", got.ExternalReferenceUID)
		}
		if got.Country != credit.CountryIndonesia {
			t.Errorf("decoded Country = %q", got.Country)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"requestId":"r-1","externalReferenceUid":"user-42","status":"DRAFT","jumpUrl":"https://x"}}`))
	}))
	defer srv.Close()

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(pemForRSA(t, signKey)),
		atomefin.WithEncryptAtomePublicCertPEM(pubPEMForRSA(t, &atomeEncryptKey.PublicKey)),
		atomefin.WithRetry(transport.RetryPolicy{
			MaxAttempts:           3,
			Base:                  1 * time.Millisecond,
			Cap:                   5 * time.Millisecond,
			Jitter:                0,
			RetryOnStatus:         transport.DefaultRetryOnStatus,
			RetryOnTransportError: transport.DefaultRetryOnTransportError,
		}),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	resp, err := credit.New(c).SubmitInformation(context.Background(), validInformationParam())
	if err != nil {
		t.Fatalf("SubmitInformation: %v", err)
	}
	if !resp.IsSuccess() {
		t.Errorf("Code = %q, want SUCCESS", resp.Code)
	}
	if resp.Data == nil || resp.Data.JumpURL != "https://x" {
		t.Errorf("Data = %#v", resp.Data)
	}
}

// TestE2E_SubmitApplication_DecryptingServer is the matching
// end-to-end test for /credit-application.
func TestE2E_SubmitApplication_DecryptingServer(t *testing.T) {
	atomeEncryptKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey atome: %v", err)
	}
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey sign: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encHeader := r.Header.Get(encrypt.EncryptHeaderName)
		body, _ := io.ReadAll(r.Body)
		plain, derr := encrypt.Unmarshal(encHeader, string(body), atomeEncryptKey)
		if derr != nil {
			t.Errorf("Unmarshal: %v", derr)
			http.Error(w, `{"code":"INVALID_ENCRYPTION","message":""}`, http.StatusBadRequest)
			return
		}
		var got credit.CreditApplicationParam
		if jerr := json.Unmarshal(plain, &got); jerr != nil {
			t.Errorf("decode: %v", jerr)
			return
		}
		if got.ExtendInfo == nil || got.ExtendInfo.CreditInformationRequestID == "" {
			t.Errorf("missing extendInfo.creditInformationRequestId on decoded body: %#v", got)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok","data":{"externalReferenceUid":"user-42","status":"PROCESSING","currency":"IDR"}}`))
	}))
	defer srv.Close()

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(pemForRSA(t, signKey)),
		atomefin.WithEncryptAtomePublicCertPEM(pubPEMForRSA(t, &atomeEncryptKey.PublicKey)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	resp, err := credit.New(c).SubmitApplication(context.Background(), validApplicationParam())
	if err != nil {
		t.Fatalf("SubmitApplication: %v", err)
	}
	if !resp.IsProcessing() {
		t.Errorf("expected PROCESSING, got %#v", resp.Data)
	}
}

// ---------- helpers (shared with other E2E tests) ----------

func pemForRSA(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

func pubPEMForRSA(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}
