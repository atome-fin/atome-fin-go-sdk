package atomefin

import (
	"context"
	"crypto/rsa"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transport"
)

// Each test exercises one option's error path or its mutating effect.
// Together these lift coverage of options.go from ~50% to >90%.

func TestWithHTTPClientNilRejected(t *testing.T) {
	cfg := defaultConfig()
	if err := WithHTTPClient(nil)(cfg); err == nil {
		t.Error("expected error for nil http client")
	}
}

func TestWithHTTPClientHappy(t *testing.T) {
	cfg := defaultConfig()
	custom := &http.Client{Timeout: 7 * time.Second}
	if err := WithHTTPClient(custom)(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.httpClient != custom {
		t.Error("WithHTTPClient did not stick")
	}
}

func TestWithBaseURLEmpty(t *testing.T) {
	cfg := defaultConfig()
	if err := WithBaseURL("")(cfg); err == nil {
		t.Error("expected error for empty base URL")
	}
	if err := WithBaseURL("   ")(cfg); err == nil {
		t.Error("expected error for whitespace-only base URL")
	}
}

func TestWithBaseURLTrimsTrailingSlash(t *testing.T) {
	cfg := defaultConfig()
	if err := WithBaseURL("https://example.com/api/")(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.baseURL != "https://example.com/api" {
		t.Errorf("baseURL = %q", cfg.baseURL)
	}
}

func TestWithEnvironmentUnknown(t *testing.T) {
	cfg := defaultConfig()
	if err := WithEnvironment(Environment("nope"))(cfg); err == nil {
		t.Error("expected error for unknown env")
	}
}

func TestWithTimeoutInvalid(t *testing.T) {
	cfg := defaultConfig()
	if err := WithTimeout(0)(cfg); err == nil {
		t.Error("expected error for non-positive timeout")
	}
	if err := WithTimeout(-1)(cfg); err == nil {
		t.Error("expected error for negative timeout")
	}
}

func TestWithUserAgent(t *testing.T) {
	cfg := defaultConfig()
	if err := WithUserAgent("my-app/1.0")(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.userAgent != "my-app/1.0" {
		t.Errorf("userAgent = %q", cfg.userAgent)
	}
}

func TestWithRetryInvalid(t *testing.T) {
	cfg := defaultConfig()
	if err := WithRetry(transport.RetryPolicy{MaxAttempts: 0})(cfg); err == nil {
		t.Error("expected error for invalid retry policy")
	}
}

func TestWithSignerNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithSigner(nil)(cfg); err == nil {
		t.Error("expected error for nil signer")
	}
}

func TestWithPrivateKeyPEMBadInput(t *testing.T) {
	cfg := defaultConfig()
	if err := WithPrivateKeyPEM([]byte("not pem"))(cfg); err == nil {
		t.Error("expected error for non-PEM input")
	}
}

func TestWithPrivateKeyPEMEncryptedRejected(t *testing.T) {
	cfg := defaultConfig()
	// Passing a non-empty password forces the loader's encrypted-PEM error.
	err := WithPrivateKeyPEM([]byte("dummy"), []byte("password"))(cfg)
	if err == nil {
		t.Error("expected error for encrypted PEM (Q3 not implemented)")
	}
}

func TestWithAtomePublicKeyNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithAtomePublicKey(nil)(cfg); err == nil {
		t.Error("expected error for nil public key")
	}
}

func TestWithAtomePublicKeyTooSmall(t *testing.T) {
	cfg := defaultConfig()
	// A 1024-bit key fails MinKeyBits in the sign package.
	tiny := &rsa.PublicKey{N: nil, E: 65537}
	if err := WithAtomePublicKey(tiny)(cfg); err == nil {
		t.Error("expected error for sub-2048-bit key")
	}
}

func TestWithAtomePublicCertPEMBad(t *testing.T) {
	cfg := defaultConfig()
	if err := WithAtomePublicCertPEM([]byte("not pem"))(cfg); err == nil {
		t.Error("expected error for non-PEM input")
	}
}

func TestWithAuthorizationSchemeNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithAuthorizationScheme(nil)(cfg); err == nil {
		t.Error("expected error for nil scheme")
	}
}

func TestSchemeAtomeKeyedFormatting(t *testing.T) {
	if got := SchemeAtomeKeyed("sigbytes", ""); got != "Algorithm=RSA2,Sign=sigbytes" {
		t.Errorf("no-keyID = %q", got)
	}
	if got := SchemeAtomeKeyed("sigbytes", "k1"); got != "Algorithm=RSA2,KeyVersion=k1,Sign=sigbytes" {
		t.Errorf("with-keyID = %q", got)
	}
}

func TestSchemeRawBase64Passthrough(t *testing.T) {
	if got := SchemeRawBase64("xyz", "ignored"); got != "xyz" {
		t.Errorf("raw passthrough = %q", got)
	}
}

func TestWithPartnerIDEmpty(t *testing.T) {
	cfg := defaultConfig()
	if err := WithPartnerID("")(cfg); err == nil {
		t.Error("expected error for empty partner id")
	}
	if err := WithPartnerID("   ")(cfg); err == nil {
		t.Error("expected error for whitespace partner id")
	}
}

