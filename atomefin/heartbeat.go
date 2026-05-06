package atomefin

import (
	"context"
	"errors"
)

// HeartBeat probes the atome-fin gateway with a signed GET
// /heart-beat. Returns nil if the gateway responds 2xx, an
// *APIError for any non-2xx (the canonical envelope is decoded
// where present), or a *TransportError for transport-level
// failures.
//
// v0.2 chunk #9. Use case: liveness probe before a critical flow,
// pre-flight check after partner-side connectivity blips, or as
// the synthetic-monitoring poll in production.
//
// The call is signed (the empty canonical query is itself the
// signing canonical — `sign.CanonicalQuery(nil)` returns "" and
// the partner's signature over zero bytes still verifies). All
// the standard plumbing applies: retry policy, observer hooks,
// ctx cancellation honoured during backoff sleeps.
//
// HeartBeat returns the parsed response body unread because the
// spec does not define a stable response payload — partners that
// need response inspection can call DoSignedGET("/heart-beat", nil)
// directly until the spec stabilises and a typed sibling lands.
func (c *Client) HeartBeat(ctx context.Context) error {
	if c == nil {
		return errors.New("atomefin: HeartBeat called on nil *Client")
	}
	_, err := c.DoSignedGET(ctx, "/heart-beat", nil)
	return err
}
