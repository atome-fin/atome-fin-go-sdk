package mock

import (
	"bytes"
	"io"
)

// newNopCloserBytes wraps b in an io.ReadCloser whose Close is a
// no-op. Equivalent to io.NopCloser(bytes.NewReader(b)) but kept
// as a tiny named helper so transport.go reads cleanly.
func newNopCloserBytes(b []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(b))
}
