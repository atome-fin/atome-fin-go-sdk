package atomefin

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// AuthorizationScheme renders a signature into the value placed in the
// Authorization header. The SDK's default is SchemeRawBase64 — emit the
// signature verbatim (DESIGN.md §5, team-lead 2026-05-05 confirmation
// pending the partner's own confirmation under DESIGN.md §13/Q2).
//
// keyID is whatever Signer.KeyID() returned at Client construction time;
// it may be empty. Schemes that do not surface a keyId should ignore it.
//
// The function must produce an RFC 7230 field-value (no CR/LF, no leading/
// trailing whitespace).
type AuthorizationScheme func(signature, keyID string) string

// SchemeRawBase64 emits the signature verbatim. This is the default and
// matches what the spec says for partners that have not been told to use
// a scheme prefix.
func SchemeRawBase64(signature, _ string) string { return signature }

// SchemeAtomeKeyed renders the structured form some Atome partners use:
//
//	Algorithm=RSA2,KeyVersion=<keyID>,Sign=<signature>
//
// Provided as a one-line override target for WithAuthorizationScheme.
// Wire-format authorization stays open until DESIGN.md §13/Q2 is closed —
// keep both paths until the partner confirms.
func SchemeAtomeKeyed(signature, keyID string) string {
	if keyID == "" {
		return "Algorithm=RSA2,Sign=" + signature
	}
	return "Algorithm=RSA2,KeyVersion=" + keyID + ",Sign=" + signature
}

// Option configures a Client at construction time. Functional options
// pattern (DESIGN.md §3): every option returns an error so configuration
// failures surface from atomefin.New rather than at first request.
type Option func(*config) error

// config is the internal staging struct used by New() to assemble a
// Client. It is not exposed; partners compose state via the Option
// constructors below.
type config struct {
	httpClient   *http.Client
	baseURL      string
	environment  Environment
	timeout      time.Duration
	logger       transport.Logger
	observer     transport.Observer
	signer       sign.Signer
	verifier     sign.Verifier // for atome-side signature checks (callbacks, optional sync)
	keyID        string
	authScheme   AuthorizationScheme
	partnerID    string
	merchantID   string
	retry        transport.RetryPolicy
	userAgent    string
	clock        func() time.Time
	requestIDGen func() string
	maxRespBytes int64
	debugBodyLog bool

	// v0.3 hybrid-encryption keypair. Both nilable; required only by
	// /credit-information and /credit-application.
	encryptAtomePub *rsa.PublicKey
	encryptPriv     *rsa.PrivateKey
}

// defaultConfig produces the initial config used by New. Each Option may
// override fields; required fields validated post-options.
func defaultConfig() *config {
	return &config{
		httpClient:   newDefaultHTTPClient(),
		environment:  EnvProd,
		timeout:      30 * time.Second,
		logger:       transport.NopLogger{},
		observer:     transport.NopObserver{},
		authScheme:   SchemeRawBase64,
		retry:        transport.DefaultRetryPolicy(),
		clock:        time.Now,
		requestIDGen: DefaultRequestID,
		// Symmetric to the callback BodyLimit (DESIGN.md §8). A 1 MiB cap
		// is well above any realistic auth/capture envelope and bounds OOM
		// surface area for hostile or buggy server responses.
		maxRespBytes: 1 << 20,
	}
}

// newDefaultHTTPClient builds the SDK's default *http.Client with a CLONED
// transport — never share http.DefaultTransport directly, since fields the
// SDK might tune (MaxIdleConnsPerHost, IdleConnTimeout, TLSHandshakeTimeout)
// would otherwise mutate the singleton used by every other HTTP user in
// the partner's process. See architect's batteries review §5.
func newDefaultHTTPClient() *http.Client {
	var t *http.Transport
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		t = base.Clone()
	} else {
		t = &http.Transport{}
	}
	t.MaxIdleConnsPerHost = 32
	t.IdleConnTimeout = 90 * time.Second
	t.TLSHandshakeTimeout = 10 * time.Second
	return &http.Client{Transport: t, Timeout: 30 * time.Second}
}

