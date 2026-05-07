package atomefin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// RawResponse is the unparsed response handed back from Client.DoSigned on
// 2xx. The body has already been read, the connection has been closed, and
// the headers are a defensive clone — partners can pass a RawResponse
// across goroutines without worrying about reuse.
type RawResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// DoSignedOption modifies a single DoSigned call without affecting Client
// configuration. Used for per-request headers — most prominently the
// `sessionid` header that DESIGN.md §1.3 requires on /auth — and reserved
// for the timestamp/nonce headers that Q5 may add.
//
// Implemented as a small interface (rather than `func(*doSignedConfig)`)
// so the zero-value semantics match the rest of the SDK's option types.
type DoSignedOption interface{ applyDoSigned(*doSignedConfig) }

type doSignedConfig struct {
	extraHeaders http.Header
}

type extraHeaderOpt struct{ key, value string }

func (e extraHeaderOpt) applyDoSigned(c *doSignedConfig) {
	if c.extraHeaders == nil {
		c.extraHeaders = http.Header{}
	}
	c.extraHeaders.Set(e.key, e.value)
}

// WithRequestHeader adds (or overrides) a single HTTP header on the
// outgoing request. Use it for the spec's per-call headers: the
// `sessionid` header on /auth, and (once Q5 closes) a replay-prevention
// timestamp/nonce. Partner-controlled headers cannot override the
// SDK-controlled `Authorization` / `Content-Type` / `User-Agent`.
func WithRequestHeader(key, value string) DoSignedOption {
	return extraHeaderOpt{key: key, value: value}
}

// DoSigned signs `body`, sends it as a POST to <baseURL><path>, and returns
// the response. It is the single transport entry point used by the
// payment service and (in T4) by any service that needs a signed,
// retried, observability-instrumented HTTP call.
//
// Behaviour:
//   - method must be POST today; v1 of the spec has zero GET endpoints
//
// . Other methods return *ValidationError.
//   - body is the canonical signing input (DESIGN.md §4): the bytes
//     transmitted are exactly the bytes signed; no whitespace
//     normalization, no re-marshalling. Caller is responsible for the
//     marshal step.
//   - On 2xx: returns (*RawResponse, nil). Caller decodes the JSON.
//   - On 4xx/5xx (after retries): returns (nil, *APIError) with the
//     canonical envelope decoded where possible.
//   - On transport / signing failure: returns (nil, *TransportError) or
//     (nil, *SignatureError).
//
// op is a short identifier (typically the path, e.g. "/auth") used in
// Observer hooks and log lines.
func (c *Client) DoSigned(ctx context.Context, method, path string, body []byte, opts ...DoSignedOption) (*RawResponse, error) {
	if c == nil {
		return nil, errors.New("atomefin: DoSigned called on nil *Client")
	}
	var cfg doSignedConfig
	for _, o := range opts {
		if o != nil {
			o.applyDoSigned(&cfg)
		}
	}
	if method != http.MethodPost {
		// DoSigned is the POST entry point. GET requests use DoSignedGET
		// (different signing canonical: the alphabetically-sorted query
		// string per spec §Signature, not the body).
		return nil, &ValidationError{Field: "method", Message: "DoSigned is POST-only; use DoSignedGET for GET requests"}
	}
	if path == "" || path[0] != '/' {
		return nil, &ValidationError{Field: "path", Message: "path must be absolute (start with '/')"}
	}

	urlStr := c.baseURL + path
	return c.signAndDispatch(ctx, path, urlStr, body, cfg, func(reqCtx context.Context) (*http.Request, error) {
		// Fresh body reader per attempt — bytes.NewReader is single-use.
		return http.NewRequestWithContext(reqCtx, method, urlStr, bytes.NewReader(body))
	})
}

