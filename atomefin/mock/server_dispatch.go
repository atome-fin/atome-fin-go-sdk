package mock

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	specpkg "github.com/atome-fin/atome-fin-go-sdk/internal/spec"
)

// serveHTTP is the v0.5 dispatch pipeline. The request flow:
//
//  1. Read the body once (recording side).
//  2. WithSpecValidation: validate header / query / body
//     presence against the pinned spec; reject with
//     400 PARAMS_MISSING on any miss. (Encrypted bodies skip
//     body validation — same rule as qa/specserver.)
//  3. WithIdempotency: extract requestId, look up the
//     `(method, path, requestId)` cache; replay on hit.
//  4. Dispatch to the active Scenario.
//  5. WithResponseSigning: sign the response body and emit
//     `Authorization: <sig>` on the wire.
//  6. WithAutoCallback: fire the matching `*Event` to the
//     partner's handler (in-process or via WithCallbackURL).
//
// Steps 2/3/5/6 are no-ops when their respective ServerOption
// is not set, so a v0.4-shape `mock.NewServer(t, scenario)` call
// runs the same code path it always has.
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// Capture body once (idempotency + spec validation may both
	// inspect it; the Transport recording also wants it).
	body := readAndReset(r)
	r.Body = newNopCloserBytes(body)

	op := specpkg.OpKey(r.Method, r.URL.Path)

	// ---- Step 2: spec validation ----
	if s.cfg.specValidation {
		if status, code, msg, ok := s.specValidate(r, body); !ok {
			writeJSONErrorMock(w, status, code, msg)
			return
		}
	}

	// ---- Step 3: idempotency replay ----
	var idemKey string
	if s.idem != nil {
		idemKey = s.idemKey(op, r, body)
		if idemKey != "" {
			if hit := s.idem.Get(idemKey); hit != nil {
				replay(w, hit)
				return
			}
		}
	}

	// ---- Step 4: scenario dispatch via Transport ----
	resp, err := s.transport.RoundTrip(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	// ---- Step 5: response signing (forward-compat) ----
	if len(s.cfg.responseSigningKeyPEM) > 0 {
		if authz, err := signResponseBody(r.Context(), s.cfg.responseSigningKeyPEM, respBody); err == nil {
			resp.Header.Set("Authorization", authz)
		} else {
			s.tb.Errorf("mock.Server: WithResponseSigning: %v", err)
		}
	}

	// Stamp idempotency cache before writing — partners replaying
	// in concurrent goroutines see consistent state.
	if s.idem != nil && idemKey != "" {
		s.idem.Put(idemKey, &cacheEntry{
			key:     idemKey,
			status:  resp.StatusCode,
			body:    append([]byte(nil), respBody...),
			headers: resp.Header.Clone(),
		})
	}

	// Write the response.
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)

	// ---- Step 6: auto-callback firing ----
	if len(s.cfg.autoCallbackHandlers) > 0 || s.cfg.autoCallbackURL != "" {
		s.fireAutoCallback(r.Context(), op, body, respBody)
	}
}

// specValidate runs the v0.5 pre-step against the pinned spec.
// Returns (status, code, message, ok) — when ok=false, the
// caller writes the canonical PARAMS_MISSING / WRONG_PARAMS_FORMAT
// envelope.
func (s *Server) specValidate(r *http.Request, body []byte) (int, string, string, bool) {
	spec, err := specpkg.LoadDefault()
	if err != nil {
		s.tb.Errorf("mock.Server: WithSpecValidation: load: %v", err)
		return 0, "", "", true // fail-open on internal error
	}
	op, ok := spec.Op(r.Method, r.URL.Path)
	if !ok {
		// Path not in spec — pass through; partners may exercise
		// off-spec endpoints intentionally (404 / 410 testing).
		return 0, "", "", true
	}
	encrypted := false
	for _, name := range op.RequiredHeader {
		if r.Header.Get(name) == "" {
			return http.StatusBadRequest, "PARAMS_MISSING",
				fmt.Sprintf("missing required header %q", name), false
		}
		if name == "Encrypt" {
			encrypted = true
		}
	}
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		for _, name := range op.RequiredQuery {
			if q.Get(name) == "" {
				return http.StatusBadRequest, "PARAMS_MISSING",
					fmt.Sprintf("missing required query param %q", name), false
			}
		}
		return 0, "", "", true
	}
	if r.Method == http.MethodPost && !encrypted {
		missing, ferr := specpkg.ValidateBody(body, op.RequiredBody)
		if ferr != nil {
			return http.StatusBadRequest, "WRONG_PARAMS_FORMAT", ferr.Error(), false
		}
		if missing != "" {
			return http.StatusBadRequest, "PARAMS_MISSING",
				fmt.Sprintf("missing required body field %q", missing), false
		}
	}
	return 0, "", "", true
}

