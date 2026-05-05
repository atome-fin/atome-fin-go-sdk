package sign

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// ErrPEM is returned when a PEM payload cannot be parsed.
var ErrPEM = errors.New("sign: invalid PEM")

// LoadPrivateKeyPEM parses an RSA private key from PEM-encoded data.
//
// Supported block types:
//   - "RSA PRIVATE KEY" — PKCS#1 RSAPrivateKey.
//   - "PRIVATE KEY"     — PKCS#8 PrivateKeyInfo wrapping an RSA key.
//
// Encrypted PEM ("ENCRYPTED PRIVATE KEY", or PKCS#1 with DEK-Info headers) is
// rejected with a typed error so callers can prompt for credentials at a
// higher layer. The variadic password parameter is reserved: passing a
// non-empty password today returns ErrPEM. Encrypted-key support is an open
// item tracked alongside DESIGN.md §10 Q3 (key rotation).
//
// On success the returned key is validated to be at least MinKeyBits.
func LoadPrivateKeyPEM(data []byte, password ...[]byte) (*rsa.PrivateKey, error) {
	for _, p := range password {
		if len(p) > 0 {
			return nil, fmt.Errorf("%w: encrypted PEM not yet supported", ErrPEM)
		}
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block found", ErrPEM)
	}
	// PKCS#1 with DEK-Info is encrypted; we don't support it.
	if _, encrypted := block.Headers["DEK-Info"]; encrypted {
		return nil, fmt.Errorf("%w: encrypted PKCS#1 not supported", ErrPEM)
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: PKCS#1: %v", ErrPEM, err)
		}
		key = k
	case "PRIVATE KEY":
		anyKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: PKCS#8: %v", ErrPEM, err)
		}
		rsaKey, ok := anyKey.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%w: PKCS#8 key is not RSA (got %T)",
				ErrPEM, anyKey)
		}
		key = rsaKey
	case "ENCRYPTED PRIVATE KEY":
		return nil, fmt.Errorf("%w: encrypted PKCS#8 not supported", ErrPEM)
	default:
		return nil, fmt.Errorf("%w: unsupported block type %q", ErrPEM, block.Type)
	}

	if key.N == nil || key.N.BitLen() < MinKeyBits {
		bits := 0
		if key.N != nil {
			bits = key.N.BitLen()
		}
		return nil, fmt.Errorf("%w: modulus %d bits, need >= %d",
			ErrInvalidKey, bits, MinKeyBits)
	}
	return key, nil
}

// LoadPublicCertPEM parses an RSA public key from a PEM-encoded payload.
//
// Supported block types:
//   - "CERTIFICATE"     — X.509 certificate (the spec exchange format).
//   - "PUBLIC KEY"      — SubjectPublicKeyInfo (PKIX).
//   - "RSA PUBLIC KEY"  — PKCS#1 RSAPublicKey (uncommon but tolerated).
//
// On success the returned key is validated to be at least MinKeyBits. Cert
// validity periods are NOT enforced here — the SDK signs/verifies at request
// time and a separate channel (cert exchange / partner ops) is responsible
// for rotating expiring certs (DESIGN.md §10 Q3).
func LoadPublicCertPEM(data []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%w: no PEM block found", ErrPEM)
	}
	var key *rsa.PublicKey
	switch block.Type {
	case "CERTIFICATE":
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: certificate: %v", ErrPEM, err)
		}
		rsaKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: certificate public key is not RSA (got %T)",
				ErrPEM, cert.PublicKey)
		}
		key = rsaKey
	case "PUBLIC KEY":
		anyKey, err := x509.ParsePKIXPublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: PKIX: %v", ErrPEM, err)
		}
		rsaKey, ok := anyKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("%w: PKIX public key is not RSA (got %T)",
				ErrPEM, anyKey)
		}
		key = rsaKey
	case "RSA PUBLIC KEY":
		k, err := x509.ParsePKCS1PublicKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: PKCS#1: %v", ErrPEM, err)
		}
		key = k
	default:
		return nil, fmt.Errorf("%w: unsupported block type %q", ErrPEM, block.Type)
	}

	if key.N == nil || key.N.BitLen() < MinKeyBits {
		bits := 0
		if key.N != nil {
			bits = key.N.BitLen()
		}
		return nil, fmt.Errorf("%w: modulus %d bits, need >= %d",
			ErrInvalidKey, bits, MinKeyBits)
	}
	return key, nil
}