// DoSignedGET signs and sends a GET request to <baseURL><path> with the
// given query parameters. Mirror of DoSigned for the GET branch of the
// spec's signature section:
//
//	"GET: Sign the request parameters which parameter names are sorted
//	 in alphabetical natural order"
//
// (DESIGN.md §1.3 verbatim quote, §5 canonical rules).
//
// Behaviour:
//   - The signing canonical is sign.CanonicalQuery(query) — keys in
//     lexicographic order, RFC 3986 percent-encoding (space → %20, NOT
//     +). The Signer is fed those bytes; the wire RawQuery is set to
//     the SAME bytes verbatim so server-side reconstruction (parse +
//     re-canonicalise) verifies against the partner's signature
//     byte-for-byte.
//   - opts pass through identically to DoSigned (sessionid headers,
//     custom traces, etc.). Reserved headers
//     (Authorization/Content-Type/User-Agent/Accept) cannot be
//     overridden.
//   - On 2xx: returns (*RawResponse, nil).
//   - On 4xx/5xx (after retries): returns (nil, *APIError).
//   - On transport / signing failure: (nil, *TransportError) or
//     (nil, *SignatureError).
//   - Same retry / observer / body-cap pipeline as DoSigned. ctx
//     cancellation is honoured DURING backoff sleeps.
//
// op (used by Observer hooks and log lines) is the path argument.
func (c *Client) DoSignedGET(ctx context.Context, path string, query url.Values, opts ...DoSignedOption) (*RawResponse, error) {
	if c == nil {
		return nil, errors.New("atomefin: DoSignedGET called on nil *Client")
	}
	var cfg doSignedConfig
	for _, o := range opts {
		if o != nil {
			o.applyDoSigned(&cfg)
		}
	}
	if path == "" || path[0] != '/' {
		return nil, &ValidationError{Field: "path", Message: "path must be absolute (start with '/')"}
	}

	// v0.5.1 (R13a + R13b): the spec semantic is asymmetric for
	// multi-value queries. Wire keeps every value (no data loss);
	// the signing canonical observes only the first value per
	// key (matches the upstream gateway's verification). For
	// single-value inputs both helpers produce byte-identical
	// output, so wire == canonical (R13a) is preserved
	// automatically — most calls hit this path.
	canonical := []byte(sign.CanonicalQueryFirstValue(query))
	wireQuery := sign.EncodeWireQueryRFC3986(query)
	urlStr := c.baseURL + path
	return c.signAndDispatch(ctx, path, urlStr, canonical, cfg, func(reqCtx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, err
		}
		// Wire CAN diverge from canonical when multi-value is
		// supplied (R13b). Use EncodeWireQueryRFC3986's output
		// (NOT url.Values.Encode() — that uses '+' for space and
		// would break single-value R13a byte-equality with
		// CanonicalQueryFirstValue's RFC-3986 output).
		req.URL.RawQuery = wireQuery
		return req, nil
	})
}

// DoEncryptedSigned signs and sends a POST whose body is hybrid-
// encrypted per the apaylater partner protocol (Q31–Q34).
//
// Pipeline (mirrors `~/Downloads/main.go` line 28-30):
//
//  1. Build the encrypt envelope: a fresh AES-256 key encrypts
//     `plainBody` with AES-ECB-PKCS5; the key is RSA-PKCS#1 v1.5
//     wrapped against the configured Atome encrypt public key.
//  2. Inject the resulting `Encrypt: symmetricKey=<...>` header
//     via `WithRequestHeader` (works because `Encrypt` is NOT in
//     the reserved-header allowlist).
//  3. Sign the ENCRYPTED body bytes (NOT the plaintext) — the
//     spec's signing canonical for these endpoints is the
//     base64 ciphertext.
//  4. Dispatch via the shared `signAndDispatch` retry loop —
//     identical pipeline to DoSigned. Per-retry the SDK sends the
//     same encrypted body + same signature (idempotency keyed on
//     the partner-supplied `requestId` inside the plaintext).
//
// The fresh AES key is generated ONCE per call, before signing —
// it is re-used across retries. This matches the partner
// reference implementation and the spec's idempotency model.
//
// Errors:
//   - WithEncryptAtomePublicCertPEM not configured →
//     *ValidationError, no network.
//   - method != POST → *ValidationError.
//   - encrypt.Marshal failure (rand.Read, RSA wrap) → *TransportError-class
//     error returned verbatim.
//   - 4xx/5xx (after retries) → *APIError; transport/signing failures
//     follow the DoSigned conventions.
//
// Today's only callers are credit.SubmitInformation and
// credit.SubmitApplication; see `docs/internal/V0.3_DESIGN.md` §3.
func (c *Client) DoEncryptedSigned(ctx context.Context, method, path string, plainBody []byte, opts ...DoSignedOption) (*RawResponse, error) {
	if c == nil {
		return nil, errors.New("atomefin: DoEncryptedSigned called on nil *Client")
	}
	if c.encryptAtomePub == nil {
		return nil, &ValidationError{
			Field:   "encryptAtomePublicCert",
			Message: "WithEncryptAtomePublicCertPEM is required for endpoints with the Encrypt: header (today /credit-information and /credit-application)",
		}
	}
	if method != http.MethodPost {
		return nil, &ValidationError{Field: "method", Message: "DoEncryptedSigned is POST-only"}
	}
	if path == "" || path[0] != '/' {
		return nil, &ValidationError{Field: "path", Message: "path must be absolute (start with '/')"}
	}

	header, bodyB64, err := encrypt.Marshal(plainBody, c.encryptAtomePub)
	if err != nil {
		return nil, &TransportError{Op: "encrypt", URL: path, Err: fmt.Errorf("hybrid-encrypt request body: %w", err)}
	}

	// Inject the Encrypt header. WithRequestHeader is permissive on
	// non-reserved names; Encrypt is intentionally NOT in
	// isReservedHeader so this works without a special-case.
	encryptOpts := append([]DoSignedOption{}, opts...)
	encryptOpts = append(encryptOpts, WithRequestHeader(encrypt.EncryptHeaderName, header))

	var cfg doSignedConfig
	for _, o := range encryptOpts {
		if o != nil {
			o.applyDoSigned(&cfg)
		}
	}

	urlStr := c.baseURL + path
	canonical := []byte(bodyB64)
	return c.signAndDispatch(ctx, path, urlStr, canonical, cfg, func(reqCtx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(reqCtx, method, urlStr, bytes.NewReader(canonical))
	})
}

