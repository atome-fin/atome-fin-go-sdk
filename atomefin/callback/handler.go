package callback

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// AuthorizationHeader is the canonical name of the header carrying the
// inbound signature. The spec uses the exact string "Authorization"
// (DESIGN.md §1.3) — net/http's CanonicalHeaderKey already lower-cases
// this consistently so we don't need a normalization step.
const AuthorizationHeader = "Authorization"

// AuthHandlerFunc is the user-supplied callback for /authNotifyUrl
// events. The handler returns nil to ack the callback (HTTP 200) or
// any error to force Atome to retry (HTTP 500).
type AuthHandlerFunc func(ctx context.Context, event *AuthEvent) error

// CaptureHandlerFunc is the user-supplied callback for /captureNotifyUrl
// events.
type CaptureHandlerFunc func(ctx context.Context, event *CaptureEvent) error

// AuthHandler returns an http.Handler that:
//   - reads the body via io.LimitReader (Verifier.BodyLimit, default
//     1 MiB),
//   - verifies the Authorization header against the raw body using
//     Verifier (multi-cert succeeds if any key verifies),
//   - decodes the body into *AuthEvent,
//   - invokes fn,
//   - emits the ack envelope with the appropriate HTTP status.
//
// Atome retries on HTTP non-2xx; partners are responsible for keeping
// fn idempotent (callbacks are at-least-once). See package docs.
func AuthHandler(v *Verifier, fn AuthHandlerFunc) http.Handler {
	if fn == nil {
		// Surface the nil user-fn before we wrap it in the typed
		// adapter — otherwise the closure would be non-nil and
		// `handle`'s nil-fn guard wouldn't fire.
		return handle[AuthEvent](v, nil)
	}
	return handle[AuthEvent](v, func(ctx context.Context, e *AuthEvent) error {
		return fn(ctx, e)
	})
}

// CaptureHandler is the /captureNotifyUrl counterpart of AuthHandler.
func CaptureHandler(v *Verifier, fn CaptureHandlerFunc) http.Handler {
	if fn == nil {
		return handle[CaptureEvent](v, nil)
	}
	return handle[CaptureEvent](v, func(ctx context.Context, e *CaptureEvent) error {
		return fn(ctx, e)
	})
}

// handle is the shared implementation behind AuthHandler and
// CaptureHandler. Generic so the typed event reaches the user's
// handler without an interface{} dance.
func handle[T any](v *Verifier, fn func(context.Context, *T) error) http.Handler {
	if v == nil {
		// Returning a non-nil handler that always 500s gives partners
		// a clear error path during integration; panicking would crash
		// the server they registered the handler with.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeAck(w, http.StatusInternalServerError,
				AckServerError("callback handler missing Verifier"))
		})
	}
	if fn == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writeAck(w, http.StatusInternalServerError,
				AckServerError("callback handler missing user function"))
		})
	}
	limit := v.bodyLimit
	if limit <= 0 {
		limit = DefaultBodyLimit
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per DESIGN.md §8 + spec, callbacks are POST. Other verbs
		// fall straight to 405 with no signature work.
		if r.Method != http.MethodPost {
			writeAck(w, http.StatusMethodNotAllowed,
				AckParamsWrong("method not allowed"))
			return
		}

		body, err := readLimited(r.Body, limit)
		if err != nil {
			status := http.StatusBadRequest
			writeAck(w, status, AckParamsWrong(err.Error()))
			return
		}

		// Signature header MUST exist. Empty signatures are surfaced as
		// 401 INVALID_SIGNATURE (not 400) so the partner sees the same
		// error class regardless of whether the header is malformed or
		// outright wrong.
		sig := r.Header.Get(AuthorizationHeader)
		if sig == "" {
			writeAck(w, http.StatusUnauthorized, AckBadSignature())
			return
		}

		if vErr := v.Verify(r.Context(), body, sig); vErr != nil {
			writeAck(w, http.StatusUnauthorized, AckBadSignature())
			return
		}

		// Decode AFTER verification — the bytes signed must equal the
		// bytes verified, and decoding via UseNumber catches a
		// callback partner that injects an `originalAmount: 1.5`
		// loudly rather than silently rounding.
		event, err := strictDecode[T](body)
		if err != nil {
			writeAck(w, http.StatusBadRequest,
				AckParamsWrong(fmt.Sprintf("decode failed: %v", err)))
			return
		}

		if err := fn(r.Context(), event); err != nil {
			// Atome retries on non-2xx — surface user errors via 500.
			// Reason is included for partner debugging; keep it short
			// because Atome may log this verbatim.
			writeAck(w, http.StatusInternalServerError,
				AckServerError(err.Error()))
			return
		}

		writeAck(w, http.StatusOK, AckSuccess())
	})
}

// readLimited reads up to limit+1 bytes from r and rejects when the
// body exceeds the limit. r is always closed before return.
func readLimited(r io.ReadCloser, limit int64) ([]byte, error) {
	if r == nil {
		return nil, errors.New("missing request body")
	}
	defer r.Close()
	buf, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(buf)) > limit {
		return nil, fmt.Errorf("body exceeds %d bytes", limit)
	}
	return buf, nil
}

// strictDecode decodes b into a fresh *T with DisallowUnknownFields
// and json.Number, mirroring the qa/marshal harness's invariants. The
// stricter decoding catches a malformed callback (or a spec evolution
// the SDK hasn't been rebuilt for) loudly.
func strictDecode[T any](b []byte) (*T, error) {
	out := new(T)
	dec := json.NewDecoder(bytes.NewReader(b))
	// DisallowUnknownFields is intentionally OFF on the inbound path:
	// the spec is draft (DESIGN.md §12) and the callback is the place
	// most likely to add a field before the SDK rebuilds. Round-trip
	// invariants live in qa/marshal_audit_test.go for the strict
	// case; runtime callbacks are forgiving.
	if err := dec.Decode(out); err != nil {
		return nil, err
	}
	return out, nil
}

// writeAck encodes ack via atomefin.MarshalSigning (HTML-escape OFF
// for parity with the request-side) and writes the response. We use
// MarshalSigning even though this body is not signed so the bytes
// match what a future signed-ack scheme (Q5 / replay protection) would
// produce.
func writeAck(w http.ResponseWriter, status int, ack AckResponse) {
	body, err := atomefin.MarshalSigning(ack)
	if err != nil {
		// Should not happen for a tiny struct of two strings. Fall
		// back to a hard-coded body so the partner still gets a clean
		// HTTP response.
		body = []byte(`{"code":"SERVER_ERROR","message":"ack marshal failed"}`)
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