// idemKey builds the `(method, path, requestId)` cache key.
// Returns "" when no requestId can be extracted (cache miss
// becomes a normal dispatch).
//
// v0.6.0: encrypted POSTs are no longer a hard bypass — when
// WithIdempotencyDecryptKey is configured, the Server unwraps
// the per-request AES key, decrypts the body, and extracts
// requestId from the plaintext. Without the decrypt key the
// v0.5.0 bypass behaviour is preserved.
func (s *Server) idemKey(op string, r *http.Request, body []byte) string {
	encryptHeader := r.Header.Get("Encrypt")
	if encryptHeader != "" {
		// Encrypted POST — try v0.6 decrypt-then-extract path.
		if len(s.cfg.idempotencyDecryptPEM) == 0 {
			return ""
		}
		rid := s.requestIDFromEncryptedBody(encryptHeader, body)
		if rid == "" {
			return ""
		}
		return op + "::" + rid
	}
	switch r.Method {
	case http.MethodGet:
		if rid := r.URL.Query().Get("requestId"); rid != "" {
			return op + "::" + rid
		}
	case http.MethodPost:
		var blob struct {
			RequestID string `json:"requestId"`
		}
		if err := json.Unmarshal(body, &blob); err == nil && blob.RequestID != "" {
			return op + "::" + blob.RequestID
		}
	}
	return ""
}

// requestIDFromEncryptedBody decrypts an inbound hybrid-encrypted
// body and returns its top-level `requestId`. Used by idemKey()
// to support the v0.6 encrypted-POST idempotency path. Returns
// "" on any decryption / parse failure (caller falls back to
// cache-miss dispatch).
func (s *Server) requestIDFromEncryptedBody(encryptHeader string, body []byte) string {
	priv, err := sign.LoadPrivateKeyPEM(s.cfg.idempotencyDecryptPEM)
	if err != nil {
		s.tb.Errorf("mock.Server: WithIdempotencyDecryptKey: load: %v", err)
		return ""
	}
	plain, err := encrypt.Unmarshal(encryptHeader, string(body), priv)
	if err != nil {
		// Don't fail the test — partners may intentionally drive
		// malformed-encrypt failure paths.
		return ""
	}
	var blob struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(plain, &blob); err != nil {
		return ""
	}
	return blob.RequestID
}

// fireAutoCallback dispatches the matching `*Event` to the
// partner's handler (in-process if WithAutoCallback was given,
// network if WithCallbackURL was given).
//
// v0.5 covers the typed-builder Path-A flow: when the active
// Scenario carries a *callbackPayload (set by the typed
// scenario builders shipped in scenarios.go's typed
// constructors), use that as the event body. Otherwise the
// auto-callback fire is skipped — there's no reasonable
// inferred payload.
//
// ctx is the inbound request's context — threaded into the
// signer so a cancelled / deadlined request doesn't waste work
// on a sign call that will never be observed (v0.5.2 fix).
func (s *Server) fireAutoCallback(ctx context.Context, op string, reqBody, respBody []byte) {
	scenario := s.transport.snapshotScenario()
	carrier, ok := scenario.(autoCallbackCarrier)
	if !ok {
		// Plain Scenario (e.g. AlwaysSuccess) — no event to fire.
		return
	}
	payload := carrier.AutoCallback(op, reqBody, respBody)
	if payload == nil {
		return
	}
	if s.cfg.autoCallbackDelay > 0 {
		time.Sleep(s.cfg.autoCallbackDelay)
	}
	if h, ok := s.cfg.autoCallbackHandlers[normalizeOpKey(payload.handlerKey)]; ok {
		s.fireCallbackInProcess(ctx, h, payload)
		return
	}
	if s.cfg.autoCallbackURL != "" {
		s.fireCallbackToURL(ctx, payload)
	}
}

