package sign

import (
	"context"
	"testing"
)

// TestOVOHMAC_GrabVector pins the implementation to the self-test
// vector from the Grab gateway test suite
// (docs/grab/credit-application-hmac-signing.md §4). Any change to
// key derivation, concatenation order, base64 padding, or hex casing
// must fail here.
func TestOVOHMAC_GrabVector(t *testing.T) {
	const (
		clientID = "test"
		secret   = "test123"
		timeMs   = "1717671690611"
		method   = "POST"
		path     = "/path/abc"
		body     = "request_body1"
		wantSig  = "1bbe847a37eff8b004f607672f7a6747d1ff32f7db47748fc7c27f9ee84c71df"
	)

	canonical := OVOHMACCanonical(clientID, timeMs, method, path, []byte(body))

	signer, err := NewOVOHMACSigner(secret)
	if err != nil {
		t.Fatalf("NewOVOHMACSigner: %v", err)
	}
	got, err := signer.Sign(context.Background(), canonical)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if got != wantSig {
		t.Fatalf("signature mismatch:\n got %s\nwant %s", got, wantSig)
	}

	verifier, err := NewOVOHMACVerifier(secret)
	if err != nil {
		t.Fatalf("NewOVOHMACVerifier: %v", err)
	}
	if err := verifier.Verify(context.Background(), canonical, wantSig); err != nil {
		t.Fatalf("Verify(valid): %v", err)
	}
}

func TestOVOHMAC_VerifyRejects(t *testing.T) {
	canonical := OVOHMACCanonical("test", "1717671690611", "POST", "/path/abc", []byte("request_body1"))
	v, err := NewOVOHMACVerifier("test123")
	if err != nil {
		t.Fatalf("NewOVOHMACVerifier: %v", err)
	}
	ctx := context.Background()

	if err := v.Verify(ctx, canonical, ""); err == nil {
		t.Fatal("empty signature accepted")
	}
	// Correct MAC but uppercase hex must be rejected (spec mandates lowercase).
	if err := v.Verify(ctx, canonical, "1BBE847A37EFF8B004F607672F7A6747D1FF32F7DB47748FC7C27F9EE84C71DF"); err == nil {
		t.Fatal("uppercase hex signature accepted")
	}
	// Tampered body.
	tampered := OVOHMACCanonical("test", "1717671690611", "POST", "/path/abc", []byte("request_body2"))
	if err := v.Verify(ctx, tampered, "1bbe847a37eff8b004f607672f7a6747d1ff32f7db47748fc7c27f9ee84c71df"); err == nil {
		t.Fatal("tampered canonical accepted")
	}
	// Wrong secret.
	v2, err := NewOVOHMACVerifier("other-secret")
	if err != nil {
		t.Fatalf("NewOVOHMACVerifier: %v", err)
	}
	if err := v2.Verify(ctx, canonical, "1bbe847a37eff8b004f607672f7a6747d1ff32f7db47748fc7c27f9ee84c71df"); err == nil {
		t.Fatal("wrong-secret signature accepted")
	}
}

func TestOVOHMAC_EmptySecretRejected(t *testing.T) {
	if _, err := NewOVOHMACSigner(""); err == nil {
		t.Fatal("NewOVOHMACSigner(\"\") accepted")
	}
	if _, err := NewOVOHMACVerifier(""); err == nil {
		t.Fatal("NewOVOHMACVerifier(\"\") accepted")
	}
}

// TestOVOHMAC_GETEmptyBody pins the GET branch: body is empty, its
// base64 segment is the empty string (doc §2).
func TestOVOHMAC_GETEmptyBody(t *testing.T) {
	got := OVOHMACCanonical("cid", "123", "GET", "/q?a=1", nil)
	want := "cid123GET /q?a=1"
	if string(got) != want {
		t.Fatalf("canonical mismatch:\n got %q\nwant %q", got, want)
	}
}
