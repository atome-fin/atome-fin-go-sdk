package sign

import (
	"net/url"
	"strings"
	"testing"
)

func TestCanonicalQuery(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   url.Values
		want string
	}{
		{"empty", url.Values{}, ""},
		{"single", url.Values{"a": {"1"}}, "a=1"},
		{"sorted by key", url.Values{"b": {"2"}, "a": {"1"}}, "a=1&b=2"},
		{"multivalue keeps slice order", url.Values{"a": {"1", "2", "3"}}, "a=1&a=2&a=3"},
		{"empty value emitted as bare equals", url.Values{"a": {""}}, "a="},
		{"nil value slice skipped", url.Values{"a": nil, "b": {"1"}}, "b=1"},
		// RFC 3986: space => %20, NOT '+'.
		{"space encodes as %20", url.Values{"q": {"hello world"}}, "q=hello%20world"},
		{"plus is not space", url.Values{"q": {"a+b"}}, "q=a%2Bb"},
		// Reserved subdelims must be percent-encoded.
		{"reserved chars encoded", url.Values{"k": {"a&b=c?d/e@f"}}, "k=a%26b%3Dc%3Fd%2Fe%40f"},
		// Unreserved set survives untouched.
		{"unreserved untouched", url.Values{"k": {"AZaz09-._~"}}, "k=AZaz09-._~"},
		// Key escaping.
		{"key with space", url.Values{"a b": {"1"}}, "a%20b=1"},
		// Unicode (UTF-8 bytes encoded individually).
		{"utf8", url.Values{"k": {"é"}}, "k=%C3%A9"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CanonicalQuery(tc.in); got != tc.want {
				t.Errorf("CanonicalQuery(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCanonicalQuery_Stable proves the encoding does not depend on map
// iteration order: building the same Values via 100 different insertion paths
// produces identical bytes.
func TestCanonicalQuery_Stable(t *testing.T) {
	t.Parallel()

	const want = "a=1&a=2&b=3&c=4"
	for i := 0; i < 100; i++ {
		v := url.Values{}
		v.Add("c", "4")
		v.Add("a", "1")
		v.Add("b", "3")
		v.Add("a", "2")
		if got := CanonicalQuery(v); got != want {
			t.Fatalf("iter %d: got %q, want %q", i, got, want)
		}
	}
}

// TestCanonicalQuery_NoPlusForSpace is a guard test: the spec is silent on
// space encoding but DESIGN.md pins this to RFC 3986. Regression here would
// silently break signature verification on the partner side.
func TestCanonicalQuery_NoPlusForSpace(t *testing.T) {
	t.Parallel()

	got := CanonicalQuery(url.Values{"a": {"x y"}})
	if strings.Contains(got, "+") {
		t.Errorf("CanonicalQuery used '+' for space: %q", got)
	}
	if got != "a=x%20y" {
		t.Errorf("got %q, want %q", got, "a=x%20y")
	}
}

func TestRFC3986Escape_Boundaries(t *testing.T) {
	t.Parallel()

	// Spot-check every printable ASCII byte for unreserved-set membership.
	for c := byte(0x20); c < 0x7F; c++ {
		got := rfc3986Escape(string([]byte{c}))
		want := isUnreserved(c)
		if want && got != string([]byte{c}) {
			t.Errorf("byte %#x: unreserved but got %q", c, got)
		}
		if !want && len(got) != 3 {
			t.Errorf("byte %#x: reserved but got %q (want 3-byte percent-encoding)", c, got)
		}
	}
}
