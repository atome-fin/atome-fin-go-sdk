package mock

import (
	"encoding/json"
	"net/http"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/refund"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
)

// typedScenario is the concrete Scenario implementation used by
// the typed builders below (AuthSuccess / RefundFailed / etc).
// It carries:
//
//   - the sync-response status/body the SDK sees,
//   - optionally a `*callback.*Event` + handler key + callback
//     path so a Server with WithAutoCallback can fire the
//     terminal-state callback after the sync response.
//
// Plain Scenarios that aren't typed (AlwaysSuccess etc) DON'T
// implement autoCallbackCarrier — auto-callback firing is a
// no-op for them.
type typedScenario struct {
	respStatus int
	respBody   []byte

	// Callback wiring. callbackEvent == nil → no auto-callback
	// (e.g. PROCESSING outcomes which the spec says never trigger
	// callbacks).
	callbackEvent      any
	callbackHandlerKey string // e.g. "POST /<authNotifyUrl>"
	callbackPath       string // e.g. "/<authNotifyUrl>"
}

// Respond implements Scenario.
func (t *typedScenario) Respond(_ *http.Request) (*http.Response, error) {
	return jsonResponse(t.respStatus, string(t.respBody)), nil
}

// AutoCallback implements autoCallbackCarrier — emits the typed
// builder's pre-baked callback body when the partner enabled
// WithAutoCallback. Returns nil for builders that don't carry a
// callback (PROCESSING outcomes, voidAuth which has no notify URL).
func (t *typedScenario) AutoCallback(_ string, _, _ []byte) *callbackPayload {
	if t.callbackEvent == nil {
		return nil
	}
	body, err := json.Marshal(t.callbackEvent)
	if err != nil {
		return nil
	}
	return &callbackPayload{
		handlerKey: t.callbackHandlerKey,
		path:       t.callbackPath,
		body:       body,
	}
}

// jsonOK is a small helper that JSON-encodes v and packs it
// with a 200 status. Used by every typed Success/Failed builder.
func jsonOK(v any) (int, []byte) {
	body, err := json.Marshal(v)
	if err != nil {
		// Should be unreachable on the in-house types below;
		// fall back to a static SUCCESS envelope.
		return http.StatusOK, []byte(`{"code":"SUCCESS","message":"ok"}`)
	}
	return http.StatusOK, body
}

// ---------- /auth typed builders ----------

// AuthSuccess returns a Scenario that replies SUCCESS for
// /auth and (when WithAutoCallback is wired) fires a
// callback.AuthEvent with Status=SUCCESS to the partner's
// /<authNotifyUrl> handler.
func AuthSuccess(authOrderID string) Scenario {
	resp := &payment.AuthResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.AuthorizationData{
			AuthOrderID: authOrderID,
			Currency:    atomefin.CurrencyIDR,
			Status:      atomefin.StatusSuccess,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp, // AuthEvent = AuthResponse alias
		callbackHandlerKey: "POST /<authNotifyUrl>",
		callbackPath:       "/<authNotifyUrl>",
	}
}

