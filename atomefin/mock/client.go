package mock

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// Option configures the mock Client / Server. All Options are
// optional; sane defaults below.
type Option func(*config) error

type config struct {
	scenario       Scenario
	environment    atomefin.Environment
	baseURL        string
	signingPriv    []byte // PEM bytes for the signing private key
	encryptPub     []byte // PEM bytes for the encrypt public key (Atome side)
	encryptPriv    []byte // PEM bytes for the encrypt private key (partner side)
	allowMockKeys  bool
	verifierPubPEM []byte // optional: configure WithAtomePublicCertPEM
}

// WithScenario swaps the default scenario. Default: AlwaysSuccess.
func WithScenario(s Scenario) Option {
	return func(c *config) error {
		if s == nil {
			return errors.New("mock: WithScenario: nil Scenario")
		}
		c.scenario = s
		return nil
	}
}

// WithEnvironment sets the atomefin.Environment of the constructed
// Client. EnvProd is REFUSED — `mock.NewClient` calls `t.Fatalf`
// rather than return a Client when this is EnvProd. Default:
// EnvPre.
//
// **Side effect:** passing this option clears the package's
// default mock base URL (`https://mock.atome-fin.test`) so the
// resolved baseURL is the upstream placeholder
// (`https://id-api-pre.apaylater.net/grabpaylater` for EnvPre, etc.).
// That means `PerEndpoint` keys must include the `/grabpaylater`
// path prefix — pass `WithBaseURL("https://mock.atome-fin.test")`
// alongside if you prefer the clean default.
//
// EnvProd refusal is the package's #1 risk-class guardrail:
// nothing in atomefin/mock should ever co-exist with a production
// configuration accidentally, and the simplest way to enforce
// that is to fail loud at test setup.
func WithEnvironment(env atomefin.Environment) Option {
	return func(c *config) error {
		c.environment = env
		c.baseURL = "" // let upstream placeholders apply
		return nil
	}
}

// WithBaseURL sets the explicit base URL on the Client. Useful
// when the test substitutes an httptest.NewServer URL that is
// not in the EnvX placeholders. Mutually exclusive with
// WithEnvironment in practice (WithBaseURL takes precedence
// inside `atomefin.New`).
func WithBaseURL(url string) Option {
	return func(c *config) error {
		c.baseURL = url
		return nil
	}
}

// WithSigningKeyPEM brings your own RSA-2048 signing private key
// (PEM bytes). Mutually exclusive with WithMockKeysAllowed: if
// both are passed, the explicit key wins.
func WithSigningKeyPEM(privPEM []byte) Option {
	return func(c *config) error {
		if len(privPEM) == 0 {
			return errors.New("mock: WithSigningKeyPEM: empty PEM")
		}
		c.signingPriv = privPEM
		return nil
	}
}

// WithEncryptKeyPair brings your own RSA-2048 encrypt keys (PEM
// bytes). Pass either side; pass both if you need round-trip
// (e.g. to call encrypt.Marshal in test setup AND have the SDK
// dispatch through DoEncryptedSigned). Mutually exclusive with
// WithMockKeysAllowed.
func WithEncryptKeyPair(atomePubPEM, partnerPrivPEM []byte) Option {
	return func(c *config) error {
		c.encryptPub = atomePubPEM
		c.encryptPriv = partnerPrivPEM
		return nil
	}
}

// WithVerifierPubCertPEM sets the Atome-side signing PUBLIC key
// for callback signature verification (mirrors
// atomefin.WithAtomePublicCertPEM). Default: empty (no verifier).
func WithVerifierPubCertPEM(pem []byte) Option {
	return func(c *config) error {
		c.verifierPubPEM = pem
		return nil
	}
}

// WithMockKeysAllowed opts the Client into the bundled mock
// keypairs (signing + encrypt). OFF by default — a partner who
// forgets to pass this option AND forgets to pass an explicit
// WithSigningKeyPEM will see a clear error from `mock.NewClient`
// rather than silently picking up the bundled keys.
//
// Rationale: bundled-and-default would mean every test in the
// monorepo silently shares one keypair; tests that need to
// verify signatures across multiple "partners" would need to
// explicitly fork. Opt-in is partner-friendlier.
func WithMockKeysAllowed() Option {
	return func(c *config) error {
		c.allowMockKeys = true
		return nil
	}
}

