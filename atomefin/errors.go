package atomefin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Error is the umbrella interface implemented by every typed error this SDK
// emits. Callers are expected to use errors.As to recover the concrete type
// (DESIGN.md §6); Temporary() exists for code that just wants to know
// "should I retry this" without caring what kind of error it was.
type Error interface {
	error
	Temporary() bool
}

// APIError is returned when the server replied with a non-2xx HTTP status.
//
// The decoder is intentionally lenient: if the body is one of the known
// envelope shapes (`PaymentStyle400Error`, `Capture400Error`, `Void400Error`,
// `AuthorizationErrorResponse`, `ServerErrorResponse` — DESIGN.md §1.6) the
// `Code` and `Message` fields are populated; otherwise Code is empty and
// callers can still inspect `Raw`. This keeps the SDK forward-compatible
// with future error variants the partner may add without an SDK release.
type APIError struct {
	HTTPStatus int             // 400, 401, 500, ...
	Code       Code            // mapped to the typed Code where possible
	Message    string          // server-supplied human-readable message
	RequestID  string          // server-echoed `requestId` if present in the body
	Endpoint   string          // path that produced the error, e.g. "/auth"
	Raw        json.RawMessage // full body, for diagnostics / unknown shapes
}

// Error implements the error interface.
func (e *APIError) Error() string {
	if e == nil {
		return "<nil *atomefin.APIError>"
	}
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("atomefin: %s %d %s: %s",
			e.Endpoint, e.HTTPStatus, e.Code, e.Message)
	case e.Code != "":
		return fmt.Sprintf("atomefin: %s %d %s",
			e.Endpoint, e.HTTPStatus, e.Code)
	default:
		return fmt.Sprintf("atomefin: %s HTTP %d", e.Endpoint, e.HTTPStatus)
	}
}