// AuthProcessing returns a Scenario that replies PROCESSING for
// /auth. No callback is fired — the spec says callbacks are
// terminal-only.
func AuthProcessing() Scenario {
	resp := &payment.AuthResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.AuthorizationData{
			Status:   atomefin.StatusProcessing,
			Currency: atomefin.CurrencyIDR,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// AuthFailed returns a Scenario that replies SUCCESS sync but
// FAILED business-status, with the supplied FailureCode. Fires
// the matching FAILED callback.
func AuthFailed(failureCode atomefin.FailureCode) Scenario {
	resp := &payment.AuthResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.AuthorizationData{
			Status:      atomefin.StatusFailed,
			FailureCode: failureCode,
			Currency:    atomefin.CurrencyIDR,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<authNotifyUrl>",
		callbackPath:       "/<authNotifyUrl>",
	}
}

// ---------- /capture typed builders ----------

// CaptureSuccess returns a typed SUCCESS scenario for /capture.
func CaptureSuccess(authOrderID string) Scenario {
	resp := &payment.CaptureResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.CaptureResultData{
			AuthOrderID: authOrderID,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<captureNotifyUrl>",
		callbackPath:       "/<captureNotifyUrl>",
	}
}

// CaptureProcessing returns a typed PROCESSING scenario for
// /capture. No callback fired.
func CaptureProcessing() Scenario {
	resp := &payment.CaptureResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.CaptureResultData{},
	}
	// PROCESSING is encoded by the SDK's nil-status default —
	// CaptureResultData carries Status via embedded PaymentResult;
	// we just emit a minimal envelope and rely on the partner's
	// handler treating "no terminal status" as PROCESSING.
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// CaptureFailed returns a typed FAILED scenario for /capture.
func CaptureFailed(failureCode atomefin.FailureCode) Scenario {
	resp := &payment.CaptureResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.CaptureResultData{},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<captureNotifyUrl>",
		callbackPath:       "/<captureNotifyUrl>",
	}
}

// ---------- /voidAuth typed builders (no callback per spec) ----------

// VoidAuthSuccess returns a typed SUCCESS scenario for /voidAuth.
// /voidAuth has no dedicated notify URL in the 2026-05-06 spec
// snapshot, so this builder never fires an auto-callback.
func VoidAuthSuccess() Scenario {
	resp := &payment.VoidAuthResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.VoidResultData{Status: atomefin.StatusSuccess},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// VoidAuthFailed returns a typed FAILED scenario for /voidAuth.
func VoidAuthFailed(failureCode atomefin.FailureCode) Scenario {
	resp := &payment.VoidAuthResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &payment.VoidResultData{
			Status:      atomefin.StatusFailed,
			FailureCode: failureCode,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// ---------- /refund typed builders ----------

// RefundSuccess returns a typed SUCCESS scenario for /refund.
func RefundSuccess(refundOrderID string) Scenario {
	resp := &refund.RefundResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &refund.RefundResult{
			RefundOrderID: refundOrderID,
			Currency:      atomefin.CurrencyIDR,
			Status:        atomefin.StatusSuccess,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<refundNotifyUrl>",
		callbackPath:       "/<refundNotifyUrl>",
	}
}

// RefundProcessing returns a typed PROCESSING scenario for /refund.
func RefundProcessing() Scenario {
	resp := &refund.RefundResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &refund.RefundResult{
			Currency: atomefin.CurrencyIDR,
			Status:   atomefin.StatusProcessing,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// RefundFailed returns a typed FAILED scenario for /refund.
func RefundFailed(failureCode atomefin.FailureCode) Scenario {
	resp := &refund.RefundResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &refund.RefundResult{
			Currency:    atomefin.CurrencyIDR,
			Status:      atomefin.StatusFailed,
			FailureCode: failureCode,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<refundNotifyUrl>",
		callbackPath:       "/<refundNotifyUrl>",
	}
}

// ---------- /repayment-request typed builders ----------

// RepaymentSuccess returns a typed SUCCESS scenario for
// /repayment-request.
func RepaymentSuccess(repaymentID string) Scenario {
	resp := &repayment.RepaymentResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &repayment.RepaymentResult{
			RepaymentID: repaymentID,
			Currency:    atomefin.CurrencyIDR,
			Status:      atomefin.StatusSuccess,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /repayment-callback",
		callbackPath:       "/repayment-callback",
	}
}

// RepaymentProcessing returns a typed PROCESSING scenario for
// /repayment-request.
func RepaymentProcessing() Scenario {
	resp := &repayment.RepaymentResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &repayment.RepaymentResult{
			Currency: atomefin.CurrencyIDR,
			Status:   atomefin.StatusProcessing,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// RepaymentFailed returns a typed FAILED scenario for
// /repayment-request.
func RepaymentFailed(failureCode atomefin.FailureCode) Scenario {
	resp := &repayment.RepaymentResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &repayment.RepaymentResult{
			Currency: atomefin.CurrencyIDR,
			Status:   atomefin.StatusFailed,
		},
	}
	_ = failureCode // RepaymentResult has no FailureCode field; surface via Code on caller side
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /repayment-callback",
		callbackPath:       "/repayment-callback",
	}
}

// ---------- /credit-application typed builders ----------

// CreditApplicationSuccess returns a typed SUCCESS scenario for
// /credit-application.
func CreditApplicationSuccess() Scenario {
	resp := &credit.CreditApplicationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &credit.CreditApplicationResult{
			Status:   credit.CreditStatus("SUCCESS"),
			Currency: atomefin.CurrencyIDR,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<creditApplicationNotifyUrl>",
		callbackPath:       "/<creditApplicationNotifyUrl>",
	}
}

// CreditApplicationProcessing returns a typed PROCESSING scenario.
func CreditApplicationProcessing() Scenario {
	resp := &credit.CreditApplicationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &credit.CreditApplicationResult{
			Status:   credit.CreditStatus("PROCESSING"),
			Currency: atomefin.CurrencyIDR,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// CreditApplicationFailed returns a typed FAILED scenario.
func CreditApplicationFailed() Scenario {
	resp := &credit.CreditApplicationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
		Data: &credit.CreditApplicationResult{
			Status:   credit.CreditStatus("FAILED"),
			Currency: atomefin.CurrencyIDR,
		},
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus:         status,
		respBody:           body,
		callbackEvent:      resp,
		callbackHandlerKey: "POST /<creditApplicationNotifyUrl>",
		callbackPath:       "/<creditApplicationNotifyUrl>",
	}
}

// ---------- /credit-information typed builders ----------

// CreditInformationSuccess returns a typed SUCCESS scenario for
// /credit-information.
func CreditInformationSuccess() Scenario {
	resp := &credit.CreditInformationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus: status,
		respBody:   body,
		callbackEvent: &callback.CreditInformationEvent{
			Code: atomefin.CodeSuccess, Message: "ok",
			Data: &credit.CreditApplicationCollectQueryResult{
				Status: credit.CreditStatus("SUCCESS"),
			},
		},
		callbackHandlerKey: "POST /<creditInformationNotifyUrl>",
		callbackPath:       "/<creditInformationNotifyUrl>",
	}
}

// CreditInformationProcessing returns a typed PROCESSING scenario.
func CreditInformationProcessing() Scenario {
	resp := &credit.CreditInformationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
	}
	status, body := jsonOK(resp)
	return &typedScenario{respStatus: status, respBody: body}
}

// CreditInformationFailed returns a typed FAILED scenario.
func CreditInformationFailed() Scenario {
	resp := &credit.CreditInformationResponse{
		Code: atomefin.CodeSuccess, Message: "ok",
	}
	status, body := jsonOK(resp)
	return &typedScenario{
		respStatus: status,
		respBody:   body,
		callbackEvent: &callback.CreditInformationEvent{
			Code: atomefin.CodeSuccess, Message: "ok",
			Data: &credit.CreditApplicationCollectQueryResult{
				Status: credit.CreditStatus("FAILED"),
			},
		},
		callbackHandlerKey: "POST /<creditInformationNotifyUrl>",
		callbackPath:       "/<creditInformationNotifyUrl>",
	}
}
