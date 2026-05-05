package transport

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildUserAgent(t *testing.T) {
	ua := BuildUserAgent("0.1.0", "")
	if !strings.HasPrefix(ua, "atome-fin-go-sdk/0.1.0") {
		t.Errorf("UA prefix = %q, want atome-fin-go-sdk/0.1.0", ua)
	}
	if !strings.Contains(ua, runtime.GOOS) {
		t.Errorf("UA = %q, missing GOOS %q", ua, runtime.GOOS)
	}
	if !strings.Contains(ua, runtime.GOARCH) {
		t.Errorf("UA = %q, missing GOARCH %q", ua, runtime.GOARCH)
	}

	with := BuildUserAgent("0.1.0", "merchant-foo/1.2")
	if !strings.HasSuffix(with, "merchant-foo/1.2") {
		t.Errorf("UA = %q, want trailing suffix", with)
	}

	noVersion := BuildUserAgent("", "")
	if !strings.HasPrefix(noVersion, "atome-fin-go-sdk ") {
		t.Errorf("UA = %q, expected product name without slash", noVersion)
	}

	// Whitespace-only suffixes are trimmed away — we don't want trailing
	// spaces in the header value.
	trimmed := BuildUserAgent("0.1.0", "   ")
	if strings.HasSuffix(trimmed, " ") {
		t.Errorf("UA = %q, has trailing whitespace", trimmed)
	}
}