// --- HTTP / transport options ---

// WithHTTPClient overrides the SDK's HTTP client. The default is a clone of
// http.DefaultClient with a 30s timeout; partners that need custom TLS
// config, proxies, or a transport with connection pooling tuned for their
// volume should pass their own.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) error {
		if h == nil {
			return errors.New("atomefin: WithHTTPClient: nil *http.Client")
		}
		c.httpClient = h
		return nil
	}
}

// WithBaseURL pins an explicit base URL, overriding whatever
// WithEnvironment selected. Useful when the partner has a confirmed
// gateway URL different from the spec placeholder, or for routing
// requests through an in-network mock during tests.
//
// Validation: scheme must be http or https, host must be non-empty, the
// trailing slash is stripped to keep `baseURL + path` joins idempotent.
func WithBaseURL(u string) Option {
	return func(c *config) error {
		u = strings.TrimSpace(u)
		if u == "" {
			return errors.New("atomefin: WithBaseURL: empty URL")
		}
		parsed, err := url.Parse(u)
		if err != nil {
			return fmt.Errorf("atomefin: WithBaseURL: parse %q: %w", u, err)
		}
		switch parsed.Scheme {
		case "http", "https":
		default:
			return fmt.Errorf("atomefin: WithBaseURL: scheme must be http or https, got %q", parsed.Scheme)
		}
		if parsed.Host == "" {
			return fmt.Errorf("atomefin: WithBaseURL: missing host in %q", u)
		}
		c.baseURL = strings.TrimRight(u, "/")
		return nil
	}
}

// WithEnvironment selects one of the spec-defined placeholder URLs
// (EnvPre / EnvProd). If WithBaseURL is also passed, the
// explicit base URL wins. Default is EnvProd.
func WithEnvironment(env Environment) Option {
	return func(c *config) error {
		if _, err := BaseURL(env); err != nil {
			return err
		}
		c.environment = env
		return nil
	}
}

// WithTimeout caps how long a single HTTP request (including retries' own
// per-attempt deadline) is allowed to take. Composes with the parent
// context: whichever expires first wins.
func WithTimeout(d time.Duration) Option {
	return func(c *config) error {
		if d <= 0 {
			return errors.New("atomefin: WithTimeout: duration must be > 0")
		}
		c.timeout = d
		return nil
	}
}

// WithRetry replaces the default RetryPolicy. The policy is validated; an
// invalid one (negative MaxAttempts, etc.) fails Client construction.
func WithRetry(p transport.RetryPolicy) Option {
	return func(c *config) error {
		if err := p.Validate(); err != nil {
			return err
		}
		c.retry = p
		return nil
	}
}

// WithUserAgent appends a partner-supplied product token to the default
// "atome-fin-go-sdk/<version> (gox.y; goos/goarch)" prefix.
func WithUserAgent(ua string) Option {
	return func(c *config) error {
		c.userAgent = ua
		return nil
	}
}

// --- Identity / signing options ---

// WithSigner sets an explicit Signer. Required (or supply via
// WithPrivateKeyPEM). Mutually exclusive with WithPrivateKeyPEM — passing
// both is an error.
func WithSigner(s sign.Signer) Option {
	return func(c *config) error {
		if s == nil {
			return errors.New("atomefin: WithSigner: nil Signer")
		}
		if c.signer != nil {
			return errors.New("atomefin: WithSigner: signer already configured (use WithSigner OR WithPrivateKeyPEM)")
		}
		c.signer = s
		return nil
	}
}