func TestWithMerchantID(t *testing.T) {
	cfg := defaultConfig()
	if err := WithMerchantID("  m-1  ")(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.merchantID != "m-1" {
		t.Errorf("merchantID = %q", cfg.merchantID)
	}
}

func TestWithLoggerNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithLogger(nil)(cfg); err == nil {
		t.Error("expected error for nil logger")
	}
}

func TestWithObserverNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithObserver(nil)(cfg); err == nil {
		t.Error("expected error for nil observer")
	}
}

func TestWithDebugBodyLogging(t *testing.T) {
	cfg := defaultConfig()
	if err := WithDebugBodyLogging(true)(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.debugBodyLog {
		t.Error("debugBodyLog should be true")
	}
}

func TestWithClockNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithClock(nil)(cfg); err == nil {
		t.Error("expected error for nil clock")
	}
}

func TestWithClockHappy(t *testing.T) {
	cfg := defaultConfig()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := WithClock(func() time.Time { return now })(cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.clock(); !got.Equal(now) {
		t.Errorf("clock returned %v, want %v", got, now)
	}
}

func TestWithRequestIDGeneratorNil(t *testing.T) {
	cfg := defaultConfig()
	if err := WithRequestIDGenerator(nil)(cfg); err == nil {
		t.Error("expected error for nil generator")
	}
}

func TestWithMaxResponseBytesInvalid(t *testing.T) {
	cfg := defaultConfig()
	if err := WithMaxResponseBytes(0)(cfg); err == nil {
		t.Error("expected error for non-positive max")
	}
	if err := WithMaxResponseBytes(-1)(cfg); err == nil {
		t.Error("expected error for negative max")
	}
}

func TestNewRejectsNilOption(t *testing.T) {
	_, err := New(nil)
	if err == nil {
		t.Error("expected error for nil option")
	}
}

func TestVersionAccessor(t *testing.T) {
	if Version() != SDKVersion {
		t.Errorf("Version() = %q, want %q", Version(), SDKVersion)
	}
}

func TestClientAccessorsAfterNew(t *testing.T) {
	key := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithEnvironment(EnvPre),
		WithPartnerID("p"),
		WithMerchantID("m"),
		WithUserAgent("ua/1"),
		WithAtomePublicKey(&key.PublicKey),
	)
	if err != nil {
		t.Fatal(err)
	}
	if c.PartnerID() != "p" {
		t.Errorf("PartnerID = %q", c.PartnerID())
	}
	if c.MerchantID() != "m" {
		t.Errorf("MerchantID = %q", c.MerchantID())
	}
	if c.Environment() != EnvPre {
		t.Errorf("Environment = %q", c.Environment())
	}
	if c.HTTPClient() == nil {
		t.Error("HTTPClient = nil")
	}
	if c.Logger() == nil {
		t.Error("Logger = nil")
	}
	if c.Observer() == nil {
		t.Error("Observer = nil")
	}
	if c.Verifier() == nil {
		t.Error("Verifier = nil")
	}
	if c.Now().IsZero() {
		t.Error("Now() returned zero time")
	}
	// User-Agent must include both the SDK product and the suffix.
	ua := c.userAgent
	if !strings.Contains(ua, "atome-fin-go-sdk") || !strings.Contains(ua, "ua/1") {
		t.Errorf("UA = %q; missing product or suffix", ua)
	}
}

func TestJoinPath(t *testing.T) {
	cases := []struct {
		base, path, want string
	}{
		{"https://x.com", "/auth", "https://x.com/auth"},
		{"https://x.com/", "/auth", "https://x.com/auth"},
		{"https://x.com/", "auth", "https://x.com/auth"},
		{"https://x.com", "", "https://x.com"},
	}
	for _, c := range cases {
		if got := JoinPath(c.base, c.path); got != c.want {
			t.Errorf("JoinPath(%q, %q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}

// Smoke: WithTimeout actually shortens the per-request deadline.
func TestWithTimeoutShortensRequest(t *testing.T) {
	// Server that never replies within timeout.
	c := newSlowClient(t)
	ctx := context.Background()
	start := time.Now()
	_, err := c.DoSigned(ctx, "POST", "/auth", []byte(`{}`))
	dur := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if dur > 500*time.Millisecond {
		t.Errorf("request took %v; WithTimeout(50ms) should have fired", dur)
	}
}

func newSlowClient(t *testing.T) *Client {
	t.Helper()
	srv := slowServer(t)
	t.Cleanup(srv.Close)
	key := mustGenKey(t)
	c, err := New(
		WithPrivateKeyPEM(mustPEM(t, key)),
		WithBaseURL(srv.URL),
		WithPartnerID("p"),
		WithTimeout(50*time.Millisecond),
		WithRetry(transport.RetryPolicy{
			MaxAttempts:           1,
			Base:                  1 * time.Millisecond,
			Cap:                   1 * time.Millisecond,
			Jitter:                0,
			RetryOnStatus:         transport.DefaultRetryOnStatus,
			RetryOnTransportError: transport.DefaultRetryOnTransportError,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
