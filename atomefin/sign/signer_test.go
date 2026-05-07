package sign

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// genTestKey creates an RSA-2048 key once per test invocation.
//
// Generation is the slowest part of these tests; we cache by key-bits so the
// happy-path tests can share a key.
func genTestKey(tb testing.TB, bits int) *rsa.PrivateKey {
	tb.Helper()
	key, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		tb.Fatalf("rsa.GenerateKey(%d): %v", bits, err)
	}
	return key
}

func TestNewRSA2Signer_RejectsBadKeys(t *testing.T) {
	t.Parallel()

	if _, err := NewRSA2Signer(nil); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("nil key: want ErrInvalidKey, got %v", err)
	}

	short := genTestKey(t, 1024)
	if _, err := NewRSA2Signer(short); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("1024-bit key: want ErrInvalidKey, got %v", err)
	}
}

func TestRSA2Signer_PKCS1v15_RoundTrip(t *testing.T) {
	t.Parallel()

	key := genTestKey(t, 2048)
	signer, err := NewRSA2Signer(key, WithKeyID("partner-2026-01"))
	if err != nil {
		t.Fatalf("NewRSA2Signer: %v", err)
	}
	if got, want := signer.KeyID(), "partner-2026-01"; got != want {
		t.Errorf("KeyID = %q, want %q", got, want)
	}

	canonical := []byte(`{"requestId":"R1","totalAmount":12345}`)
	sig, err := signer.Sign(context.Background(), canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if sig == "" {
		t.Fatal("empty signature")
	}
	if _, dErr := base64.StdEncoding.DecodeString(sig); dErr != nil {
		t.Errorf("signature is not std base64: %v", dErr)
	}

	verifier, err := NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	if err := verifier.Verify(context.Background(), canonical, sig); err != nil {
		t.Errorf("Verify happy: %v", err)
	}

	// Tamper detection.
	tampered := append([]byte(nil), canonical...)
	tampered[len(tampered)-1] ^= 0x01
	if err := verifier.Verify(context.Background(), tampered, sig); !errors.Is(err, ErrSignature) {
		t.Errorf("Verify tampered canonical: want ErrSignature, got %v", err)
	}

	if err := verifier.Verify(context.Background(), canonical, sig+"=="); !errors.Is(err, ErrSignature) {
		t.Errorf("Verify mangled signature: want ErrSignature, got %v", err)
	}
	if err := verifier.Verify(context.Background(), canonical, "not_base64!"); !errors.Is(err, ErrSignature) {
		t.Errorf("Verify non-base64 signature: want ErrSignature, got %v", err)
	}
	if err := verifier.Verify(context.Background(), canonical, ""); !errors.Is(err, ErrSignature) {
		t.Errorf("Verify empty signature: want ErrSignature, got %v", err)
	}
}

// TestRSA2Signer_PSS_RoundTrip was removed in v0.7.0 along with the
// PSS scaffolding. The Atome gateway only supports PKCS#1 v1.5 — see
// CHANGELOG `## [0.7.0]` and atomefin/sign/signer.go's package doc
// for the history.

func TestSign_RespectsContextCancellation(t *testing.T) {
	t.Parallel()

	key := genTestKey(t, 2048)
	signer, err := NewRSA2Signer(key)
	if err != nil {
		t.Fatalf("NewRSA2Signer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, sErr := signer.Sign(ctx, []byte("hi")); !errors.Is(sErr, context.Canceled) {
		t.Errorf("Sign(canceled ctx): want Canceled, got %v", sErr)
	}

	verifier, err := NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	// With canceled ctx the verifier returns context.Canceled before doing
	// any crypto — a permissive signature value is fine.
	if err := verifier.Verify(ctx, []byte("hi"), strings.Repeat("A", 4)); !errors.Is(err, context.Canceled) {
		t.Errorf("Verify(canceled ctx): want Canceled, got %v", err)
	}
}

func TestRSA2Verifier_RejectsShortKey(t *testing.T) {
	t.Parallel()

	if _, err := NewRSA2Verifier(nil); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("nil pub: want ErrInvalidKey, got %v", err)
	}
	short := genTestKey(t, 1024)
	if _, err := NewRSA2Verifier(&short.PublicKey); !errors.Is(err, ErrInvalidKey) {
		t.Errorf("1024-bit pub: want ErrInvalidKey, got %v", err)
	}
}