// WithPrivateKeyPEM is a shorthand that wraps the provided PEM-encoded
// RSA private key in the default RSA-PKCS1-v1.5 / SHA-256 Signer. Pass
// password (a single argument) only when the PEM is encrypted — encrypted
// PEM support is gated on DESIGN.md §13/Q3 and currently returns an error.
func WithPrivateKeyPEM(pem []byte, password ...[]byte) Option {
	return func(c *config) error {
		if c.signer != nil {
			return errors.New("atomefin: WithPrivateKeyPEM: signer already configured")
		}
		key, err := sign.LoadPrivateKeyPEM(pem, password...)
		if err != nil {
			return fmt.Errorf("atomefin: WithPrivateKeyPEM: %w", err)
		}
		var opts []sign.SignerOption
		if c.keyID != "" {
			opts = append(opts, sign.WithKeyID(c.keyID))
		}
		s, err := sign.NewRSA2Signer(key, opts...)
		if err != nil {
			return fmt.Errorf("atomefin: WithPrivateKeyPEM: %w", err)
		}
		c.signer = s
		return nil
	}
}

// WithAtomePublicKey sets the verifier used to sanity-check signatures on
// inbound traffic the Client itself receives (callback handlers and
// optional verification of sync error envelopes). Stored on the Client so
// the callback package can pull it during T4. Pass an *rsa.PublicKey
// directly when you already have one parsed; use WithAtomePublicCertPEM
// for raw PEM.
func WithAtomePublicKey(pub *rsa.PublicKey) Option {
	return func(c *config) error {
		if pub == nil {
			return errors.New("atomefin: WithAtomePublicKey: nil *rsa.PublicKey")
		}
		v, err := sign.NewRSA2Verifier(pub)
		if err != nil {
			return fmt.Errorf("atomefin: WithAtomePublicKey: %w", err)
		}
		c.verifier = v
		return nil
	}
}

// WithAtomePublicCertPEM is the PEM-bytes counterpart of
// WithAtomePublicKey. Accepts CERTIFICATE / PUBLIC KEY / RSA PUBLIC KEY
// blocks (see sign.LoadPublicCertPEM).
func WithAtomePublicCertPEM(pem []byte) Option {
	return func(c *config) error {
		key, err := sign.LoadPublicCertPEM(pem)
		if err != nil {
			return fmt.Errorf("atomefin: WithAtomePublicCertPEM: %w", err)
		}
		v, err := sign.NewRSA2Verifier(key)
		if err != nil {
			return fmt.Errorf("atomefin: WithAtomePublicCertPEM: %w", err)
		}
		c.verifier = v
		return nil
	}
}

// WithEncryptAtomePublicCertPEM sets Atome's encrypt public key (the
// key used to wrap per-request AES keys for endpoints with the
// `Encrypt:` header). Required when calling /credit-information or
// /credit-application; optional otherwise.
//
// Distinct from WithAtomePublicCertPEM (which sets the verifier
// public key for signature checks). Q34 RESOLVED 2026-05-06: the
// partner protocol mandates a SEPARATE certificate pair for
// hybrid encryption versus signing — different keypairs, different
// rotation cadence.
//
// Accepts CERTIFICATE / PUBLIC KEY / RSA PUBLIC KEY blocks, same as
// WithAtomePublicCertPEM. Rejects keys < 2048 bits.
func WithEncryptAtomePublicCertPEM(pem []byte) Option {
	return func(c *config) error {
		if c.encryptAtomePub != nil {
			return errors.New("atomefin: WithEncryptAtomePublicCertPEM: encrypt public key already configured")
		}
		key, err := sign.LoadPublicCertPEM(pem)
		if err != nil {
			return fmt.Errorf("atomefin: WithEncryptAtomePublicCertPEM: %w", err)
		}
		if key.N.BitLen() < 2048 {
			return fmt.Errorf("atomefin: WithEncryptAtomePublicCertPEM: RSA modulus %d < min 2048 bits", key.N.BitLen())
		}
		c.encryptAtomePub = key
		return nil
	}
}

