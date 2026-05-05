package transport

import (
	"runtime"
	"strings"
)

// DefaultProductName is the leading product token in the SDK's default
// User-Agent header. It must not contain spaces or the assembly below
// produces an invalid header.
const DefaultProductName = "atome-fin-go-sdk"

// BuildUserAgent returns a User-Agent header value of the form
//
//	atome-fin-go-sdk/<version> (go<goversion>; <goos>/<goarch>) <suffix>
//
// where suffix is whatever the partner passed to atomefin.WithUserAgent.
// suffix is appended verbatim — callers are responsible for keeping it
// well-formed (RFC 7231 §5.5.3).
//
// version is supplied by the umbrella package so transport stays
// dependency-free of the SDK's semver constant; passing an empty string
// reduces to "atome-fin-go-sdk".
func BuildUserAgent(version, suffix string) string {
	var b strings.Builder
	b.Grow(64 + len(suffix))
	b.WriteString(DefaultProductName)
	if version != "" {
		b.WriteByte('/')
		b.WriteString(version)
	}
	b.WriteString(" (go")
	b.WriteString(runtime.Version()[2:]) // strip leading "go"
	b.WriteString("; ")
	b.WriteString(runtime.GOOS)
	b.WriteByte('/')
	b.WriteString(runtime.GOARCH)
	b.WriteByte(')')
	if suffix = strings.TrimSpace(suffix); suffix != "" {
		b.WriteByte(' ')
		b.WriteString(suffix)
	}
	return b.String()
}