// fireCallbackInProcess shares the v0.4 fire() signing core
// (sign body → POST via ServeHTTP). Panic-isolated since v0.6.0:
// a partner-side handler that panics during ServeHTTP no
// longer propagates into the SDK request pipeline. Mirrors the
// Client.safeObsRequest pattern around Observer hooks. The
// recovered panic is surfaced via tb.Errorf so a misbehaving
// handler is visible at test time but the inbound SDK request
// completes its synchronous dispatch.
func (s *Server) fireCallbackInProcess(ctx context.Context, h http.Handler, payload *callbackPayload) {
	body, authz, err := s.signCallback(ctx, payload.body)
	if err != nil {
		s.tb.Errorf("mock.Server: auto-callback sign: %v", err)
		return
	}
	req := httptest.NewRequest(http.MethodPost, payload.path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authz)
	rec := httptest.NewRecorder()

	defer func() {
		if r := recover(); r != nil {
			s.tb.Errorf("mock.Server: auto-callback handler panicked: %v", r)
		}
	}()
	h.ServeHTTP(rec, req)
}

func (s *Server) fireCallbackToURL(ctx context.Context, payload *callbackPayload) {
	body, authz, err := s.signCallback(ctx, payload.body)
	if err != nil {
		s.tb.Errorf("mock.Server: auto-callback sign: %v", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.autoCallbackURL, bytes.NewReader(body))
	if err != nil {
		s.tb.Errorf("mock.Server: auto-callback build request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authz)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.tb.Errorf("mock.Server: auto-callback POST: %v", err)
		return
	}
	_ = resp.Body.Close()
}

// signCallback applies the shared sign-then-dispatch core: signs
// `body` with WithAutoCallbackKey or the bundled mock signing key
// and returns (body, "<base64sig>", err). Routes through
// signBodyWithPEM (v0.6.0 consolidation — see sign_helper.go).
// ctx is threaded so a cancelled / deadlined inbound request
// short-circuits the sign call rather than racing it (v0.5.2 fix).
func (s *Server) signCallback(ctx context.Context, body []byte) ([]byte, string, error) {
	keyPEM := s.cfg.autoCallbackSignerPEM
	if len(keyPEM) == 0 {
		keyPEM = MockSigningPrivKeyPEM()
	}
	authz, err := signBodyWithPEM(ctx, body, keyPEM)
	if err != nil {
		return nil, "", err
	}
	return body, authz, nil
}

// signResponseBody wraps body with an RSA-PKCS#1 v1.5 SHA-256
// signature using the supplied PEM. Returns the base64
// signature suitable for the Authorization header. ctx is
// threaded from the inbound request (v0.5.2 fix). Routes
// through signBodyWithPEM (v0.6.0 consolidation).
func signResponseBody(ctx context.Context, privPEM, body []byte) (string, error) {
	return signBodyWithPEM(ctx, body, privPEM)
}

// replay writes the cached entry verbatim.
func replay(w http.ResponseWriter, e *cacheEntry) {
	for k, v := range e.headers {
		w.Header()[k] = v
	}
	w.Header().Set("X-Mock-Replay", "1")
	w.WriteHeader(e.status)
	_, _ = w.Write(e.body)
}

// writeJSONErrorMock mirrors the qa/specserver shape — kept as a
// dedicated helper here so the mock package has zero qa/
// imports.
func writeJSONErrorMock(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}

// snapshotScenario returns the currently-active Scenario without
// holding the Transport's mutex. Tiny accessor needed by
// fireAutoCallback (which can't import Transport's private
// scenario field).
func (t *Transport) snapshotScenario() Scenario {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scenario
}

// callbackPayload is the data an autoCallbackCarrier produces.
// `body` is the JSON bytes the partner-side handler will see;
// `handlerKey` is the "POST /<authNotifyUrl>" shape used to
// look up the handler in WithAutoCallback's map; `path` is the
// URL path (sans baseURL) used in the signed request.
type callbackPayload struct {
	handlerKey string
	path       string
	body       []byte
}

// autoCallbackCarrier is the optional capability typed scenarios
// implement to drive auto-callback firing. Vanilla
// AlwaysSuccess / AlwaysAPIError / etc don't implement this so
// auto-callback firing is a no-op for them — only typed
// builders (`mock.AuthSuccess`, etc, in scenarios.go) carry the
// matching `*Event` payload.
type autoCallbackCarrier interface {
	AutoCallback(op string, reqBody, respBody []byte) *callbackPayload
}

// noopAutoCallback assertion — keeps the type used.
var _ autoCallbackCarrier = (*typedScenario)(nil)

// Force the strings package referenced by other files in the
// package to have at least one referring use even if helpers
// shrink — keeps go vet happy under aggressive minimisation.
var _ = strings.ToUpper