// signAndDispatch is the shared sign + retry-loop pipeline used by
// DoSigned (POST/body) and DoSignedGET (GET/query). Caller supplies:
//
//   - op:        Observer / log label (typically the path)
//   - urlStr:    full URL for error annotation (no query)
//   - canonical: bytes fed to the Signer
//   - cfg:       parsed DoSignedOption config
//   - buildReq:  closure invoked once per attempt to build a fresh
//     *http.Request. The closure is responsible for setting RawQuery
//     (GET) or Body (POST). Headers are populated by the loop AFTER
//     the closure returns, so callers should not pre-fill any header
//     the SDK owns.
//
// Retries on 5xx + transport errors per c.retry. ctx cancellation is
// honoured during backoff sleeps via RetryPolicy.Sleep.
func (c *Client) signAndDispatch(
	ctx context.Context,
	op, urlStr string,
	canonical []byte,
	cfg doSignedConfig,
	buildReq func(reqCtx context.Context) (*http.Request, error),
) (*RawResponse, error) {
	sig, err := c.signer.Sign(ctx, canonical)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, &TransportError{Op: "sign", URL: urlStr, Err: err}
		}
		return nil, &SignatureError{Reason: "sign", Err: err}
	}
	authValue := c.authScheme(sig, c.signer.KeyID())

	reqCtx := ctx
	if c.timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var lastErr error
	for attempt := 1; attempt <= c.retry.MaxAttempts; attempt++ {
		c.safeObsRequest(reqCtx, op, attempt)

		req, err := buildReq(reqCtx)
		if err != nil {
			return nil, &TransportError{Op: "build", URL: urlStr, Err: err}
		}
		c.populateHeaders(req.Header, authValue)
		// Partner-supplied per-request headers are applied AFTER the
		// SDK-controlled ones, but the SDK rejects collisions on
		// reserved headers so callers cannot override Authorization /
		// Content-Type / User-Agent / Accept.
		for k, vs := range cfg.extraHeaders {
			if isReservedHeader(k) {
				continue
			}
			req.Header[http.CanonicalHeaderKey(k)] = append([]string(nil), vs...)
		}

		start := c.clock()
		resp, err := c.httpClient.Do(req)
		dur := c.clock().Sub(start)

		if err != nil {
			retryable := c.retry.RetryOnTransportError(err) && IsRetryableTransport(err)
			te := &TransportError{Op: "do", URL: urlStr, Err: err, Retry: retryable}
			c.safeObsRetry(reqCtx, op, attempt, te)
			c.logger.Warn("atomefin: request failed",
				"op", op, "attempt", attempt, "err", err, "dur", dur)
			lastErr = te
			if !retryable || attempt == c.retry.MaxAttempts {
				return nil, te
			}
			if sleepErr := c.retry.Sleep(reqCtx, attempt); sleepErr != nil {
				return nil, &TransportError{Op: "sleep", URL: urlStr, Err: sleepErr}
			}
			continue
		}

		respBody, readErr := readAndClose(resp.Body, c.maxRespBytes)
		c.safeObsResponse(reqCtx, op, resp.StatusCode, dur)
		if readErr != nil {
			te := &TransportError{Op: "read", URL: urlStr, Err: readErr, Retry: false}
			c.logger.Error("atomefin: response body read failed",
				"op", op, "attempt", attempt, "err", readErr)
			return nil, te
		}

		if c.debugBodyLog {
			c.logger.Debug("atomefin: response",
				"op", op, "attempt", attempt, "status", resp.StatusCode,
				"body_size", len(respBody))
		}

		if c.retry.RetryOnStatus(resp.StatusCode) && attempt < c.retry.MaxAttempts {
			apiErr := decodeAPIError(resp.StatusCode, op, respBody)
			c.safeObsRetry(reqCtx, op, attempt, apiErr)
			c.logger.Warn("atomefin: retrying after non-2xx",
				"op", op, "attempt", attempt, "status", resp.StatusCode, "code", string(apiErr.Code))
			lastErr = apiErr
			if sleepErr := c.retry.Sleep(reqCtx, attempt); sleepErr != nil {
				return nil, &TransportError{Op: "sleep", URL: urlStr, Err: sleepErr}
			}
			continue
		}

		if resp.StatusCode >= 400 {
			return nil, decodeAPIError(resp.StatusCode, op, respBody)
		}

		return &RawResponse{
			StatusCode: resp.StatusCode,
			Header:     cloneHeader(resp.Header),
			Body:       respBody,
		}, nil
	}

	// Defensive: loop exited without returning. Shouldn't happen because
	// MaxAttempts >= 1 and every branch returns or continues, but Go
	// can't prove that.
	if lastErr == nil {
		lastErr = &TransportError{Op: "do", URL: urlStr, Err: errors.New("retry loop exhausted with no error captured")}
	}
	return nil, lastErr
}

