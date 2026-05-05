package atomefin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestAPIErrorTemporary(t *testing.T) {
	cases := []struct {
		name string
		e    *APIError
		want bool
	}{
		{"500", &APIError{HTTPStatus: 500}, true},
		{"502", &APIError{HTTPStatus: 502}, true},
		{"503", &APIError{HTTPStatus: 503}, true},
		{"504", &APIError{HTTPStatus: 504}, true},
		{"server-error-code-on-200", &APIError{HTTPStatus: 200, Code: CodeServerError}, true},
		{"400", &APIError{HTTPStatus: 400}, false},
		{"401-invalid-sig", &APIError{HTTPStatus: 401, Code: CodeInvalidSignature}, false},
		{"nil", nil, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.e.Temporary(); got != tt.want {
				t.Errorf("Temporary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAPIErrorErrorString(t *testing.T) {
	cases := []struct {
		name     string
		e        *APIError
		mustHave []string
	}{
		{
			name: "code+msg",
			e: &APIError{
				HTTPStatus: 400, Code: CodeParamsMissing, Message: "requestId is required", Endpoint: "/auth",
			},
			mustHave: []string{"/auth", "400", "PARAMS_MISSING", "requestId is required"},
		},
		{
			name:     "code-only",
			e:        &APIError{HTTPStatus: 401, Code: CodeInvalidSignature, Endpoint: "/auth"},
			mustHave: []string{"/auth", "401", "INVALID_SIGNATURE"},
		},
		{
			name:     "no-code",
			e:        &APIError{HTTPStatus: 502, Endpoint: "/capture"},
			mustHave: []string{"/capture", "502"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.e.Error()
			for _, want := range tt.mustHave {
				if !strings.Contains(s, want) {
					t.Errorf("Error() = %q; missing %q", s, want)
				}
			}
		})
	}
}

func TestAPIErrorIsSignature(t *testing.T) {
	yes := &APIError{HTTPStatus: 401, Code: CodeInvalidSignature}
	if !yes.IsSignature() {
		t.Error("expected IsSignature() == true for 401 INVALID_SIGNATURE")
	}
	noStatus := &APIError{HTTPStatus: 400, Code: CodeInvalidSignature}
	if noStatus.IsSignature() {
		t.Error("expected IsSignature() == false for non-401")
	}
	noCode := &APIError{HTTPStatus: 401, Code: CodeServerError}
	if noCode.IsSignature() {
		t.Error("expected IsSignature() == false for non-INVALID_SIGNATURE code")
	}
}

func TestTransportErrorUnwrapAndTemporary(t *testing.T) {
	inner := io.EOF
	te := &TransportError{Op: "do", URL: "http://x", Err: inner, Retry: true}
	if !errors.Is(te, io.EOF) {
		t.Error("errors.Is(te, io.EOF) = false, want true")
	}
	if !te.Temporary() {
		t.Error("Temporary() = false, want true")
	}
	if !strings.Contains(te.Error(), "http://x") {
		t.Errorf("Error() = %q; missing URL", te.Error())
	}
}

func TestSignatureErrorTemporary(t *testing.T) {
	se := &SignatureError{Reason: "sign", Err: errors.New("boom")}
	if se.Temporary() {
		t.Error("SignatureError.Temporary() = true, want false")
	}
	if !errors.Is(se, errors.Unwrap(se)) {
		t.Error("Unwrap chain broken")
	}
}

func TestValidationErrorTemporary(t *testing.T) {
	ve := &ValidationError{Field: "x", Message: "bad"}
	if ve.Temporary() {
		t.Error("ValidationError.Temporary() = true, want false")
	}
	if !strings.Contains(ve.Error(), "x") {
		t.Errorf("Error() = %q; missing field name", ve.Error())
	}
}

func TestIsRetryableTransport(t *testing.T) {
	if IsRetryableTransport(nil) {
		t.Error("nil should not be retryable")
	}
	if IsRetryableTransport(context.Canceled) {
		t.Error("context.Canceled should not be retryable")
	}
	if !IsRetryableTransport(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded should be retryable")
	}
	if !IsRetryableTransport(io.EOF) {
		t.Error("EOF should be retryable")
	}

	// net.Error timeout
	te := &timeoutError{}
	if !IsRetryableTransport(te) {
		t.Error("net.Error.Timeout() should be retryable")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

var _ net.Error = timeoutError{}

func TestDecodeAPIErrorEnvelope(t *testing.T) {
	body := []byte(`{"code":"PARAMS_MISSING","message":"requestId required","data":{"requestId":"r-1"}}`)
	e := decodeAPIError(http.StatusBadRequest, "/auth", body)
	if e == nil {
		t.Fatal("decodeAPIError returned nil")
	}
	if e.HTTPStatus != http.StatusBadRequest {
		t.Errorf("HTTPStatus = %d, want %d", e.HTTPStatus, http.StatusBadRequest)
	}
	if e.Code != CodeParamsMissing {
		t.Errorf("Code = %q, want %q", e.Code, CodeParamsMissing)
	}
	if e.Message != "requestId required" {
		t.Errorf("Message = %q", e.Message)
	}
	if e.RequestID != "r-1" {
		t.Errorf("RequestID = %q, want r-1", e.RequestID)
	}
	if !json.Valid(e.Raw) {
		t.Error("Raw is not valid JSON")
	}
}

func TestDecodeAPIErrorMalformed(t *testing.T) {
	// Body that isn't JSON should still produce a usable APIError, just
	// with empty code/message and the raw bytes preserved.
	body := []byte("not json")
	e := decodeAPIError(http.StatusInternalServerError, "/capture", body)
	if e == nil {
		t.Fatal("decodeAPIError returned nil")
	}
	if e.HTTPStatus != http.StatusInternalServerError {
		t.Errorf("HTTPStatus = %d", e.HTTPStatus)
	}
	if e.Code != "" {
		t.Errorf("Code should be empty for malformed body, got %q", e.Code)
	}
	if string(e.Raw) != "not json" {
		t.Errorf("Raw = %q", string(e.Raw))
	}
}

func TestErrorInterfaceImplementations(t *testing.T) {
	var _ Error = (*APIError)(nil)
	var _ Error = (*TransportError)(nil)
	var _ Error = (*SignatureError)(nil)
	var _ Error = (*ValidationError)(nil)
}
