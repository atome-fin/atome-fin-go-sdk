package encrypt_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	plain := []byte(`{"requestId":"r-1","externalReferenceUid":"u-1","data":"hello"}`)

	header, bodyB64, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.HasPrefix(header, "symmetricKey=") {
		t.Errorf("header = %q; want symmetricKey=… prefix", header)
	}
	if bodyB64 == "" {
		t.Errorf("bodyB64 is empty")
	}

	got, err := encrypt.Unmarshal(header, bodyB64, priv)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("round-trip mismatch:\n got=%q\nwant=%q", got, plain)
	}
}

func TestMarshal_RejectsNilKey(t *testing.T) {
	if _, _, err := encrypt.Marshal([]byte("x"), nil); err == nil {
		t.Error("Marshal nil-key: want error, got nil")
	}
}

func TestUnmarshal_RejectsNilKey(t *testing.T) {
	if _, err := encrypt.Unmarshal("symmetricKey=AAAA", "AAAA", nil); err == nil {
		t.Error("Unmarshal nil-priv: want error, got nil")
	}
}

func TestUnmarshal_RejectsBadHeader(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	if _, err := encrypt.Unmarshal("malformed", "AAAA", priv); err == nil {
		t.Error("Unmarshal bad-header: want error, got nil")
	}
}

func TestUnmarshal_RejectsTamperedKey(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	plain := []byte("hello world")

	header, body, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Mutate the wrapped key in the header — flip a base64 char in
	// the symmetricKey value.
	tampered := []byte(header)
	for i := range tampered {
		if tampered[i] == 'A' {
			tampered[i] = 'B'
			break
		}
	}
	if _, err := encrypt.Unmarshal(string(tampered), body, priv); err == nil {
		t.Error("Unmarshal tampered-key: want error, got nil")
	}
}

func TestUnmarshal_RejectsTamperedBody(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	plain := []byte("hello world")
	header, body, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Mutate the LAST block of the body so PKCS#5 unpadding fails.
	mutated := []byte(body)
	mutated[len(mutated)-2] = 'X' // flip a near-tail byte

	_, err = encrypt.Unmarshal(header, string(mutated), priv)
	if err == nil {
		t.Error("Unmarshal tampered-body: want error, got nil")
	}
}

func TestMarshal_FreshKeyPerCall(t *testing.T) {
	// Marshal generates a NEW AES key on every call. Two
	// invocations with identical (plain, atomePub) inputs produce
	// distinct ciphertexts (and headers) — pin that property so a
	// future regression that caches the key is caught.
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	plain := []byte("identical payload")

	h1, b1, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal #1: %v", err)
	}
	h2, b2, err := encrypt.Marshal(plain, &priv.PublicKey)
	if err != nil {
		t.Fatalf("Marshal #2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("two Marshal calls produced identical headers — AES key not refreshed:\n%q", h1)
	}
	if b1 == b2 {
		t.Errorf("two Marshal calls produced identical bodies — AES key not refreshed:\n%q", b1)
	}
}
