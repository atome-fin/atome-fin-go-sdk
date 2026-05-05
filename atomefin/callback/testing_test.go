package callback_test

import (
	"io"
	"os"
)

// openFixture is a tiny os.Open wrapper kept in its own file so the
// handler tests above don't need to import "os" at the top level.
func openFixture(path string) (io.ReadCloser, error) {
	return os.Open(path)
}
