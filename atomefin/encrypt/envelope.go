package encrypt

import (
	"crypto/rsa"
	"fmt"
)

// Marshal is the high-level outbound envelope: generate a fresh
// AES-256 key, encrypt the plaintext body, RSA-wrap the AES key
// against atomePub, and return the (header, base64Body) pair the
// caller dispatches.
//
//	header, bodyB64, err := encrypt.Marshal(plain, atomePub)
//	req.Header.Set(encrypt.EncryptHeaderName, header)
//	authorization := signer.Sign([]byte(bodyB64))   // sign the encrypted body
//	req.Body = strings.NewReader(bodyB64)
//
// One call. The fresh AES key never escapes this function — for
// per-test determinism use the lower-level building blocks
// (RandomAESKey + EncryptBody + WrapAESKey + BuildEncryptHeader)
// directly.
//
// Errors:
//   - atomePub nil or < 2048 bits → wrap error
//   - rand.Read failure (extremely rare) → key-generation error
//   - underlying AES / RSA / encoding errors propagate verbatim
func Marshal(plain []byte, atomePub *rsa.PublicKey) (header, bodyB64 string, err error) {
	if atomePub == nil {
		return "", "", fmt.Errorf("encrypt: nil RSA public key")
	}
	aesKey, err := RandomAESKey()
	if err != nil {
		return "", "", err
	}
	bodyB64, err = EncryptBody(plain, []byte(aesKey))
	if err != nil {
		return "", "", err
	}
	wrappedB64, err := WrapAESKey([]byte(aesKey), atomePub)
	if err != nil {
		return "", "", err
	}
	return BuildEncryptHeader(wrappedB64), bodyB64, nil
}

// Unmarshal is the inverse of Marshal: parses the Encrypt header,
// unwraps the AES key, validates the alphabet, and AES-decrypts +
// strips PKCS#5 padding from the body. Used by partner-side
// callback decryption (forward-compat — Q31 leaves credit
// callbacks plaintext for now).
//
// Returns the plaintext body bytes on success.
func Unmarshal(header, bodyB64 string, partnerPriv *rsa.PrivateKey) ([]byte, error) {
	if partnerPriv == nil {
		return nil, fmt.Errorf("encrypt: nil RSA private key")
	}
	kv, err := ParseEncryptHeader(header)
	if err != nil {
		return nil, err
	}
	wrappedB64, err := SymmetricKeyFrom(kv)
	if err != nil {
		return nil, err
	}
	aesKey, err := UnwrapAESKey(wrappedB64, partnerPriv)
	if err != nil {
		return nil, err
	}
	if err := ValidateAESKey(string(aesKey)); err != nil {
		return nil, err
	}
	return DecryptBody(bodyB64, aesKey)
}
