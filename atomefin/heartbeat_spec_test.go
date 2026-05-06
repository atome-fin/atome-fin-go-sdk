package atomefin_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_HeartBeat drives Client.HeartBeat against the spec
// server. /heart-beat declares no required query parameters; the
// signed empty-canonical request must reach the server cleanly.
func TestSpec_HeartBeat(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "GET /heart-beat",
			Run: func(c *atomefin.Client) error {
				return c.HeartBeat(context.Background())
			},
		},
	})
}
