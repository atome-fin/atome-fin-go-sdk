// Package sign implements the RSA-2048 signing primitives required by the
// atomefin white-label "G" Auth-Capture-Void API.
//
// Per DESIGN.md §4 the signing helper provides:
//   - A Signer over RSA-2048 keys producing base64-standard-encoded signatures
//     placed verbatim in the Authorization header.
//   - A Verifier that mirrors the Signer for callback handlers and for sanity
//     checking sync error envelopes.
//   - Canonical-string helpers (POST: raw body; GET: sorted RFC 3986 query).
//   - PEM loaders for partner private keys and Atome public certs.
//
// The default scheme is RSASSA-PKCS#1 v1.5 over SHA-256 ("RSA2" in the spec
// wording). RSA-PSS is gated behind WithSaltedPSS / WithVerifierSaltedPSS and
// requires a separate cert exchange (DESIGN.md §10 Q4 — open).
package sign

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// MinKeyBits is the minimum RSA modulus size accepted by this package.
//
// The spec mandates RSA-2048; we reject smaller keys at construction time
// rather than at sign time so configuration errors fail fast.
const MinKeyBits = 2048

// ErrInvalidKey is returned when an RSA key is missing or below MinKeyBits.
var ErrInvalidKey = errors.New("sign: invalid RSA key")

// Signer signs canonical bytes and returns a base64-standard-encoded
// signature suitable for the Authorization header.
//
// Implementations must be safe for concurrent use.
type Signer interface {
	// Sign hashes canonical with SHA-256 and signs the digest with the
	// configured RSA key. The returned string is base64-standard encoded
	// (no URL-safe substitution, no padding stripping) so it can be placed
	// verbatim into the Authorization header.
	Sign(ctx context.Context, canonical []byte) (signature string, err error)

	// KeyID returns an optional key identifier surfaced to the caller so it
	// can be transmitted alongside the signature once the partner adopts a
	// keyId/keyVersion convention (DESIGN.md §10 Q3 — open). Implementations
	// may return "" when no key identifier is configured.
	KeyID() string
}

// SignerOption configures a Signer constructed via NewRSA2Signer.
type SignerOption func(*rsa2Signer) error

// WithKeyID sets the value returned by Signer.KeyID().
//
// The SDK does not currently inject a key-id header on its own — wire-format
// for key rotation is an open question (DESIGN.md §10 Q3). Setting it here
// future-proofs the caller's configuration.
func WithKeyID(id string) SignerOption {
	return func(s *rsa2Signer) error {
		s.keyID = id
		return nil
	}
}

// rsa2Signer is the default RSA-PKCS#1-v1.5 / RSA-PSS implementation. It is
// immutable after construction and therefore safe for concurrent use.
type rsa2Signer struct {
	priv    *rsa.PrivateKey
	keyID   string
	pss     bool // toggled by WithSaltedPSS in salt.go
	saltLen int
}

// NewRSA2Signer constructs a Signer that signs SHA-256 digests with an
// RSA-2048-or-larger private key. The default scheme is RSASSA-PKCS#1 v1.5;
// callers can opt into RSA-PSS via WithSaltedPSS.
func NewRSA2Signer(priv *rsa.PrivateKey, opts ...SignerOption) (Signer, error) {
	if priv == nil {
		return nil, fmt.Errorf("%w: nil private key", ErrInvalidKey)
	}
	if priv.N == nil || priv.N.BitLen() < MinKeyBits {
		bits := 0
		if priv.N != nil {
			bits = priv.N.BitLen()
		}
		return nil, fmt.Errorf("%w: modulus %d bits, need >= %d",
			ErrInvalidKey, bits, MinKeyBits)
	}
	s := &rsa2Signer{priv: priv}
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *rsa2Signer) Sign(ctx context.Context, canonical []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	var (
		sig []byte
		err error
	)
	if s.pss {
		sig, err = rsa.SignPSS(rand.Reader, s.priv, crypto.SHA256, sum[:],
			&rsa.PSSOptions{SaltLength: s.saltLen, Hash: crypto.SHA256})
	} else {
		sig, err = rsa.SignPKCS1v15(rand.Reader, s.priv, crypto.SHA256, sum[:])
	}
	if err != nil {
		return "", fmt.Errorf("sign: rsa sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

func (s *rsa2Signer) KeyID() string { return s.keyID }
