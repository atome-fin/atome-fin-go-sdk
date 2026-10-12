package sign

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

// OVO.HMAC (HMAC-SHA256) is the signing scheme the Grab/OVO gateway
// applies to every Atome → partner callback per the 2026-10-12 spec
// (headers `client-id`, `time`, `signature`; RSA2 `Authorization` is
// not used on callbacks). The canonical and key derivation follow
// docs/grab/credit-application-hmac-signing.md:
//
//	string-to-sign = {client-id}{time}{METHOD} {path_including_query}{base64RawStd(raw_body)}
//	key            = hex(sha256(client_secret))  // the hex TEXT bytes, lowercase
//	signature      = hex(hmac_sha256(key, string-to-sign)), lowercase
//
// Only METHOD and path are separated (by one space); every other field
// is concatenated with no delimiter. The body is base64 standard
// alphabet WITHOUT padding. A GET signs an empty body segment.

// ErrHMACKey is returned when an OVO.HMAC signer/verifier is
// constructed without a client secret.
var ErrHMACKey = errors.New("sign: OVO.HMAC requires a non-empty client secret")

// OVOHMACKey derives the HMAC key from the Grab-provisioned client
// secret: the lowercase hex encoding of sha256(secret), used as ASCII
// bytes (NOT the raw digest, and NOT the secret itself).
func OVOHMACKey(clientSecret string) []byte {
	sum := sha256.Sum256([]byte(clientSecret))
	return []byte(hex.EncodeToString(sum[:]))
}

// OVOHMACCanonical assembles the OVO.HMAC string-to-sign.
// pathWithQuery must include the query string when one is present
// (e.g. "/callback?a=1"). body may be nil for bodyless requests.
func OVOHMACCanonical(clientID, timeMillis, method, pathWithQuery string, body []byte) []byte {
	b64 := base64.RawStdEncoding.EncodeToString(body)
	out := make([]byte, 0, len(clientID)+len(timeMillis)+len(method)+1+len(pathWithQuery)+len(b64))
	out = append(out, clientID...)
	out = append(out, timeMillis...)
	out = append(out, method...)
	out = append(out, ' ')
	out = append(out, pathWithQuery...)
	out = append(out, b64...)
	return out
}

// ovoHMAC computes the lowercase-hex HMAC-SHA256 of canonical under
// the derived key.
func ovoHMAC(key, canonical []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(canonical)
	return hex.EncodeToString(mac.Sum(nil))
}

// ovoHMACVerifier implements Verifier over the OVO.HMAC scheme. The
// caller (atomefin/callback's HMAC handler path) builds the canonical
// with OVOHMACCanonical from the inbound request; Verify only computes
// and constant-time-compares the MAC.
type ovoHMACVerifier struct {
	key []byte
}

// NewOVOHMACVerifier constructs a Verifier for OVO.HMAC signatures.
// clientSecret is the Grab-provisioned secret (delivered via the
// envelope-encryption flow); the HMAC key is derived internally with
// OVOHMACKey.
func NewOVOHMACVerifier(clientSecret string) (Verifier, error) {
	if clientSecret == "" {
		return nil, ErrHMACKey
	}
	return &ovoHMACVerifier{key: OVOHMACKey(clientSecret)}, nil
}

func (v *ovoHMACVerifier) Verify(ctx context.Context, canonical []byte, signature string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if signature == "" {
		return fmt.Errorf("%w: empty signature", ErrSignature)
	}
	want := ovoHMAC(v.key, canonical)
	// Case-insensitive hex compare is deliberately NOT offered: the
	// spec mandates lowercase, and normalizing would widen the
	// accepted-input space for no interop gain.
	if subtle.ConstantTimeCompare([]byte(want), []byte(signature)) != 1 {
		return fmt.Errorf("%w: OVO.HMAC mismatch", ErrSignature)
	}
	return nil
}

// ovoHMACSigner is the outbound mirror of ovoHMACVerifier. The SDK's
// partner-side use is test/mock traffic (atomefin/mock, handler
// tests); production outbound signing runs on the Atome side.
type ovoHMACSigner struct {
	key []byte
}

// NewOVOHMACSigner constructs a Signer producing lowercase-hex
// OVO.HMAC signatures over a canonical built with OVOHMACCanonical.
// KeyID returns "" (the scheme has no key-identifier convention).
func NewOVOHMACSigner(clientSecret string) (Signer, error) {
	if clientSecret == "" {
		return nil, ErrHMACKey
	}
	return &ovoHMACSigner{key: OVOHMACKey(clientSecret)}, nil
}

func (s *ovoHMACSigner) Sign(ctx context.Context, canonical []byte) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return ovoHMAC(s.key, canonical), nil
}

func (s *ovoHMACSigner) KeyID() string { return "" }
