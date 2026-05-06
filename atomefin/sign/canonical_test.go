package sign

import (
	"errors"
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
			got, err := CanonicalQuery(tc.in)
			if err != nil {
				t.Fatalf("CanonicalQuery(%v): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("CanonicalQuery(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCanonicalQuery_RejectsMultiValue pins the v0.2.3 fix: multi-value
// queries hard-fail with *MultiValueQueryError rather than silently
// emitting both values (which the upstream gateway, observing only
// vs[0], would reject as INVALID_SIGNATURE).
func TestCanonicalQuery_RejectsMultiValue(t *testing.T) {
	t.Parallel()

	got, err := CanonicalQuery(url.Values{"a": {"1", "2", "3"}})
	if err == nil {
		t.Fatalf("CanonicalQuery(multi-value): want error, got %q", got)
	}
	var mverr *MultiValueQueryError
	if !errors.As(err, &mverr) {
		t.Fatalf("err = %v; want *MultiValueQueryError", err)
	}
	if mverr.Key != "a" {
		t.Errorf("mverr.Key = %q; want %q", mverr.Key, "a")
	}
	if mverr.Count != 3 {
		t.Errorf("mverr.Count = %d; want 3", mverr.Count)
	}
	// Error message mentions the key and points at the fix.
	if !strings.Contains(err.Error(), `"a"`) {
		t.Errorf("error message missing key reference: %v", err)
	}
	if !strings.Contains(err.Error(), "single-value") {
		t.Errorf("error message missing single-value guidance: %v", err)
	}
}

// TestCanonicalQuery_Stable proves the encoding does not depend on map
// iteration order: building the same Values via 100 different insertion paths
// produces identical bytes (single-value only post-v0.2.3).
func TestCanonicalQuery_Stable(t *testing.T) {
	t.Parallel()

	const want = "a=1&b=3&c=4"
	for i := 0; i < 100; i++ {
		v := url.Values{}
		v.Add("c", "4")
		v.Add("a", "1")
		v.Add("b", "3")
		got, err := CanonicalQuery(v)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("iter %d: got %q, want %q", i, got, want)
		}
	}
}

// TestCanonicalQuery_NoPlusForSpace is a guard test: the spec is silent on
// space encoding but DESIGN.md pins this to RFC 3986. Regression here would
// silently break signature verification on the partner side.
func TestCanonicalQuery_NoPlusForSpace(t *testing.T) {
	t.Parallel()

	got, err := CanonicalQuery(url.Values{"a": {"x y"}})
	if err != nil {
		t.Fatalf("CanonicalQuery: %v", err)
	}
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