// WithEncryptPrivateKeyPEM sets the partner's encrypt PRIVATE key
// (used to unwrap inbound encrypted bodies). Q31 RESOLVED 2026-05-06:
// credit callbacks are plaintext, so v0.3 has no inbound caller for
// this. Shipped for symmetry + forward-compat — partners with custom
// callback decryption tooling can call encrypt.Unmarshal directly
// using EncryptPrivateKey().
//
// Distinct from WithPrivateKeyPEM (which sets the signing private
// key). Accepts PKCS#1 / PKCS#8 PEM blocks. Rejects keys < 2048 bits.
func WithEncryptPrivateKeyPEM(pem []byte, password ...[]byte) Option {
	return func(c *config) error {
		if c.encryptPriv != nil {
			return errors.New("atomefin: WithEncryptPrivateKeyPEM: encrypt private key already configured")
		}
		key, err := sign.LoadPrivateKeyPEM(pem, password...)
		if err != nil {
			return fmt.Errorf("atomefin: WithEncryptPrivateKeyPEM: %w", err)
		}
		if key.N.BitLen() < 2048 {
			return fmt.Errorf("atomefin: WithEncryptPrivateKeyPEM: RSA modulus %d < min 2048 bits", key.N.BitLen())
		}
		c.encryptPriv = key
		return nil
	}
}

// AtomeCertSource bundles a (partner-private, atome-public)
// keypair for one of the two cert roles the SDK uses (signing
// or encryption). PartnerPriv is the partner's PEM-encoded RSA
// private key for that role; AtomePub is Atome's PEM-encoded
// public key. Either field may be nil — the matching cert
// option is then skipped (useful for partial setups, e.g. a
// signing-only Client that doesn't touch the credit POSTs).
//
// The four atomefin.WithX cert options stay supported.
// AtomeCertSource is a convenience handle for partners who keep
// the four PEM blobs together in their config — the
// (signing, encrypting) `WithAtomeCerts` form maps cleanly to
// most key-rotation tooling.
type AtomeCertSource struct {
	// PartnerPriv is the partner's RSA private key (PEM bytes).
	// Required when the role is exercised; optional otherwise.
	PartnerPriv []byte

	// AtomePub is Atome's matching public cert (PEM bytes).
	// Required when the role is exercised; optional otherwise.
	AtomePub []byte
}

