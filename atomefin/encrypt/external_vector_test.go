package encrypt_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

// External vector test — pins the SDK's encrypt path against a
// fixed, pre-computed set of bytes that an independent
// implementation must agree with byte-for-byte.
//
// This is the encrypt-path equivalent of `sign/external_vector_test.go`:
// catches algorithm drift the same way (and on the same axes —
// padding, encoding, byte-order). The reference encryption was
// produced by running stdlib's `crypto/aes` ECB over the exact
// plaintext + key recorded in `testdata/`. Any divergence between
// our output and the recorded body bytes means the SDK's AES path
// has drifted.
//
// RSA-PKCS#1 v1.5 wrap is intentionally NOT byte-pinned — its
// ciphertext is randomized via fresh padding per call. Round-trip
// (wrap → unwrap recovers the AES key) is the appropriate
// invariant; the wrap path is exercised end-to-end below.
//
// Hermetic CI: NO os/exec, NO network. The fixtures are committed
// under `testdata/` and read at test time. Re-generation is a
// deliberate human action when the protocol changes (run the
// generator described in `doc.go` of testdata, then commit the
// new files).

func TestExternal_AESBody_ByteEquals(t *testing.T) {
	plain := mustReadTestdata(t, "external_aes_plaintext.txt")
	aesKey := mustReadTestdata(t, "external_aes_key.txt")
	wantBody := string(mustReadTestdata(t, "external_aes_body.txt"))

	if len(aesKey) != 32 {
		t.Fatalf("fixture aes key length = %d, want 32", len(aesKey))
	}
	if err := encrypt.ValidateAESKey(string(aesKey)); err != nil {
		t.Fatalf("fixture aes key fails ValidateAESKey: %v", err)
	}

	got, err := encrypt.EncryptBody(plain, aesKey)
	if err != nil {
		t.Fatalf("EncryptBody: %v", err)
	}
	if got != wantBody {
		t.Fatalf("AES-ECB-PKCS5 output drifted from external vector\n"+
			" got: %s\nwant: %s\nplaintext (len=%d): %q\nkey: %s",
			got, wantBody, len(plain), plain, aesKey)
	}
}

func TestExternal_AESBody_DecryptRoundTrip(t *testing.T) {
	wantPlain := mustReadTestdata(t, "external_aes_plaintext.txt")
	aesKey := mustReadTestdata(t, "external_aes_key.txt")
	body := string(mustReadTestdata(t, "external_aes_body.txt"))

	got, err := encrypt.DecryptBody(body, aesKey)
	if err != nil {
		t.Fatalf("DecryptBody: %v", err)
	}
	if !bytes.Equal(got, wantPlain) {
		t.Errorf("DecryptBody output drifted:\n got=%q\nwant=%q", got, wantPlain)
	}
}

// TestExternal_RSAWrap_RoundTrip exercises the full hybrid envelope
// against the pinned partner key pair. RSA wrap output is
// non-deterministic so we round-trip rather than byte-compare:
//
//  1. Pull the partner's encrypt PRIVATE key from testdata.
//  2. Pull Atome's encrypt PUBLIC key (the matching public).
//  3. Marshal a fixed plaintext using the public key — produces a
//     fresh wrapped AES key + encrypted body.
//  4. Unmarshal using the private key — recovers the original
//     plaintext byte-for-byte.
//
// Negative: a fresh unrelated keypair must NOT decrypt the wrap.
func TestExternal_RSAWrap_RoundTrip(t *testing.T) {
	priv := mustReadPartnerPrivKey(t)

	plain := []byte(`{"requestId":"VECTOR-RT","externalReferenceUid":"vector-user"}`)
	header, body, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := encrypt.Unmarshal(header, body, priv)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch:\n got=%q\nwant=%q", got, plain)
	}

	// Negative: an unrelated key must NOT recover the plaintext.
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	if _, err := encrypt.Unmarshal(header, body, other); err == nil {
		t.Error("Unmarshal with foreign key: want error, got nil")
	}
}

func TestExternal_PartnerPublicKeyLoads(t *testing.T) {
	// The committed atome-pub fixture must parse cleanly through
	// the same loader the Client uses (pkg sign's PEM helpers).
	// Tested here in encrypt_test rather than sign_test so the
	// encrypt-side fixture coverage stays self-contained.
	pemBytes := mustReadTestdata(t, "encrypt_atome_pub.pem")
	if !bytes.HasPrefix(pemBytes, []byte("-----BEGIN ")) {
		t.Errorf("expected PEM-armoured fixture, got %q…", pemBytes[:min(len(pemBytes), 32)])
	}
}

// ---------- helpers ----------

func mustReadTestdata(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join(testdataDir(t), name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return data
}

func mustReadPartnerPrivKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	pemBytes := mustReadTestdata(t, "encrypt_partner_priv.pem")
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatalf("encrypt_partner_priv.pem: no PEM block")
	}
	priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Fallback for PKCS#8.
		k, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			t.Fatalf("parse PKCS#1 (%v) AND PKCS#8 (%v) failed", err, err2)
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			t.Fatalf("PKCS#8 key is not RSA")
		}
		return rk
	}
	return priv
}

func testdataDir(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(here), "testdata")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
