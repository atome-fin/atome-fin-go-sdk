// Package encrypt provides the AES-ECB-PKCS5 + RSA-PKCS#1 v1.5
// hybrid envelope used by the apaylater partner protocol on its two
// credit-flow POSTs (`/credit-information`, `/credit-application`).
// Q31 — Q34 (resolved 2026-05-06) pin the algorithm choices below;
// every other v0.2 endpoint stays plaintext.
//
// # Wire shape
//
//	Encrypt: symmetricKey=<urlEncoded(base64(rsaWrapped(aesKey)))>
//	Authorization: <base64(SHA256WithRSA(<encryptedBody>))>
//	<HTTP body> := base64(AES-ECB-PKCS5(plaintext, aesKey))
//
// Critical ordering (per partner reference at
// `~/Downloads/main.go` line 28-30):
//
//	Outbound : encrypt body → sign encrypted body
//	Inbound  : verify signature on encrypted body → decrypt
//
// # Algorithm choices
//
//   - Body cipher: AES-ECB with PKCS#5 padding.
//     ECB is partner-protocol-mandated; the SDK does NOT choose it.
//     (`crypto/cipher` deliberately omits ECB; aes.go walks the
//     block cipher manually.)
//   - Key length: AES-256 — exactly 32 bytes. Per partner
//     constraint, those 32 bytes are restricted to printable A — Z
//     (sample line 367-384). The SDK enforces both.
//   - Key wrap: RSA-PKCS#1 v1.5 (NOT OAEP). Mirrors Java's
//     `Cipher.getInstance("RSA")` default. Min 2048-bit RSA modulus.
//   - Header value: `symmetricKey=<urlEncoded(base64(...))>` — the
//     `,`-delimited `k=v` shape leaves room for forward-compat
//     fields (e.g. an `iv=` if the partner ever moves to AES-CBC);
//     ParseEncryptHeader returns a `map[string]string`.
//
// # Determinism
//
// The body ciphertext is deterministic for a given (plaintext,
// aesKey) pair — round-trip tests can byte-compare against an
// external vector. The wrapped AES key is NON-deterministic (RSA
// PKCS#1 v1.5 picks fresh padding bytes each call) so external-
// vector tests verify the wrap path via wrap-then-unwrap rather
// than byte equality.
//
// # Surface
//
// High-level (the only function `Client.DoEncryptedSigned`
// reaches for):
//
//	Marshal(plain []byte, atomePub *rsa.PublicKey) (header, bodyB64 string, err error)
//	Unmarshal(header, bodyB64 string, partnerPriv *rsa.PrivateKey) (plain []byte, err error)
//
// Low-level building blocks for callers that need explicit control
// (e.g. partner-side callback decryption — Q31 leaves credit
// callbacks plaintext for now, but the surface is shipped for
// forward-compat):
//
//	RandomAESKey() (string, error)        // 32-char A — Z, rejection-sampled
//	ValidateAESKey(key string) error      // strict alphabet + length check
//
//	EncryptBody(plain, key []byte) (b64 string, err error)
//	DecryptBody(b64 string, key []byte) (plain []byte, err error)
//
//	WrapAESKey(key []byte, atomePub *rsa.PublicKey) (b64 string, err error)
//	UnwrapAESKey(b64 string, partnerPriv *rsa.PrivateKey) (key []byte, err error)
//
//	BuildEncryptHeader(wrappedKeyB64 string) string
//	ParseEncryptHeader(headerValue string) (kv map[string]string, err error)
package encrypt
