package callback_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- Test helpers ----------

func mustKey(t testing.TB) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func mustSigner(t testing.TB, k *rsa.PrivateKey) sign.Signer {
	t.Helper()
	s, err := sign.NewRSA2Signer(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustVerifierFromKey(t testing.TB, pub *rsa.PublicKey) sign.Verifier {
	t.Helper()
	v, err := sign.NewRSA2Verifier(pub)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func mustCertPEM(t testing.TB, k *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// ---------- NewVerifier construction ----------

func TestNewVerifier_RejectsEmptySlice(t *testing.T) {
	_, err := callback.NewVerifier(nil)
	if err == nil {
		t.Error("expected error for empty verifiers slice")
	}
}

func TestNewVerifier_RejectsNilElement(t *testing.T) {
	k := mustKey(t)
	_, err := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &k.PublicKey), nil})
	if err == nil || !strings.Contains(err.Error(), "[1]") {
		t.Errorf("err = %v; want index-tagged nil error", err)
	}
}

func TestNewVerifier_DefaultBodyLimit(t *testing.T) {
	k := mustKey(t)
	v, err := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &k.PublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	if v.BodyLimit() != callback.DefaultBodyLimit {
		t.Errorf("BodyLimit = %d, want %d", v.BodyLimit(), callback.DefaultBodyLimit)
	}
	if v.KeyCount() != 1 {
		t.Errorf("KeyCount = %d, want 1", v.KeyCount())
	}
}

func TestNewVerifier_WithBodyLimit(t *testing.T) {
	k := mustKey(t)
	v, err := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &k.PublicKey)},
		callback.WithBodyLimit(2048),
	)
	if err != nil {
		t.Fatal(err)
	}
	if v.BodyLimit() != 2048 {
		t.Errorf("BodyLimit = %d, want 2048", v.BodyLimit())
	}
	// Non-positive values are silently ignored (defensive default).
	v2, _ := callback.NewVerifier(
		[]sign.Verifier{mustVerifierFromKey(t, &k.PublicKey)},
		callback.WithBodyLimit(0),
	)
	if v2.BodyLimit() != callback.DefaultBodyLimit {
		t.Errorf("zero BodyLimit override should be ignored; got %d", v2.BodyLimit())
	}
}

// ---------- FromClient / FromCertPEMs ----------

