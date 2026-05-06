package atomefin_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

// pemFromKey wraps an RSA private key in a PKCS#1 PEM block for
// the test setup.
func pemFromKey(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// pemFromPub wraps an RSA public key in a SPKI/PKIX PEM block.
func pemFromPub(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("x509.MarshalPKIXPublicKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestDoEncryptedSigned_HappyPath(t *testing.T) {
	// Atome's encrypt keypair (server-side). The SDK gets the
	// public half via WithEncryptAtomePublicCertPEM; the test
	// server holds the private half so it can decrypt the
	// inbound body and assert the SDK sent the right shape.
	atomeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey atome: %v", err)
	}
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey sign: %v", err)
	}

	const wantPlain = `{"requestId":"r-1","externalReferenceUid":"u-1","payload":"hello"}`

	var gotPath, gotMethod, gotEncryptHeader string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotEncryptHeader = r.Header.Get(encrypt.EncryptHeaderName)
		gotBody, _ = io.ReadAll(r.Body)

		// The SDK should have signed the encrypted body bytes.
		// Verify the signature against the partner's signing
		// public key (signKey.PublicKey).
		if r.Header.Get("Authorization") == "" {
			t.Errorf("missing Authorization header")
		}

		// Decrypt the inbound body (server-side validation that
		// the SDK encrypted the right bytes with the right key).
		plain, derr := encrypt.Unmarshal(gotEncryptHeader, string(gotBody), atomeKey)
		if derr != nil {
			t.Errorf("Unmarshal inbound body: %v", derr)
			w.WriteHeader(500)
			return
		}
		if string(plain) != wantPlain {
			t.Errorf("decrypted body = %q\n  want %q", plain, wantPlain)
		}

		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"ok"}`))
	}))
	defer srv.Close()

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(pemFromKey(t, signKey)),
		atomefin.WithEncryptAtomePublicCertPEM(pemFromPub(t, &atomeKey.PublicKey)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	resp, err := c.DoEncryptedSigned(context.Background(), http.MethodPost, "/credit-information", []byte(wantPlain))
	if err != nil {
		t.Fatalf("DoEncryptedSigned: %v", err)
	}
	if resp == nil || string(resp.Body) == "" {
		t.Fatalf("nil/empty response: %#v", resp)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q", gotMethod)
	}
	if gotPath != "/credit-information" {
		t.Errorf("path = %q", gotPath)
	}
	if !strings.HasPrefix(gotEncryptHeader, "symmetricKey=") {
		t.Errorf("Encrypt header = %q; want symmetricKey=… prefix", gotEncryptHeader)
	}
}

func TestDoEncryptedSigned_RejectsMissingEncryptOption(t *testing.T) {
	// Construct a Client without WithEncryptAtomePublicCertPEM —
	// must fail at the dial site, NO network attempt.
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server must NOT be reached when encrypt key is unconfigured")
	}))
	defer srv.Close()

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(pemFromKey(t, signKey)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	_, err = c.DoEncryptedSigned(context.Background(), http.MethodPost, "/credit-information", []byte(`{}`))
	if err == nil {
		t.Fatal("DoEncryptedSigned: want error, got nil")
	}
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if ve.Field != "encryptAtomePublicCert" {
		t.Errorf("Field = %q; want encryptAtomePublicCert", ve.Field)
	}
}

func TestDoEncryptedSigned_RejectsNonPOST(t *testing.T) {
	atomeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	c, err := atomefin.New(
		atomefin.WithBaseURL("http://example.invalid"),
		atomefin.WithPrivateKeyPEM(pemFromKey(t, signKey)),
		atomefin.WithEncryptAtomePublicCertPEM(pemFromPub(t, &atomeKey.PublicKey)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	for _, m := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		_, err := c.DoEncryptedSigned(context.Background(), m, "/credit-information", []byte(`{}`))
		if err == nil {
			t.Errorf("method %q: want error, got nil", m)
			continue
		}
		var ve *atomefin.ValidationError
		if !errors.As(err, &ve) {
			t.Errorf("method %q: err = %v; want *ValidationError", m, err)
		}
	}
}

func TestDoEncryptedSigned_NilClient(t *testing.T) {
	var c *atomefin.Client
	if _, err := c.DoEncryptedSigned(context.Background(), http.MethodPost, "/credit-information", []byte(`{}`)); err == nil {
		t.Error("nil receiver: want error, got nil")
	}
}
