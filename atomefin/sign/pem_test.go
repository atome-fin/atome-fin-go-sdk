package sign

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

// pemEncode wraps DER bytes in a PEM block with the given type.
func pemEncode(blockType string, der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
}

func TestLoadPrivateKeyPEM_PKCS1(t *testing.T) {
	t.Parallel()

	key := genTestKey(t, 2048)
	der := x509.MarshalPKCS1PrivateKey(key)
	got, err := LoadPrivateKeyPEM(pemEncode("RSA PRIVATE KEY", der))
	if err != nil {
		t.Fatalf("LoadPrivateKeyPEM: %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Errorf("loaded modulus differs")
	}

	// Round-trip: load → sign → verify with same key's public half.
	signer, err := NewRSA2Signer(got)
	if err != nil {
		t.Fatalf("NewRSA2Signer: %v", err)
	}
	sig, err := signer.Sign(context.Background(), []byte("ping"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	verifier, err := NewRSA2Verifier(&key.PublicKey)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	if err := verifier.Verify(context.Background(), []byte("ping"), sig); err != nil {
		t.Errorf("round-trip verify: %v", err)
	}
}

func TestLoadPrivateKeyPEM_PKCS8(t *testing.T) {
	t.Parallel()

	key := genTestKey(t, 2048)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	got, err := LoadPrivateKeyPEM(pemEncode("PRIVATE KEY", der))
	if err != nil {
		t.Fatalf("LoadPrivateKeyPEM(PKCS8): %v", err)
	}
	if got.N.Cmp(key.N) != 0 {
		t.Errorf("loaded modulus differs")
	}
}

func TestLoadPrivateKeyPEM_RejectsTooShort(t *testing.T) {
	t.Parallel()

	short := genTestKey(t, 1024)
	der := x509.MarshalPKCS1PrivateKey(short)
	_, err := LoadPrivateKeyPEM(pemEncode("RSA PRIVATE KEY", der))
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("1024-bit key: want ErrInvalidKey, got %v", err)
	}
}

func TestLoadPrivateKeyPEM_RejectsNonRSAPKCS8(t *testing.T) {
	t.Parallel()

	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(ec)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	_, err = LoadPrivateKeyPEM(pemEncode("PRIVATE KEY", der))
	if !errors.Is(err, ErrPEM) {
		t.Errorf("ECDSA key: want ErrPEM, got %v", err)
	}
}

func TestLoadPrivateKeyPEM_RejectsEncrypted(t *testing.T) {
	t.Parallel()

	// PKCS#8 encrypted block type.
	enc := pemEncode("ENCRYPTED PRIVATE KEY", []byte{0x00})
	if _, err := LoadPrivateKeyPEM(enc); !errors.Is(err, ErrPEM) {
		t.Errorf("encrypted PKCS#8: want ErrPEM, got %v", err)
	}

	// Legacy PKCS#1 with DEK-Info header.
	pkcs1Encrypted := &pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: map[string]string{"DEK-Info": "AES-256-CBC,0000"},
		Bytes:   []byte{0x00},
	}
	if _, err := LoadPrivateKeyPEM(pem.EncodeToMemory(pkcs1Encrypted)); !errors.Is(err, ErrPEM) {
		t.Errorf("encrypted PKCS#1: want ErrPEM, got %v", err)
	}

	// Variadic password is not yet supported.
	if _, err := LoadPrivateKeyPEM([]byte("garbage"), []byte("pw")); !errors.Is(err, ErrPEM) {
		t.Errorf("password arg: want ErrPEM, got %v", err)
	}
}

func TestLoadPrivateKeyPEM_RejectsGarbage(t *testing.T) {
	t.Parallel()

	cases := map[string][]byte{
		"empty":          nil,
		"not pem":        []byte("hello"),
		"unknown block":  pemEncode("DH PARAMETERS", []byte{0x00}),
		"corrupt PKCS#1": pemEncode("RSA PRIVATE KEY", []byte{0x00, 0x01, 0x02}),
		"corrupt PKCS#8": pemEncode("PRIVATE KEY", []byte{0x00, 0x01, 0x02}),
	}
	for name, in := range cases {
		name, in := name, in
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadPrivateKeyPEM(in); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestLoadPublicCertPEM_Variants(t *testing.T) {
	t.Parallel()

	key := genTestKey(t, 2048)

	t.Run("CERTIFICATE", func(t *testing.T) {
		t.Parallel()
		cert := selfSignedCert(t, key)
		pub, err := LoadPublicCertPEM(pemEncode("CERTIFICATE", cert))
		if err != nil {
			t.Fatalf("LoadPublicCertPEM: %v", err)
		}
		if pub.N.Cmp(key.N) != 0 {
			t.Errorf("modulus differs")
		}
	})

	t.Run("PUBLIC KEY (PKIX)", func(t *testing.T) {
		t.Parallel()
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			t.Fatalf("MarshalPKIXPublicKey: %v", err)
		}
		pub, err := LoadPublicCertPEM(pemEncode("PUBLIC KEY", der))
		if err != nil {
			t.Fatalf("LoadPublicCertPEM: %v", err)
		}
		if pub.N.Cmp(key.N) != 0 {
			t.Errorf("modulus differs")
		}
	})

	t.Run("RSA PUBLIC KEY (PKCS#1)", func(t *testing.T) {
		t.Parallel()
		der := x509.MarshalPKCS1PublicKey(&key.PublicKey)
		pub, err := LoadPublicCertPEM(pemEncode("RSA PUBLIC KEY", der))
		if err != nil {
			t.Fatalf("LoadPublicCertPEM: %v", err)
		}
		if pub.N.Cmp(key.N) != 0 {
			t.Errorf("modulus differs")
		}
	})
}

func TestLoadPublicCertPEM_RejectsNonRSACert(t *testing.T) {
	t.Parallel()

	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa GenerateKey: %v", err)
	}
	cert := selfSignedCertECDSA(t, ec)
	if _, err := LoadPublicCertPEM(pemEncode("CERTIFICATE", cert)); !errors.Is(err, ErrPEM) {
		t.Errorf("ECDSA cert: want ErrPEM, got %v", err)
	}
}

func TestLoadPublicCertPEM_RejectsShortKey(t *testing.T) {
	t.Parallel()

	short := genTestKey(t, 1024)
	der := x509.MarshalPKCS1PublicKey(&short.PublicKey)
	_, err := LoadPublicCertPEM(pemEncode("RSA PUBLIC KEY", der))
	if !errors.Is(err, ErrInvalidKey) {
		t.Errorf("1024-bit pub: want ErrInvalidKey, got %v", err)
	}
}

func TestLoadPublicCertPEM_RejectsGarbage(t *testing.T) {
	t.Parallel()

	cases := map[string][]byte{
		"empty":         nil,
		"not pem":       []byte("hello"),
		"unknown block": pemEncode("DH PARAMETERS", []byte{0x00}),
		"corrupt cert":  pemEncode("CERTIFICATE", []byte{0x00}),
		"corrupt pkix":  pemEncode("PUBLIC KEY", []byte{0x00}),
	}
	for name, in := range cases {
		name, in := name, in
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := LoadPublicCertPEM(in); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

// selfSignedCert produces a tiny self-signed RSA X.509 certificate suitable
// only for unit tests.
func selfSignedCert(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}

func selfSignedCertECDSA(t *testing.T, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}