// NewClient returns an *atomefin.Client wired to a mock RoundTripper.
//
// EnvProd is hard-blocked: if `WithEnvironment(EnvProd)` is
// supplied, `t.Fatalf` is called and the test fails immediately.
// Default environment is EnvPre.
//
// Default scenario is AlwaysSuccess. Default signing key is
// **none** — partners must pass `WithSigningKeyPEM` or
// `WithMockKeysAllowed` (or the construction returns an error).
//
// The returned Client carries a non-nil verifier ONLY if
// WithVerifierPubCertPEM (or WithMockKeysAllowed) supplied one.
// `Client.EncryptAtomePublicKey()` is set ONLY if the encrypt
// keys were configured (similar opt-in).
//
// Cleanup of the underlying httptest plumbing (when applicable)
// is registered via t.Cleanup.
func NewClient(tb testing.TB, opts ...Option) *atomefin.Client {
	tb.Helper()
	cfg := &config{
		scenario:    AlwaysSuccess(),
		environment: atomefin.EnvPre,
		// Default to a base URL with NO path prefix so the
		// transport sees clean "/auth", "/capture", etc. paths.
		// Partners using PerEndpoint should expect "POST /auth"
		// keys, not "POST /grabpaylater/auth". WithBaseURL
		// overrides; WithEnvironment keeps the placeholder
		// behaviour by NULL-ing this default.
		baseURL: "https://mock.atome-fin.test",
	}
	for _, o := range opts {
		if o == nil {
			continue
		}
		if err := o(cfg); err != nil {
			tb.Fatalf("mock.NewClient: %v", err)
		}
	}
	// EnvProd refusal is the partner-protective #1 guard.
	if cfg.environment == atomefin.EnvProd {
		tb.Fatalf("mock.NewClient: EnvProd is REFUSED — mock clients must not co-exist with a production environment configuration. Pass WithEnvironment(atomefin.EnvPre) instead.")
		return nil // unreachable but keeps the type checker happy when tb is faked
	}

	// Resolve signing key — explicit overrides bundled.
	if cfg.signingPriv == nil {
		if cfg.allowMockKeys {
			cfg.signingPriv = MockSigningPrivKeyPEM()
		} else {
			cfg.signingPriv = mustGenerateRSAPrivPEM(tb)
		}
	}

	// Resolve verifier — explicit overrides bundled.
	if cfg.verifierPubPEM == nil && cfg.allowMockKeys {
		cfg.verifierPubPEM = MockSigningPubCertPEM()
	}

	// Resolve encrypt keys — explicit overrides bundled. Only
	// applied if at least one half is supplied (otherwise the
	// Client constructs without encrypt support, matching v0.2's
	// behaviour for non-credit endpoints).
	if cfg.encryptPub == nil && cfg.allowMockKeys {
		cfg.encryptPub = MockEncryptPubCertPEM()
	}
	if cfg.encryptPriv == nil && cfg.allowMockKeys {
		cfg.encryptPriv = MockEncryptPrivKeyPEM()
	}

	transport := NewTransport(tb, cfg.scenario)

	atomefinOpts := []atomefin.Option{
		atomefin.WithEnvironment(cfg.environment),
		atomefin.WithPrivateKeyPEM(cfg.signingPriv),
		atomefin.WithHTTPClient(&http.Client{Transport: transport}),
	}
	if cfg.baseURL != "" {
		atomefinOpts = append(atomefinOpts, atomefin.WithBaseURL(cfg.baseURL))
	}
	if cfg.verifierPubPEM != nil {
		atomefinOpts = append(atomefinOpts, atomefin.WithAtomePublicCertPEM(cfg.verifierPubPEM))
	}
	if cfg.encryptPub != nil {
		atomefinOpts = append(atomefinOpts, atomefin.WithEncryptAtomePublicCertPEM(cfg.encryptPub))
	}
	if cfg.encryptPriv != nil {
		atomefinOpts = append(atomefinOpts, atomefin.WithEncryptPrivateKeyPEM(cfg.encryptPriv))
	}

	c, err := atomefin.New(atomefinOpts...)
	if err != nil {
		tb.Fatalf("mock.NewClient: atomefin.New: %v", err)
	}
	return c
}

// mustGenerateRSAPrivPEM generates a fresh RSA-2048 private key
// PEM. Used when neither WithSigningKeyPEM nor WithMockKeysAllowed
// is supplied — the SDK still needs SOME signing key to construct,
// so we mint a one-shot.
//
// ~50ms of work; partners running thousands of tests should pass
// WithMockKeysAllowed to amortize, or pass an explicit cached
// WithSigningKeyPEM.
func mustGenerateRSAPrivPEM(tb testing.TB) []byte {
	tb.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("mock.NewClient: rsa.GenerateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(k),
	})
}