// isReservedHeader reports whether the partner is forbidden from
// overriding this header via WithRequestHeader. The SDK owns the
// signing-related and identity-related headers because mis-set values
// would break verification end-to-end.
func isReservedHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Authorization", "Content-Type", "User-Agent", "Accept":
		return true
	}
	return false
}

// populateHeaders writes the SDK-controlled headers onto h. Q7 RESOLVED
// (2026-05-05): partner identity is established by the dedicated API URL
// plus the RSA certificate exchange, not a header — so no
// partner-identifying header is emitted. Partner-supplied per-request
// headers still flow through cfg.extraHeaders in DoSigned (used for the
// /auth `sessionid` header today, and reserved for the timestamp/nonce
// header once Q5 closes).
func (c *Client) populateHeaders(h http.Header, authValue string) {
	h.Set("Authorization", authValue)
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "application/json")
	h.Set("User-Agent", c.userAgent)
}

// readAndClose reads up to max+1 bytes from r (so we can detect overruns)
// and always closes r. Returns the body bytes (truncated to max) and an
// error if the body exceeded max.
func readAndClose(r io.ReadCloser, max int64) ([]byte, error) {
	defer r.Close()
	if max <= 0 {
		max = 4 << 20
	}
	buf, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > max {
		return buf[:max], errors.New("response body exceeds max size; consider WithMaxResponseBytes")
	}
	return buf, nil
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		cp := make([]string, len(v))
		copy(cp, v)
		out[k] = cp
	}
	return out
}

// safeObsRequest, safeObsResponse, safeObsRetry are panic-isolating
// wrappers around the Observer hook surface. Partner observability code
// can panic for any number of reasons (bad metric label cardinality, a
// nil pointer in a tracing exporter, an OOM-bound buffer flush). Without
// these wrappers, that panic unwinds the SDK's request-handling stack
// and returns a corrupted error to the caller. With them, the panic is
// logged and the request continues unaffected — the hook lost a sample
// but the partner's `Auth()` call still gets its envelope.
//
// We use a free function pattern (not a single deferred wrapper around
// each call site) so the recovery scope is exactly one Observer call.
func (c *Client) safeObsRequest(ctx context.Context, op string, attempt int) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("atomefin: observer.OnRequest panicked",
				"op", op, "attempt", attempt, "panic", fmt.Sprintf("%v", r))
		}
	}()
	c.observer.OnRequest(ctx, op, attempt)
}

func (c *Client) safeObsResponse(ctx context.Context, op string, status int, dur time.Duration) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("atomefin: observer.OnResponse panicked",
				"op", op, "status", status, "panic", fmt.Sprintf("%v", r))
		}
	}()
	c.observer.OnResponse(ctx, op, status, dur)
}

func (c *Client) safeObsRetry(ctx context.Context, op string, attempt int, err error) {
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("atomefin: observer.OnRetry panicked",
				"op", op, "attempt", attempt, "panic", fmt.Sprintf("%v", r))
		}
	}()
	c.observer.OnRetry(ctx, op, attempt, err)
}

// JoinPath joins the base URL and a relative path, defending against a
// stray trailing slash on either side. Exposed because services need it
// when building URL strings for log lines (the actual HTTP call uses
// DoSigned which already concatenates).
func JoinPath(base, path string) string {
	base = strings.TrimRight(base, "/")
	if path == "" {
		return base
	}
	if path[0] != '/' {
		return base + "/" + path
	}
	return base + path
}
