package atomefin

import (
	"bytes"
	"encoding/json"
)

// MarshalSigning serializes v as compact JSON whose bytes are simultaneously
// (a) the wire payload and (b) the canonical input passed to the Signer.
//
// Why this exists: encoding/json.Marshal HTML-escapes "&", "<" and ">" to
// "&" / "<" / ">" by default. If T3 (or any caller) marshals
// with json.Marshal, signs the result, and then transmits a *different*
// byte sequence — say, by re-marshalling without escaping somewhere along
// the way, or by feeding the unescaped form to a server-side reference
// implementation — the signature canonical inputs diverge and the request
// returns 401 INVALID_SIGNATURE in production. Any payload field that may
// contain "&" (a shipping address — "Foo & Co"), "<" or ">" trips this.
//
// MarshalSigning uses json.Encoder with SetEscapeHTML(false) so the bytes
// are stable across implementations. The trailing newline json.Encoder
// appends is stripped so what gets signed is byte-for-byte what gets sent.
//
// Always use this helper (not json.Marshal) for any payload that flows
// into Client.DoSigned.
func MarshalSigning(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	b := buf.Bytes()
	// json.Encoder.Encode appends '\n' at the end of every value — trim it.
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	return b, nil
}
