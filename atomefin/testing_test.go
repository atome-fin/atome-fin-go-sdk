package atomefin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// slowServer never sends a response; useful for exercising client-side
// timeout semantics. Caller must Close() it.
func slowServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until the client gives up.
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
}
