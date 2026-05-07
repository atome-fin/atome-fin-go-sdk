package sign

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrSignature is returned when a signature fails to verify. Callers should
// treat all verification failures as opaque: do not branch on the wrapped
// error to avoid leaking distinguishing information back to a caller.
var ErrSignature = errors.New("sign: signature verification failed")

// Verifier verifies a base64-standard-encoded RSA signature against canonical
// bytes. Implementations must be safe for concurrent use.
type Verifier interface {
	Verify(ctx context.Context, canonical []byte, signature string) error
}

// VerifierOption configures a Verifier constructed via NewRSA2Verifier.
type VerifierOption func(*rsa2Verifier) error

// rsa2Verifier mirrors rsa2Signer for the receive path (callbacks + sync
// error-envelope sanity checks). RSASSA-PKCS#1 v1.5 over SHA-256.
type rsa2Verifier struct {
	pub *rsa.PublicKey
}

// NewRSA2Verifier constructs a Verifier over an RSA-2048-or-larger public
// key using RSASSA-PKCS#1 v1.5 verification — the only scheme the Atome
// gateway uses (the v0.6.x `WithVerifierSaltedPSS` opt-in was removed in
// v0.7.0).
func NewRSA2Verifier(pub *rsa.PublicKey, opts ...VerifierOption) (Verifier, error) {
	if pub == nil {
		return nil, fmt.Errorf("%w: nil public key", ErrInvalidKey)
	}
	if pub.N == nil || pub.N.BitLen() < MinKeyBits {
		bits := 0
		if pub.N != nil {
			bits = pub.N.BitLen()
		}
		return nil, fmt.Errorf("%w: modulus %d bits, need >= %d",
			ErrInvalidKey, bits, MinKeyBits)
	}
	v := &rsa2Verifier{pub: pub}
	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}
	return v, nil
}

func (v *rsa2Verifier) Verify(ctx context.Context, canonical []byte, signature string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if signature == "" {
		return fmt.Errorf("%w: empty signature", ErrSignature)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("%w: base64 decode: %v", ErrSignature, err)
	}
	sum := sha256.Sum256(canonical)
	if err := rsa.VerifyPKCS1v15(v.pub, crypto.SHA256, sum[:], sig); err != nil {
		return fmt.Errorf("%w: %v", ErrSignature, err)
	}
	return nil
}
