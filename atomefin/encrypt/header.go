package encrypt

import (
	"fmt"
	"net/url"
	"strings"
)

// EncryptHeaderName is the exact wire-name of the encryption
// header. The upstream gateway is case-sensitive on the lookup
// side; the SDK consistently uses this constant rather than the
// raw string literal so case drift can't sneak in.
const EncryptHeaderName = "Encrypt"

// SymmetricKeyField is the only k=v entry the v0.3 protocol
// declares inside the Encrypt header value. The header parser
// returns a `map[string]string` so a future spec revision can
// add fields (e.g. `iv=` for AES-CBC) without breaking the
// surface.
const SymmetricKeyField = "symmetricKey"

// BuildEncryptHeader formats the Encrypt header value for a
// single wrapped AES key. wrappedKeyB64 is the output of
// WrapAESKey (base64 of the RSA-encrypted AES key bytes); the
// returned string is URL-escaped because the base64 alphabet
// includes `+`, `/`, and `=` which break header values without
// escaping.
//
// Result shape:
//
//	symmetricKey=<urlEncoded(wrappedKeyB64)>
//
// Compose into the actual Encrypt header via:
//
//	header := encrypt.BuildEncryptHeader(wrappedKeyB64)
//	req.Header.Set(encrypt.EncryptHeaderName, header)
func BuildEncryptHeader(wrappedKeyB64 string) string {
	return SymmetricKeyField + "=" + url.QueryEscape(wrappedKeyB64)
}

// ParseEncryptHeader splits an Encrypt header value of shape
// `k=v,k=v,...` into a map. Each value is URL-unescaped on the
// way out — callers receive the raw base64 string suitable for
// passing to UnwrapAESKey.
//
// Returns the map plus, when present, the symmetricKey value
// pre-extracted as the second return for caller convenience.
// Empty / malformed input returns an error; the partner-protocol-
// mandated minimum is one `symmetricKey=<value>` pair, but the
// parser itself enforces only well-formed `k=v` syntax — the
// caller decides which fields are required.
func ParseEncryptHeader(headerValue string) (kv map[string]string, err error) {
	if strings.TrimSpace(headerValue) == "" {
		return nil, fmt.Errorf("encrypt: header value is empty")
	}
	kv = make(map[string]string)
	for _, part := range strings.Split(headerValue, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("encrypt: header part %q missing '=' separator", part)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			return nil, fmt.Errorf("encrypt: header part %q has empty key", part)
		}
		unescaped, uerr := url.QueryUnescape(v)
		if uerr != nil {
			return nil, fmt.Errorf("encrypt: header part %q: unescape value: %w", part, uerr)
		}
		kv[k] = unescaped
	}
	if len(kv) == 0 {
		return nil, fmt.Errorf("encrypt: header value contained no k=v pairs")
	}
	return kv, nil
}

// SymmetricKeyFrom is a tiny convenience that pulls the
// symmetricKey value out of the parsed header map and verifies
// it's non-empty. Returns an error if the field is missing or
// blank — common case for partner-side callback decryption.
func SymmetricKeyFrom(kv map[string]string) (string, error) {
	v, ok := kv[SymmetricKeyField]
	if !ok || v == "" {
		return "", fmt.Errorf("encrypt: header missing required field %q", SymmetricKeyField)
	}
	return v, nil
}
