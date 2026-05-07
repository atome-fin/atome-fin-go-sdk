package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Scenario decides how the mock Transport responds to an inbound
// SDK request. Implementations must be safe for concurrent use
// (the SDK's retry pipeline may invoke the same Scenario across
// goroutines). All shipped scenarios below are stateless.
type Scenario interface {
	// Respond builds the *http.Response for r. May return an error
	// to simulate a transport failure (e.g. force a retry path);
	// returning a 5xx response triggers the SDK's retry logic
	// without surfacing as a transport error.
	Respond(r *http.Request) (*http.Response, error)
}

// ScenarioFunc adapts a plain func to the Scenario interface.
type ScenarioFunc func(r *http.Request) (*http.Response, error)

// Respond implements Scenario.
func (f ScenarioFunc) Respond(r *http.Request) (*http.Response, error) { return f(r) }

// AlwaysSuccess returns a Scenario that replies HTTP 200 with the
// canonical SUCCESS envelope `{"code":"SUCCESS","message":"ok"}`
// to every request. The body decodes cleanly into every v0.3
// response struct (Code is set; Data is nil — partners assert on
// the Code rather than the data sub-tree).
func AlwaysSuccess() Scenario {
	return ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{"code":"SUCCESS","message":"ok"}`), nil
	})
}

// AlwaysProcessing returns a Scenario that replies HTTP 200 with
// `data.status = PROCESSING`. Drives the SDK's poll-until-terminal
// branch — useful for asserting that partner code handles the
// async-pending state.
//
// The body is shaped against the auth/capture/voidAuth response
// shape; refund/repayment partners can substitute via PerEndpoint.
func AlwaysProcessing() Scenario {
	return ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		body := `{"code":"SUCCESS","message":"ok","data":{"status":"PROCESSING"}}`
		return jsonResponse(http.StatusOK, body), nil
	})
}

// AlwaysFailed returns a Scenario that replies HTTP 200 with a
// terminal-FAILED envelope. The optional FailureCode populates
// `data.failureCode` so partner branching on FailureCode is
// exercised.
//
// Note: this is a BUSINESS-side failure (status 200 + status:FAILED),
// distinct from AlwaysAPIError which models transport-side 4xx/5xx
// envelopes.
func AlwaysFailed(code atomefin.FailureCode) Scenario {
	return ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		var body string
		if code != "" {
			body = fmt.Sprintf(`{"code":"SUCCESS","message":"ok","data":{"status":"FAILED","failureCode":%q}}`, code)
		} else {
			body = `{"code":"SUCCESS","message":"ok","data":{"status":"FAILED"}}`
		}
		return jsonResponse(http.StatusOK, body), nil
	})
}

// AlwaysAPIError returns a Scenario that replies with an HTTP
// non-2xx status carrying the envelope
// `{"code":<code>,"message":<msg>}`. The SDK's DoSigned /
// DoSignedGET / DoEncryptedSigned all surface this as
// `*atomefin.APIError`.
//
// Use status 400 for spec-defined params errors (PARAMS_MISSING,
// PARAMS_WRONG, INVALID_ENCRYPTION etc.); 401 for INVALID_SIGNATURE;
// 5xx to drive the retry path.
func AlwaysAPIError(httpStatus int, code atomefin.Code, msg string) Scenario {
	return ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
		body := fmt.Sprintf(`{"code":%q,"message":%q}`, code, msg)
		return jsonResponse(httpStatus, body), nil
	})
}

// PerEndpoint returns a Scenario that dispatches by "METHOD path"
// (e.g. "POST /auth", "GET /query-auth"). Partners pass a map of
// per-endpoint Scenarios; requests with no matching key fall back
// to fallback.
//
// Exact match only — no path templating. The map is consulted
// case-sensitively; the SDK's HTTP method is upper-case so map
// keys should match (e.g. "POST", not "post").
//
// Example:
//
//	mock.PerEndpoint(map[string]mock.Scenario{
//	    "POST /auth":    mock.AlwaysSuccess(),
//	    "POST /capture": mock.AlwaysFailed(atomefin.FailureCodeRiskReject),
//	}, mock.AlwaysSuccess())
func PerEndpoint(byOp map[string]Scenario, fallback Scenario) Scenario {
	if fallback == nil {
		fallback = AlwaysSuccess()
	}
	return ScenarioFunc(func(r *http.Request) (*http.Response, error) {
		key := strings.ToUpper(r.Method) + " " + r.URL.Path
		if s, ok := byOp[key]; ok && s != nil {
			return s.Respond(r)
		}
		return fallback.Respond(r)
	})
}

// jsonResponse is the small helper every Scenario uses to emit a
// well-formed *http.Response with JSON Content-Type. The Body is
// a fresh io.NopCloser around bytes.NewReader so the response is
// safe across SDK retries.
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
	}
}

// jsonResponseFromAny encodes v as JSON and wraps it as a 200
// response. Used by partners building scenarios on top of typed
// response structs (e.g. payment.AuthResponse).
//
// Public so partners can implement custom Scenarios without
// re-exporting jsonResponse.
func JSONResponse(status int, v any) (*http.Response, error) {
	buf := &bytes.Buffer{}
	if err := json.NewEncoder(buf).Encode(v); err != nil {
		return nil, fmt.Errorf("mock: encode response: %w", err)
	}
	// Strip the trailing newline json.Encoder appends so the
	// response body matches what hand-rolled jsonResponse emits.
	out := buf.Bytes()
	if n := len(out); n > 0 && out[n-1] == '\n' {
		out = out[:n-1]
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(out)),
	}, nil
}
