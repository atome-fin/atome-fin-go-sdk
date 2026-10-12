package atomefin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// HeartBeatResponse is the GET /heart-beat 200 payload per spec:
// `code` (enum SUCCESS), `message`, and a free-form `data` object.
type HeartBeatResponse struct {
	Code    Code           `json:"code"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

// IsSuccess reports whether the envelope's Code is SUCCESS. Nil-safe.
func (r *HeartBeatResponse) IsSuccess() bool {
	return r != nil && r.Code == CodeSuccess
}

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
// HeartBeat discards the response body; callers that want the typed
// envelope (code/message/data per spec) should use HeartBeatStatus.
func (c *Client) HeartBeat(ctx context.Context) error {
	if c == nil {
		return errors.New("atomefin: HeartBeat called on nil *Client")
	}
	_, err := c.DoSignedGET(ctx, "/heart-beat", nil)
	return err
}

// HeartBeatStatus is the typed sibling of HeartBeat: same signed GET
// /heart-beat, returning the decoded spec envelope. A 2xx whose body
// does not decode is reported as a *TransportError rather than
// silently swallowed.
func (c *Client) HeartBeatStatus(ctx context.Context) (*HeartBeatResponse, error) {
	if c == nil {
		return nil, errors.New("atomefin: HeartBeatStatus called on nil *Client")
	}
	raw, err := c.DoSignedGET(ctx, "/heart-beat", nil)
	if err != nil {
		return nil, err
	}
	var out HeartBeatResponse
	dec := json.NewDecoder(bytes.NewReader(raw.Body))
	dec.UseNumber()
	if uerr := dec.Decode(&out); uerr != nil {
		return nil, &TransportError{
			Op:  "decode",
			URL: "/heart-beat",
			Err: fmt.Errorf("decode /heart-beat response: %w", uerr),
		}
	}
	if uerr := dec.Decode(new(json.RawMessage)); !errors.Is(uerr, io.EOF) {
		return nil, &TransportError{
			Op:  "decode",
			URL: "/heart-beat",
			Err: fmt.Errorf("decode /heart-beat response: trailing data after JSON value"),
		}
	}
	return &out, nil
}
