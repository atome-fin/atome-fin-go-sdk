package encrypt

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
)

// MinKeyBits is the minimum RSA modulus size the wrap / unwrap
// helpers accept. Mirrors `sign.MinKeyBits` (= 2048) so the
// encrypt and signing paths can't drift out of step on the
// partner's "RSA-2048 only" baseline.
const MinKeyBits = 2048

// WrapAESKey RSA-PKCS#1 v1.5 encrypts the AES key bytes against
// atomePub and base64-std-encodes the ciphertext. Returns the
// string that goes inside the `symmetricKey=<...>` field of the
// `Encrypt:` header (after URL-escaping; see BuildEncryptHeader).
//
// Mirrors Java's `Cipher.getInstance("RSA")` default — RSA/ECB/
// PKCS1Padding. **NOT OAEP.** Q33 RESOLVED 2026-05-06.
func WrapAESKey(aesKey []byte, atomePub *rsa.PublicKey) (string, error) {
	if atomePub == nil {
		return "", fmt.Errorf("encrypt: nil RSA public key")
	}
	if atomePub.N.BitLen() < MinKeyBits {
		return "", fmt.Errorf("encrypt: RSA modulus %d < min %d bits", atomePub.N.BitLen(), MinKeyBits)
	}
	enc, err := rsa.EncryptPKCS1v15(rand.Reader, atomePub, aesKey)
	if err != nil {
		return "", fmt.Errorf("encrypt: RSA wrap: %w", err)
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

// UnwrapAESKey is the inverse of WrapAESKey. Used by partner-side
// callback decryption (Q31 leaves credit callbacks plaintext for
// now, but the surface is shipped so partners with custom
// decryption tooling can re-use it).
func UnwrapAESKey(wrappedB64 string, partnerPriv *rsa.PrivateKey) ([]byte, error) {
	if partnerPriv == nil {
		return nil, fmt.Errorf("encrypt: nil RSA private key")
	}
	if partnerPriv.N.BitLen() < MinKeyBits {
		return nil, fmt.Errorf("encrypt: RSA modulus %d < min %d bits", partnerPriv.N.BitLen(), MinKeyBits)
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(wrappedB64)
	if err != nil {
		return nil, fmt.Errorf("encrypt: base64-decode wrapped key: %w", err)
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, partnerPriv, cipherBytes)
	if err != nil {
		return nil, fmt.Errorf("encrypt: RSA unwrap: %w", err)
	}
	return plain, nil
}
