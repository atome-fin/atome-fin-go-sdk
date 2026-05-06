package encrypt

// AES-ECB-PKCS5 — partner-protocol-mandated, NOT a Go-side
// preference.
//
// Go's crypto/cipher deliberately omits an ECB BlockMode because
// ECB leaks plaintext patterns at block boundaries and the Go
// authors consider it dangerous for general use. The apaylater
// partner protocol nonetheless requires ECB on this code path
// (Q32 RESOLVED 2026-05-06; mirrors Java's
// `Cipher.getInstance("AES/ECB/PKCS5Padding")`). This file walks
// the block cipher block-by-block via aes.NewCipher's Encrypt /
// Decrypt to implement the mandated mode without contradicting
// Go's standard library design.
//
// Alternatives like AES-CBC or AES-GCM are NOT options here —
// the upstream gateway only decrypts ECB. If the protocol ever
// gains an IV (e.g. moving to CBC), header.go's
// `map[string]string` shape leaves room.

import (
	"bytes"
	"crypto/aes"
	"encoding/base64"
	"fmt"
)

// aesKeyBytes is the strict AES-256 key length the protocol
// requires. Reused by EncryptBody / DecryptBody / ValidateAESKey.
const aesKeyBytes = 32

// EncryptBody AES-ECB-PKCS5-encrypts plain with key, then base64
// (std-encoding) the ciphertext. key must be exactly 32 bytes;
// PKCS#5 padding is appended before encryption (with AES's
// 16-byte block, PKCS#5 ≡ PKCS#7).
//
// Returns the base64 string the partner sends as the HTTP body —
// the same bytes the signer signs.
func EncryptBody(plain, key []byte) (string, error) {
	if len(key) != aesKeyBytes {
		return "", fmt.Errorf("encrypt: AES key length must be %d bytes, got %d", aesKeyBytes, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("encrypt: aes.NewCipher: %w", err)
	}
	bs := block.BlockSize()
	padded := pkcs5Pad(plain, bs)
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(out[i:i+bs], padded[i:i+bs])
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

// DecryptBody is the inverse of EncryptBody: base64-std-decodes
// the input, AES-ECB-decrypts each block, and strips PKCS#5
// padding. Returns an error on bad base64, length not a multiple
// of the AES block size, or invalid padding.
func DecryptBody(b64 string, key []byte) ([]byte, error) {
	if len(key) != aesKeyBytes {
		return nil, fmt.Errorf("encrypt: AES key length must be %d bytes, got %d", aesKeyBytes, len(key))
	}
	cipherBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, fmt.Errorf("encrypt: base64-decode body: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("encrypt: aes.NewCipher: %w", err)
	}
	bs := block.BlockSize()
	if len(cipherBytes) == 0 {
		return nil, fmt.Errorf("encrypt: ciphertext empty")
	}
	if len(cipherBytes)%bs != 0 {
		return nil, fmt.Errorf("encrypt: ciphertext length %d is not a multiple of block size %d", len(cipherBytes), bs)
	}
	out := make([]byte, len(cipherBytes))
	for i := 0; i < len(cipherBytes); i += bs {
		block.Decrypt(out[i:i+bs], cipherBytes[i:i+bs])
	}
	return pkcs5Unpad(out, bs)
}

// pkcs5Pad appends padding bytes per RFC 5652 §6.3 (PKCS#7) — for
// AES's 16-byte block, this is the same as PKCS#5. Always adds at
// least one full block of padding (so the final byte unambiguously
// encodes the pad length).
func pkcs5Pad(in []byte, blockSize int) []byte {
	padLen := blockSize - (len(in) % blockSize)
	out := make([]byte, len(in)+padLen)
	copy(out, in)
	for i := len(in); i < len(out); i++ {
		out[i] = byte(padLen)
	}
	return out
}

// pkcs5Unpad strips PKCS#5 padding from the final block of in.
// Validates that every padding byte equals the declared length;
// returns an error on any deviation. Constant-time-ish in the
// happy path (padding byte mismatch fails fast — the partner
// protocol does not need timing attack resistance here because
// the AES key changes per request).
func pkcs5Unpad(in []byte, blockSize int) ([]byte, error) {
	if len(in) == 0 || len(in)%blockSize != 0 {
		return nil, fmt.Errorf("encrypt: padded input length %d invalid", len(in))
	}
	padLen := int(in[len(in)-1])
	if padLen == 0 || padLen > blockSize {
		return nil, fmt.Errorf("encrypt: padding length %d out of range [1, %d]", padLen, blockSize)
	}
	tail := in[len(in)-padLen:]
	if !bytes.Equal(tail, bytes.Repeat([]byte{byte(padLen)}, padLen)) {
		return nil, fmt.Errorf("encrypt: padding bytes do not match declared length")
	}
	return in[:len(in)-padLen], nil
}
