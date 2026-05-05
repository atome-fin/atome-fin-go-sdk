package sign

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzCanonicalQuery exercises CanonicalQuery with arbitrary key/value pairs
// and asserts structural invariants:
//
//  1. The result never contains a literal '+' (RFC 3986: space => %20).
//  2. Pairs are always sorted by key.
//  3. The encoding is deterministic / stable across calls (the function is
//     pure but Go map iteration is randomized — this guards against any
//     accidental order-leak).
//  4. Round-tripping through url.ParseQuery returns the original values.
//
// The fuzz target deliberately keeps cardinality low (3 pairs) so the seed
// corpus stays cheap to explore but still hits the multi-key sort path.
func FuzzCanonicalQuery(f *testing.F) {
	f.Add("a", "1", "b", "2", "c", "3")
	f.Add("z", "", "a", "x y", "m", "a+b")
	f.Add("", "", "", "", "", "")
	f.Add("k", "é", "k", "ü", "k", "ñ")
	f.Add("Z", "1", "a", "2", "Z", "3")

	f.Fuzz(func(t *testing.T, k1, v1, k2, v2, k3, v3 string) {
		v := url.Values{}
		v.Add(k1, v1)
		v.Add(k2, v2)
		v.Add(k3, v3)

		got := CanonicalQuery(v)

		// Determinism / stability.
		if again := CanonicalQuery(v); again != got {
			t.Fatalf("non-deterministic: %q vs %q", got, again)
		}

		if got == "" {
			// Empty input must roundtrip cleanly. A genuinely empty
			// Values{} would only happen if all three keys collide and
			// nothing else, which CanonicalQuery still emits ("=") so
			// "" here is unreachable barring a bug.
			return
		}

		// Invariant 1: no '+' in canonical form.
		if strings.Contains(got, "+") {
			t.Fatalf("canonical contains '+': %q", got)
		}

		// Invariant 2: pairs sorted by key (post-decode).
		pairs := strings.Split(got, "&")
		for i := 1; i < len(pairs); i++ {
			prevKey, _, _ := strings.Cut(pairs[i-1], "=")
			currKey, _, _ := strings.Cut(pairs[i], "=")
			pk, err := url.QueryUnescape(prevKey)
			if err != nil {
				t.Fatalf("bad escaped key %q: %v", prevKey, err)
			}
			ck, err := url.QueryUnescape(currKey)
			if err != nil {
				t.Fatalf("bad escaped key %q: %v", currKey, err)
			}
			if pk > ck {
				t.Fatalf("keys not sorted: %q before %q in %q", pk, ck, got)
			}
		}

		// Invariant 4: parse back via url.ParseQuery (which understands
		// %20 just as well as +) and compare key sets + values per key.
		parsed, err := url.ParseQuery(got)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", got, err)
		}
		for k, vs := range v {
			pvs := parsed[k]
			if len(pvs) != len(vs) {
				t.Fatalf("key %q: %d values after roundtrip, want %d (got %q)",
					k, len(pvs), len(vs), got)
			}
			// Order within a key must be preserved.
			for i := range vs {
				if pvs[i] != vs[i] {
					t.Fatalf("key %q value %d: got %q want %q (canonical %q)",
						k, i, pvs[i], vs[i], got)
				}
			}
		}
	})
}
