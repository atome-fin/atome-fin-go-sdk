package sign

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// MultiValueQueryError is returned by CanonicalQuery when the input
// url.Values contains more than one value for any key. The
// apaylater spec's Signature section is silent on multi-value
// canonicalization (the rule reads "sorted in alphabetical natural
// order" — singular per key). The upstream gateway's observed
// behaviour is to retain only the first value for verification, so
// emitting all values produced an asymmetric canonical that
// silently failed signature verification with a generic
// `INVALID_SIGNATURE` 401 — a subtle foot-gun for any partner that
// fed a multi-value `url.Values` through `Client.DoSignedGET`.
//
// v0.2.3 makes the contract explicit: callers MUST pre-flatten
// multi-value queries (typically by joining values into a single
// comma-separated string per the partner agreement, or by picking
// one value) before signing. The resulting hard fail at sign time
// surfaces the bug in the dev loop rather than at production
// first-call.
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
//   - Each key MUST have at most one value. Multi-value keys return
//     a *MultiValueQueryError; the spec does not define repeated-key
//     canonicalization and the upstream gateway only retains the first
//     value, so emitting all values would silently fail signature
//     verification. Callers must pre-flatten before signing.
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