// Temporary reports whether the error suggests the caller should retry.
// True for HTTP 500/502/503/504 and for the explicit SERVER_ERROR business
// code; false for 4xx (PARAMS_*, INVALID_SIGNATURE, AUTH_EXPIRED, etc.).
//
// Note that the Client itself already retries 5xx per its RetryPolicy
// before returning APIError to the caller, so a Temporary() == true
// APIError reaching the caller means retries were already exhausted.
func (e *APIError) Temporary() bool {
	if e == nil {
		return false
	}
	if e.Code == CodeServerError {
		return true
	}
	switch e.HTTPStatus {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// IsSignature reports whether the error is the canonical 401 +
// INVALID_SIGNATURE envelope the spec uses for signature failures
// (`AuthorizationErrorResponse`). Cheap helper — callers can also use
// errors.As + check Code manually.
func (e *APIError) IsSignature() bool {
	return e != nil && e.HTTPStatus == http.StatusUnauthorized && e.Code == CodeInvalidSignature
}

// TransportError is returned when the request never reached a defined
// HTTP-status state — DNS failure, connection refused, TLS error, EOF
// mid-response, body-read failure, or a context cancellation that beat
// the server's reply.
type TransportError struct {
	Op    string // "build", "do", "read", "marshal", "unmarshal"
	URL   string // best-effort; may be empty if URL was not yet built
	Err   error  // underlying error (always wrapped, use errors.Unwrap)
	Retry bool   // the policy considered this retryable; retries are exhausted
}

func (e *TransportError) Error() string {
	if e == nil || e.Err == nil {
		return "atomefin: transport error"
	}
	if e.URL != "" {
		return fmt.Sprintf("atomefin: transport %s %s: %v", e.Op, e.URL, e.Err)
	}
	return fmt.Sprintf("atomefin: transport %s: %v", e.Op, e.Err)
}

// Unwrap exposes the underlying error to errors.Is / errors.As.
func (e *TransportError) Unwrap() error { return e.Err }

// Temporary reports whether the policy considered this retryable. Since the
// Client retries internally per RetryPolicy, a TransportError reaching the
// caller with Temporary() == true means retries were exhausted (e.g. the
// final attempt also failed) and the caller may still choose to back off
// further at a higher layer.
func (e *TransportError) Temporary() bool { return e != nil && e.Retry }

// IsRetryableTransport reports whether err is a transport-level failure
// that policy-level code should retry. Exposed for callers / tests; used
// internally by RetryPolicy.RetryOnTransportError.
func IsRetryableTransport(err error) bool {
	if err == nil {
		return false
	}
	// context cancellation propagates the parent's intent — never retry.
	if errors.Is(err, context.Canceled) {
		return false
	}
	// context.DeadlineExceeded is retryable only if the parent ctx is alive,
	// which the caller (not us) decides. We classify it as retryable here;
	// the caller's parent-ctx Done channel will short-circuit us anyway.
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		// net.Error.Timeout has been the canonical retry signal since Go 1.6.
		return ne.Timeout() || isProbablyTransient(err)
	}
	// Errors not implementing net.Error (EOF on body read, etc.) are
	// retryable iff the verb was idempotent — and POSTing the same
	// requestId is safe per DESIGN.md §1.4 — so default to true.
	return true
}

// isProbablyTransient is a hook for future heuristics (e.g. matching
// "connection reset by peer" strings on platforms that don't surface a
// typed error). Today it is intentionally conservative.
func isProbablyTransient(err error) bool {
	_ = err
	return true
}

// SignatureError is returned when the signing path itself fails — typically
// a malformed PEM (caught at Client construction by the sign package
// loaders), an empty / undecodable signature on a verifier path, or a
// missing required header. Distinct from APIError{Code: INVALID_SIGNATURE}
// which is the *server* rejecting a signature we produced.
type SignatureError struct {
	Reason string // "sign", "verify", "decode", "missing-header"
	Err    error
}

func (e *SignatureError) Error() string {
	if e == nil {
		return "atomefin: signature error"
	}
	if e.Err == nil {
		return fmt.Sprintf("atomefin: signature %s", e.Reason)
	}
	return fmt.Sprintf("atomefin: signature %s: %v", e.Reason, e.Err)
}

func (e *SignatureError) Unwrap() error { return e.Err }

// Temporary is always false: a signature-layer failure indicates a
// configuration bug or a malformed input; retrying with the same key/body
// will fail identically.
func (e *SignatureError) Temporary() bool { return false }

// ValidationError is raised before transmission when client-side validation
// rejects a request — the signer is unconfigured, the body exceeded a
// maxLength constraint, an enum was out of range, etc. T2 ships a minimal
// validator (just nil/empty checks at construction time); the broader
// per-field validator surfaces in T3 as the typed structs land.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return "atomefin: validation error"
	}
	if e.Field != "" {
		return fmt.Sprintf("atomefin: validation: %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("atomefin: validation: %s", e.Message)
}

// Temporary is always false: client-side validation failures are not
// retryable.
func (e *ValidationError) Temporary() bool { return false }

// errorEnvelope is the union of fields extracted from any of the spec's
// error envelopes. Decoding is lenient — fields the body doesn't carry stay
// at their zero value.
type errorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RequestID string `json:"requestId"`
	} `json:"data"`
}

// decodeAPIError builds an APIError from a non-2xx HTTP response. body is
// the already-read response body (may be empty); endpoint is the request
// path. The function never returns nil.
func decodeAPIError(status int, endpoint string, body []byte) *APIError {
	e := &APIError{HTTPStatus: status, Endpoint: endpoint, Raw: json.RawMessage(append([]byte(nil), body...))}
	if len(body) == 0 {
		return e
	}
	var env errorEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		e.Code = Code(env.Code)
		e.Message = env.Message
		e.RequestID = env.Data.RequestID
	}
	return e
}

// Compile-time interface assertions: every typed error must implement
// Error (with Temporary()).
var (
	_ Error = (*APIError)(nil)
	_ Error = (*TransportError)(nil)
	_ Error = (*SignatureError)(nil)
	_ Error = (*ValidationError)(nil)
)