// WithAtomeCerts is the v0.6 convenience option that wraps the
// four individual cert options
// (WithPrivateKeyPEM / WithAtomePublicCertPEM /
// WithEncryptPrivateKeyPEM / WithEncryptAtomePublicCertPEM)
// behind a single (signing, encrypting) pair.
//
// Equivalent to:
//
//	atomefin.New(
//	    WithPrivateKeyPEM(signing.PartnerPriv),
//	    WithAtomePublicCertPEM(signing.AtomePub),
//	    WithEncryptPrivateKeyPEM(encrypting.PartnerPriv),
//	    WithEncryptAtomePublicCertPEM(encrypting.AtomePub),
//	)
//
// Empty fields skip the matching individual option, so partners
// that only need signing (no credit POSTs) can pass an empty
// `encrypting` AtomeCertSource. Each cert option's error
// surface (already-configured, key too short, malformed PEM) is
// preserved verbatim — wrapping is purely a call-site
// convenience, not a relaxation of validation.
//
// The four individual options stay supported and recommended
// for partners who load the cert blobs from independent sources
// (e.g., signing key from KMS, encrypt cert from disk).
func WithAtomeCerts(signing, encrypting AtomeCertSource) Option {
	return func(c *config) error {
		if signing.PartnerPriv != nil {
			if err := WithPrivateKeyPEM(signing.PartnerPriv)(c); err != nil {
				return err
			}
		}
		if signing.AtomePub != nil {
			if err := WithAtomePublicCertPEM(signing.AtomePub)(c); err != nil {
				return err
			}
		}
		if encrypting.PartnerPriv != nil {
			if err := WithEncryptPrivateKeyPEM(encrypting.PartnerPriv)(c); err != nil {
				return err
			}
		}
		if encrypting.AtomePub != nil {
			if err := WithEncryptAtomePublicCertPEM(encrypting.AtomePub)(c); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithKeyID sets the keyId returned by Signer.KeyID(). Sent through the
// AuthorizationScheme function so partners can swap the wire format
// without recompiling. May be set before or after WithSigner: setting it
// after a signer is configured rebuilds the signer with the keyID applied
// where supported. For the default RSA2 signer constructed via
// WithPrivateKeyPEM the keyID is wired during PEM loading.
func WithKeyID(id string) Option {
	return func(c *config) error {
		c.keyID = id
		return nil
	}
}

// WithAuthorizationScheme overrides how the signature is rendered into the
// Authorization header value. Default is SchemeRawBase64; SchemeAtomeKeyed
// is provided for one-line switching.
func WithAuthorizationScheme(scheme AuthorizationScheme) Option {
	return func(c *config) error {
		if scheme == nil {
			return errors.New("atomefin: WithAuthorizationScheme: nil scheme")
		}
		c.authScheme = scheme
		return nil
	}
}

// WithPartnerID sets the partner identifier used in log fields and
// observability hooks.
//
// Optional. Q7 RESOLVED (2026-05-05): NOT transmitted on the wire — the
// partner is identified by the dedicated API URL plus the RSA cert
// exchange, not by a header. Use this option when you want the
// PartnerID() accessor populated for log enrichment; the SDK does not
// otherwise read the value.
func WithPartnerID(id string) Option {
	return func(c *config) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("atomefin: WithPartnerID: empty id")
		}
		c.partnerID = id
		return nil
	}
}

// WithMerchantID sets the merchant identifier.
//
// Optional. Q7 RESOLVED (2026-05-05): NOT transmitted on the wire —
// log enrichment only.
func WithMerchantID(id string) Option {
	return func(c *config) error {
		c.merchantID = strings.TrimSpace(id)
		return nil
	}
}

// --- Observability options ---

// WithLogger replaces the default no-op Logger.
func WithLogger(l transport.Logger) Option {
	return func(c *config) error {
		if l == nil {
			return errors.New("atomefin: WithLogger: nil Logger")
		}
		c.logger = l
		return nil
	}
}

// WithObserver replaces the default no-op Observer (metrics / tracing).
func WithObserver(o transport.Observer) Option {
	return func(c *config) error {
		if o == nil {
			return errors.New("atomefin: WithObserver: nil Observer")
		}
		c.observer = o
		return nil
	}
}

// WithDebugBodyLogging toggles whether the SDK logs raw request/response
// bodies at Debug level. Off by default (PII safety per DESIGN.md §10).
// When enabled, a future PII-redaction pass will scrub known sensitive
// fields before bodies hit the logger; until that lands, partners that
// flip this on accept that requests/responses may contain PII.
func WithDebugBodyLogging(enable bool) Option {
	return func(c *config) error {
		c.debugBodyLog = enable
		return nil
	}
}

// --- Testability options ---

// WithClock overrides time.Now. Used by tests; production callers should
// not need it.
func WithClock(now func() time.Time) Option {
	return func(c *config) error {
		if now == nil {
			return errors.New("atomefin: WithClock: nil clock")
		}
		c.clock = now
		return nil
	}
}

// WithRequestIDGenerator overrides the default DefaultRequestID generator.
// Useful for partners that want to embed their order ID prefix or for
// tests that need deterministic IDs.
func WithRequestIDGenerator(fn func() string) Option {
	return func(c *config) error {
		if fn == nil {
			return errors.New("atomefin: WithRequestIDGenerator: nil generator")
		}
		c.requestIDGen = fn
		return nil
	}
}

// WithMaxResponseBytes caps the size of a response body the SDK will
// buffer in memory. Default 1 MiB — symmetric to the inbound callback
// BodyLimit (DESIGN.md §8). Bodies larger than the cap are
// truncated and the SDK returns a TransportError so the caller can decide
// whether to retry against a different deployment.
func WithMaxResponseBytes(n int64) Option {
	return func(c *config) error {
		if n <= 0 {
			return errors.New("atomefin: WithMaxResponseBytes: must be > 0")
		}
		c.maxRespBytes = n
		return nil
	}
}
