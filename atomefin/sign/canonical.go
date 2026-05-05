package sign

import (
	"net/url"
	"sort"
	"strings"
)

// CanonicalQuery returns the canonical signing-input string for a GET
// request, per the apaylater spec's Signature section:
//
//	"GET: Sign the request parameters which parameter names are
//	 sorted in alphabetical natural order"
//
// (DESIGN.md §1.3 — verbatim spec quote, also DESIGN.md §4 / §5).
//
// All five v1 spec endpoints are POST, so this canonical is reserved
// rather than exercised by the current SDK surface (Client.DoSigned
// is POST-only and rejects other verbs at the call site). Partners
// writing forward-compat code that needs a GET — or partners
// integrating with a future spec revision that adds one — should
// build the bytes via this helper and feed them to Signer.Sign
// directly. The Signer is verb-agnostic; it signs whatever bytes it
// is handed.
//
// Rules:
//   - Keys are sorted in lexicographic order.
//   - For repeated keys, values are emitted in their existing slice order.
//   - Both keys and values are percent-encoded per RFC 3986 unreserved set:
//     space encodes as "%20" (NOT "+"), "+" encodes as "%2B", etc.
//   - Pairs are joined by "&", separated by "=" between key and value.
//   - Keys with an empty value slice are skipped; a key with a single empty
//     string value is emitted as "key=".
//
// The wire encoding url.Values.Encode() produces uses application/x-www-form-
// urlencoded conventions ("+" for space) which is *not* what RFC 3986 requires
// for the query component. Many partner SDKs canonicalize differently from how
// they wire-encode, so we keep the canonical form pinned to RFC 3986 and let
// the HTTP layer do its own wire encoding independently.
func CanonicalQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	// Pre-grow rough estimate: avg 16 bytes/pair.
	b.Grow(16 * len(keys))

	first := true
	for _, k := range keys {
		vs := values[k]
		if len(vs) == 0 {
			continue
		}
		ek := rfc3986Escape(k)
		for _, v := range vs {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(ek)
			b.WriteByte('=')
			b.WriteString(rfc3986Escape(v))
		}
	}
	return b.String()
}

// rfc3986Escape percent-encodes s such that only RFC 3986 unreserved
// characters (ALPHA / DIGIT / "-" / "." / "_" / "~") survive.
//
// We do not delegate to url.QueryEscape because that uses form-encoding
// conventions ("+" for space). url.PathEscape is closer but leaves a few
// sub-delims un-escaped which we don't want in a signing canonical form.
func rfc3986Escape(s string) string {
	const upperhex = "0123456789ABCDEF"

	// Fast path: scan once to see if anything needs escaping. Most keys and
	// most numeric / ULID-like values pass through untouched.
	needs := false
	for i := 0; i < len(s); i++ {
		if !isUnreserved(s[i]) {
			needs = true
			break
		}
	}
	if !needs {
		return s
	}

	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteByte(upperhex[c>>4])
		b.WriteByte(upperhex[c&0xF])
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	switch {
	case 'A' <= c && c <= 'Z':
		return true
	case 'a' <= c && c <= 'z':
		return true
	case '0' <= c && c <= '9':
		return true
	case c == '-' || c == '.' || c == '_' || c == '~':
		return true
	}
	return false
}
