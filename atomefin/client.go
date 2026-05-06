package atomefin

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// Client is the stateful root object partners use to talk to the atomefin
// white-label "G" API. Construct it via atomefin.New(...). It is safe for
// concurrent use by multiple goroutines: every field is read-only after
// New returns, the Signer guarantees concurrent safety per its package
// docs, and the embedded *http.Client is itself goroutine-safe.
type Client struct {
	httpClient   *http.Client
	baseURL      string
	environment  Environment
	timeout      time.Duration
	signer       sign.Signer
	verifier     sign.Verifier
	authScheme   AuthorizationScheme
	keyID        string
	partnerID    string
	merchantID   string
	retry        transport.RetryPolicy
	logger       transport.Logger
	observer     transport.Observer
	clock        func() time.Time
	requestIDGen func() string
	userAgent    string
	maxRespBytes int64
	debugBodyLog bool
}

// New constructs a Client.
//
// Required options (failing to pass any one returns an error before any
// network is touched):
//   - WithSigner OR WithPrivateKeyPEM — the partner's RSA private key.
//   - WithEnvironment OR WithBaseURL — where to send traffic.
//
// Everything else has a sensible default. See option-level docs in
// options.go for each WithX function.
//
// Note: partner identity is established by the dedicated API URL +
// RSA certificate exchange — no partner / merchant header is emitted
// on the wire (Q7 RESOLVED, 2026-05-05). WithPartnerID and
// WithMerchantID stay supported as log-enrichment hooks only.
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for i, opt := range opts {
		if opt == nil {
			return nil, &ValidationError{Field: "options", Message: optionIndexLabel(i) + " is nil"}
		}
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}

	if err := finalizeConfig(cfg); err != nil {
		return nil, err
	}

	c := &Client{
		httpClient:   cfg.httpClient,
		baseURL:      cfg.baseURL,
		environment:  cfg.environment,
		timeout:      cfg.timeout,
		signer:       cfg.signer,
		verifier:     cfg.verifier,
		authScheme:   cfg.authScheme,
		keyID:        cfg.keyID,
		partnerID:    cfg.partnerID,
		merchantID:   cfg.merchantID,
		retry:        cfg.retry,
		logger:       cfg.logger,
		observer:     cfg.observer,
		clock:        cfg.clock,
		requestIDGen: cfg.requestIDGen,
		userAgent:    cfg.userAgent,
		maxRespBytes: cfg.maxRespBytes,
		debugBodyLog: cfg.debugBodyLog,
	}
	return c, nil
}

// finalizeConfig applies cross-option validation and computes derived
// fields after every Option has been applied.
func finalizeConfig(cfg *config) error {
	if cfg.signer == nil {
		return errors.New("atomefin: New: a Signer is required (use WithSigner or WithPrivateKeyPEM)")
	}
	// PartnerID is no longer required — Q7 RESOLVED. Partner identity
	// is the dedicated API URL + cert exchange, not a header.
	if cfg.baseURL == "" {
		u, err := BaseURL(cfg.environment)
		if err != nil {
			return err
		}
		cfg.baseURL = u
	}
	if cfg.userAgent == "" {
		cfg.userAgent = transport.BuildUserAgent(SDKVersion, "")
	} else {
		// User passed a suffix (e.g. "merchant-foo/1.2") — wrap it in the
		// canonical product/version assembly so logs always include both.
		cfg.userAgent = transport.BuildUserAgent(SDKVersion, cfg.userAgent)
	}
	if cfg.retry.MaxAttempts == 0 {
		// Defensive: should be set by defaultConfig.
		cfg.retry = transport.DefaultRetryPolicy()
	}
	if err := cfg.retry.Validate(); err != nil {
		return err
	}
	return nil
}

// optionIndexLabel produces a human-readable index ("option 0", "option 1")
// without dragging fmt into a tiny helper.
func optionIndexLabel(i int) string {
	switch i {
	case 0:
		return "option 0"
	case 1:
		return "option 1"
	}
	return "option N"
}

// --- Public accessors used by sub-services (payment, callback) ---

// BaseURL returns the resolved base URL for this Client. Useful for log
// lines and for the payment service when building URLs.
func (c *Client) BaseURL() string { return c.baseURL }

// Environment returns the configured environment, or "" if WithBaseURL
// was used directly without WithEnvironment.
func (c *Client) Environment() Environment { return c.environment }

// PartnerID returns the configured partner identifier.
func (c *Client) PartnerID() string { return c.partnerID }

// MerchantID returns the configured merchant identifier (may be empty).
func (c *Client) MerchantID() string { return c.merchantID }

// Logger returns the configured Logger so sub-services can log alongside
// the umbrella package without re-wiring their own.
func (c *Client) Logger() transport.Logger { return c.logger }

// Observer returns the configured Observer.
func (c *Client) Observer() transport.Observer { return c.observer }

// Verifier returns the Atome-side Verifier configured via
// WithAtomePublicKey / WithAtomePublicCertPEM, or nil if none was set.
// Used by the callback package (T4) to verify inbound signatures.
func (c *Client) Verifier() sign.Verifier { return c.verifier }

// NewRequestID returns a fresh requestId via the configured generator.
// Sub-services should call this once per outbound call and reuse the
// result across retries (DESIGN.md §1.4).
func (c *Client) NewRequestID() string { return c.requestIDGen() }

// Now returns the current time according to the configured clock. Tests
// that pass WithClock can drive the clock; production callers get
// time.Now via the default.
func (c *Client) Now() time.Time { return c.clock() }

// HTTPClient returns the underlying *http.Client. Exposed for advanced
// use cases (custom transport instrumentation in tests). Sub-services
// should prefer DoSigned over reaching directly to the http client.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// Close releases resources held by the Client.
//
// Today this is a no-op stub and always returns nil. T2 ships it as a
// stub so partners can adopt the idiomatic `defer client.Close()` pattern
// now without an API churn when v0.2 introduces background goroutines
// (rate limiter, signer hot-reload, certificate watcher, etc.).
//
// Semantics planned for v0.2:
//   - Multiple Close() calls are safe (idempotent).
//   - DoSigned after Close() returns ErrClosed (not yet defined).
//
// Until then, partners that follow the `defer Close()` discipline
// observe identical behaviour to today's no-op New/use lifecycle.
func (c *Client) Close() error { return nil }

// Compile-time guard: ensure Client satisfies the minimal contract the
// sub-services in payment/ and callback/ will need (this also documents
// the exported API).
var _ interface {
	DoSigned(ctx context.Context, method, path string, body []byte, opts ...DoSignedOption) (*RawResponse, error)
	DoSignedGET(ctx context.Context, path string, query url.Values, opts ...DoSignedOption) (*RawResponse, error)
	NewRequestID() string
} = (*Client)(nil)