func TestFromClient_NoVerifier(t *testing.T) {
	priv := mustKey(t)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	c, err := atomefin.New(
		atomefin.WithPrivateKeyPEM(pemBytes),
		atomefin.WithBaseURL("https://x.example.com"),
		atomefin.WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := callback.FromClient(c); err == nil {
		t.Error("expected error when Client has no verifier configured")
	}
}

func TestFromClient_WithVerifier(t *testing.T) {
	priv := mustKey(t)
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	c, err := atomefin.New(
		atomefin.WithPrivateKeyPEM(pemBytes),
		atomefin.WithAtomePublicKey(&priv.PublicKey),
		atomefin.WithBaseURL("https://x.example.com"),
		atomefin.WithPartnerID("p"),
	)
	if err != nil {
		t.Fatal(err)
	}
	v, err := callback.FromClient(c)
	if err != nil {
		t.Fatal(err)
	}
	if v.KeyCount() != 1 {
		t.Errorf("KeyCount = %d", v.KeyCount())
	}
}

func TestFromCertPEMs_Multi(t *testing.T) {
	a := mustKey(t)
	b := mustKey(t)
	v, err := callback.FromCertPEMs([][]byte{mustCertPEM(t, a), mustCertPEM(t, b)})
	if err != nil {
		t.Fatal(err)
	}
	if v.KeyCount() != 2 {
		t.Errorf("KeyCount = %d, want 2", v.KeyCount())
	}
}

func TestFromCertPEMs_BadInput(t *testing.T) {
	if _, err := callback.FromCertPEMs(nil); err == nil {
		t.Error("expected error for nil PEM slice")
	}
	if _, err := callback.FromCertPEMs([][]byte{[]byte("not pem")}); err == nil {
		t.Error("expected error for non-PEM input")
	}
}

// ---------- Verify happy path ----------

func TestVerify_HappyPath(t *testing.T) {
	priv := mustKey(t)
	signer := mustSigner(t, priv)
	v, err := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)})
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"code":"SUCCESS","message":"ok"}`)
	sig, err := signer.Sign(context.Background(), body)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Verify(context.Background(), body, sig); err != nil {
		t.Errorf("happy-path verify failed: %v", err)
	}
}

func TestVerify_RejectsTamperedBody(t *testing.T) {
	priv := mustKey(t)
	signer := mustSigner(t, priv)
	v, _ := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)})

	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := signer.Sign(context.Background(), body)

	tampered := []byte(`{"code":"FAILED"}`)
	err := v.Verify(context.Background(), tampered, sig)
	if !errors.Is(err, sign.ErrSignature) {
		t.Errorf("err = %v; want sign.ErrSignature", err)
	}
}

func TestVerify_RejectsTamperedSignature(t *testing.T) {
	priv := mustKey(t)
	v, _ := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)})

	body := []byte(`{"code":"SUCCESS"}`)
	// A garbled signature must reject — should be sign.ErrSignature wrapped.
	err := v.Verify(context.Background(), body, "AAAAAAAA==")
	if !errors.Is(err, sign.ErrSignature) {
		t.Errorf("err = %v; want sign.ErrSignature", err)
	}
}

func TestVerify_RejectsEmptySignature(t *testing.T) {
	priv := mustKey(t)
	v, _ := callback.NewVerifier([]sign.Verifier{mustVerifierFromKey(t, &priv.PublicKey)})
	err := v.Verify(context.Background(), []byte("body"), "")
	if !errors.Is(err, sign.ErrSignature) {
		t.Errorf("err = %v; want sign.ErrSignature for empty signature", err)
	}
}

// ---------- Multi-cert fall-through ----------

func TestVerify_MultiCert_FirstKeyMatches(t *testing.T) {
	a := mustKey(t)
	b := mustKey(t)
	signer := mustSigner(t, a) // sign with A

	v, err := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &a.PublicKey),
		mustVerifierFromKey(t, &b.PublicKey),
	})
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := signer.Sign(context.Background(), body)

	if err := v.Verify(context.Background(), body, sig); err != nil {
		t.Errorf("expected accept (key A in slot), got %v", err)
	}
}

func TestVerify_MultiCert_SecondKeyMatches(t *testing.T) {
	a := mustKey(t)
	b := mustKey(t)
	// Sign with B, but list A first; second key in slot must verify.
	signer := mustSigner(t, b)

	v, _ := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &a.PublicKey),
		mustVerifierFromKey(t, &b.PublicKey),
	})

	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := signer.Sign(context.Background(), body)

	if err := v.Verify(context.Background(), body, sig); err != nil {
		t.Errorf("multi-cert fall-through failed: %v", err)
	}
}

func TestVerify_MultiCert_NeitherKeyMatches(t *testing.T) {
	a := mustKey(t)
	b := mustKey(t)
	c := mustKey(t)
	// Sign with C; only A and B in slot → reject.
	signer := mustSigner(t, c)

	v, _ := callback.NewVerifier([]sign.Verifier{
		mustVerifierFromKey(t, &a.PublicKey),
		mustVerifierFromKey(t, &b.PublicKey),
	})

	body := []byte(`{"code":"SUCCESS"}`)
	sig, _ := signer.Sign(context.Background(), body)

	err := v.Verify(context.Background(), body, sig)
	if err == nil {
		t.Error("expected rejection when no configured key matches")
	}
	if !errors.Is(err, sign.ErrSignature) {
		t.Errorf("err = %v; want sign.ErrSignature", err)
	}
}

// ---------- Defensive ----------

func TestVerify_NilReceiver(t *testing.T) {
	var v *callback.Verifier
	if err := v.Verify(context.Background(), nil, "sig"); err == nil {
		t.Error("nil Verifier must reject")
	}
	if v.BodyLimit() != 0 {
		t.Error("nil Verifier BodyLimit must be 0")
	}
	if v.KeyCount() != 0 {
		t.Error("nil Verifier KeyCount must be 0")
	}
}
