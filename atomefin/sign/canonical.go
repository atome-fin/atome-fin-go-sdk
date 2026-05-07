package sign

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// MultiValueQueryError is returned by CanonicalQuery when the input
// url.Values contains more than one value for any key.
//
// **DEPRECATED v0.5.1.** Per partner clarification, the upstream
// gateway's verification semantics are spec-aligned but
// asymmetric: the wire query keeps every value (so no data is
// dropped), and the gateway computes the verification canonical
// using only the FIRST value per key. The v0.2.3 hard-fail
// hardened a developer-advisory "callers should pre-flatten" into
// a "callers must pre-flatten" — wrong: partners genuinely need
// to send multi-value, and the SDK should sign the first-value
// canonical without dropping the wire data.
//
// The lenient first-value path now lives in
// CanonicalQueryFirstValue (no error return) and is what
// Client.DoSignedGET uses since v0.5.1. CanonicalQuery itself
// still hard-fails for backward compatibility — partners who
// programmatically caught *MultiValueQueryError to do their own
// pre-flatten still get the same diagnostic. New code should
// prefer CanonicalQueryFirstValue.
type MultiValueQueryError struct {
	// Key is the offending parameter name.
	Key string
	// Count is the number of values supplied for the key (always > 1).
	Count int
}

// Error implements the error interface.
func (e *MultiValueQueryError) Error() string {
	return fmt.Sprintf("sign: query key %q has %d values; the canonical signing form requires single-value (pre-flatten before calling)", e.Key, e.Count)
}

// CanonicalQuery returns the canonical signing-input string for a GET
// request, per the apaylater spec's Signature section:
//
//	"GET: Sign the request parameters which parameter names are
//	 sorted in alphabetical natural order"
//
// (DESIGN.md §1.3 — verbatim spec quote, also DESIGN.md §4 / §5).
//
// Rules:
//   - Keys are sorted in lexicographic order.
//   - Each key MUST have at most one value. Multi-value keys
//     return a *MultiValueQueryError. **DEPRECATED v0.5.1**: per
//     partner clarification, the spec semantic is asymmetric —
//     wire keeps both values; canonical signs first. Use
//     CanonicalQueryFirstValue for the spec-aligned canonical;
//     this function's strict behaviour is preserved for backward
//     compatibility with v0.2.3 — v0.5.0 partners catching
//     *MultiValueQueryError to do their own pre-flatten.
//   - Both keys and values are percent-encoded per RFC 3986 unreserved
//     set: space encodes as "%20" (NOT "+"), "+" encodes as "%2B", etc.
//   - Pairs are joined by "&", separated by "=" between key and value.
//   - Keys with an empty value slice are skipped; a key with a single
//     empty string value is emitted as "key=".
//
// The wire encoding url.Values.Encode() produces uses application/x-www-form-
// urlencoded conventions ("+" for space) which is *not* what RFC 3986 requires
// for the query component. Many partner SDKs canonicalize differently from how
// they wire-encode, so we keep the canonical form pinned to RFC 3986 and let
// the HTTP layer do its own wire encoding independently.
//
// Breaking change in v0.2.3: returns (string, error) — see
// MultiValueQueryError above. Pre-v0.2.3 callers using the
// single-return form must adopt the two-return shape; the only
// non-test in-repo caller is Client.DoSignedGET, updated atomically
// in the same release.
func CanonicalQuery(values url.Values) (string, error) {
	if len(values) == 0 {
		return "", nil
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
		if len(vs) > 1 {
			return "", &MultiValueQueryError{Key: k, Count: len(vs)}
		}
		ek := rfc3986Escape(k)
		if !first {
			b.WriteByte('&')
		}
		first = false
		b.WriteString(ek)
		b.WriteByte('=')
		b.WriteString(rfc3986Escape(vs[0]))
	}
	return b.String(), nil
}

// CanonicalQueryFirstValue returns the canonical signing-input
// string per the spec's first-value-per-key semantic, added in
// v0.5.1. For every key (sorted lexicographically) it emits
// EXACTLY ONE pair using the first value; multi-value keys do
// NOT raise an error.
//
// This is the spec-aligned counterpart of CanonicalQuery: the
// upstream gateway's verification canonical observes only the
// first value per key, so signing the first-value canonical and
// transmitting all values on the wire produces a request the
// gateway accepts without data loss. See R13b in
// `atomefin/dosigned_get_test.go`.
//
// Single-value inputs produce bytes byte-identical to
// CanonicalQuery's output (when CanonicalQuery doesn't error).
// Use this for sign-time canonical; use CanonicalQuery only
// when strict input-validation matters (e.g. you want a
// hard-fail on accidental multi-value supply).
//
// Encoding rules are otherwise identical to CanonicalQuery:
// alphabetical key sort, RFC 3986 percent-encoding (space →
// %20, NOT '+'), pairs joined by '&'.
func CanonicalQueryFirstValue(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.Grow(16 * len(keys))

	first := true
	for _, k := range keys {
		vs := values[k]
		if len(vs) == 0 {
			continue
		}
		if !first {
			b.WriteByte('&')
		}
		first = false
		b.WriteString(rfc3986Escape(k))
		b.WriteByte('=')
		b.WriteString(rfc3986Escape(vs[0]))
	}
	return b.String()
}

// EncodeWireQueryRFC3986 returns the wire-format query string
// for a GET request — preserves multi-value (every value per
// key gets its own `k=v` pair) using the same RFC 3986
// percent-encoding as the canonical helpers above.
//
// Single-value inputs produce bytes byte-identical to
// CanonicalQueryFirstValue (and to CanonicalQuery's output when
// it doesn't error), so the wire-equals-canonical R13a
// invariant for single-value queries is preserved automatically.
//
// Multi-value inputs produce wire bytes that DIFFER from the
// canonical first-value bytes — this is the asymmetric R13b
// invariant added in v0.5.1: wire keeps all data, canonical
// signs the first value per key.
//
// Used by Client.DoSignedGET to build the wire query bytes
// while CanonicalQueryFirstValue produces the bytes fed to the
// signer.
func EncodeWireQueryRFC3986(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
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
