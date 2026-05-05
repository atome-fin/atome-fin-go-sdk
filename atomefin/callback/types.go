package callback

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// AuthEvent is the JSON shape posted to the partner's auth-callback
// endpoint. It is byte-for-byte the same envelope as the synchronous
// /auth response (DESIGN.md §8 — partners don't learn a parallel
// schema), so we re-export the payment package's AuthResponse type
// rather than redeclaring it.
type AuthEvent = payment.AuthResponse

// CaptureEvent is the capture-callback envelope. Same JSON shape as
// CaptureResponse.
type CaptureEvent = payment.CaptureResponse

// AckResponse is the body the partner returns to Atome on every
// callback. Atome treats `code: SUCCESS` as confirmation; any other
// code (or HTTP non-2xx) is treated as a delivery failure that is
// eligible for retry.
type AckResponse struct {
	Code    atomefin.Code `json:"code"`
	Message string        `json:"message,omitempty"`
}

// AckSuccess is the canonical 200/SUCCESS ack the handler emits when
// the user-supplied function returns nil.
func AckSuccess() AckResponse {
	return AckResponse{Code: atomefin.CodeSuccess, Message: "ack"}
}

// AckBadSignature is the body emitted alongside HTTP 401 when the
// signature does not verify against any configured cert.
func AckBadSignature() AckResponse {
	return AckResponse{Code: atomefin.CodeInvalidSignature, Message: "signature did not verify"}
}

// AckServerError is the body emitted alongside HTTP 500 when the
// user-supplied handler returns an error. Atome will retry.
func AckServerError(reason string) AckResponse {
	if reason == "" {
		reason = "internal handler error"
	}
	return AckResponse{Code: atomefin.CodeServerError, Message: reason}
}

// AckParamsWrong is the body emitted alongside HTTP 400 for malformed
// requests (oversize body, undecodable JSON, missing Authorization
// header in the unauthenticated path before the verifier is reached).
func AckParamsWrong(reason string) AckResponse {
	if reason == "" {
		reason = "request rejected"
	}
	return AckResponse{Code: atomefin.CodeWrongParamsFormat, Message: reason}
}
