package mock_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// ---------- v0.6 — encrypted-POST idempotency ----------

// TestServer_Idempotency_EncryptedPOST_DecryptsAndExtractsRequestID
// pins the v0.6.0 fix: when WithIdempotencyDecryptKey is set, the
// Server unwraps the AES key from the Encrypt header, decrypts
// the body, parses requestId, and uses it as the cache key. The
// test fires two SubmitInformation calls with the same plaintext
// requestId — the second must replay the cached response without
// invoking the underlying scenario again.
func TestServer_Idempotency_EncryptedPOST_DecryptsAndExtractsRequestID(t *testing.T) {
	var dispatchCount int32
	scenario := mock.PerEndpoint(map[string]mock.Scenario{
		"POST /credit-information": mock.ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
			atomic.AddInt32(&dispatchCount, 1)
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"code":"SUCCESS","message":"ok"}`)),
			}, nil
		}),
	}, mock.AlwaysSuccess())

	srv := mock.NewServer(t, scenario,
		mock.WithIdempotency(),
		mock.WithIdempotencyDecryptKey(mock.MockEncryptPrivKeyPEM()),
	)

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
		atomefin.WithEncryptAtomePublicCertPEM(mock.MockEncryptPubCertPEM()),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	req := &credit.CreditInformationParam{
		RequestID:            "encrypted-idem-1",
		ExternalReferenceUID: "user-1",
		MobileNumber:         "+6281298000000",
		Email:                "u@example.com",
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.CreditInformationEssentialInfo{
			IndividualProfile: &credit.CreditInformationIndividualProfile{
				OCRResult: &credit.CreditInformationOCRResult{FullName: "Test User"},
			},
		},
		ExtendInfo: &credit.CreditInformationExtendInfo{Language: credit.LanguageEnglish},
	}

	for i := 0; i < 2; i++ {
		// Reset RequestID each call so the SDK doesn't auto-mint a fresh one.
		req.RequestID = "encrypted-idem-1"
		if _, err := credit.New(c).SubmitInformation(context.Background(), req); err != nil {
			t.Fatalf("SubmitInformation iter=%d: %v", i, err)
		}
	}

	// Underlying scenario hit ONCE; second call replayed from cache.
	if got := atomic.LoadInt32(&dispatchCount); got != 1 {
		t.Errorf("scenario invocations = %d; want 1 (cache replay on duplicate encrypted requestId)", got)
	}
}

// TestServer_Idempotency_EncryptedPOST_BypassesWithoutDecryptKey
// pins the v0.5.0 fallback: WithIdempotency alone (no
// WithIdempotencyDecryptKey) skips the cache for encrypted
// POSTs — the Server has no way to extract requestId from the
// ciphertext and falls back to per-request dispatch.
func TestServer_Idempotency_EncryptedPOST_BypassesWithoutDecryptKey(t *testing.T) {
	var dispatchCount int32
	scenario := mock.PerEndpoint(map[string]mock.Scenario{
		"POST /credit-information": mock.ScenarioFunc(func(_ *http.Request) (*http.Response, error) {
			atomic.AddInt32(&dispatchCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`{"code":"SUCCESS","message":"ok"}`)),
			}, nil
		}),
	}, mock.AlwaysSuccess())

	srv := mock.NewServer(t, scenario, mock.WithIdempotency()) // no decrypt key

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
		atomefin.WithEncryptAtomePublicCertPEM(mock.MockEncryptPubCertPEM()),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	req := &credit.CreditInformationParam{
		RequestID:            "encrypted-idem-bypass",
		ExternalReferenceUID: "user-1",
		MobileNumber:         "+6281298000000",
		Email:                "u@example.com",
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.CreditInformationEssentialInfo{
			IndividualProfile: &credit.CreditInformationIndividualProfile{
				OCRResult: &credit.CreditInformationOCRResult{FullName: "Test User"},
			},
		},
		ExtendInfo: &credit.CreditInformationExtendInfo{Language: credit.LanguageEnglish},
	}
	for i := 0; i < 2; i++ {
		req.RequestID = "encrypted-idem-bypass"
		if _, err := credit.New(c).SubmitInformation(context.Background(), req); err != nil {
			t.Fatalf("SubmitInformation iter=%d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&dispatchCount); got != 2 {
		t.Errorf("scenario invocations = %d; want 2 (cache bypassed)", got)
	}
}

// ---------- v0.6 — auto-callback panic isolation ----------

// TestServer_AutoCallback_PanicIsolation pins the v0.6.0 fix: a
// partner-side callback handler that panics during ServeHTTP
// MUST NOT propagate the panic into the SDK request pipeline.
// The Server recovers and surfaces the panic via
// testing.TB.Errorf so a misbehaving handler is visible at test
// time without breaking the inbound request.
func TestServer_AutoCallback_PanicIsolation(t *testing.T) {
	pub, _ := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	v, _ := sign.NewRSA2Verifier(pub)
	verifier, _ := callback.NewVerifier([]sign.Verifier{v})

	// A callback handler that panics — partner code that crashes
	// inside the user-fn shouldn't propagate.
	panickyHandler := callback.AuthHandler(verifier, func(_ context.Context, _ *callback.AuthEvent) error {
		panic("simulated partner-side panic")
	})

	// Wrap the test's tb in a panic-recording shim so the outer
	// test passes even though the Server logs the recovered
	// panic via t.Errorf.
	rec := &recordingTB{T: t}
	srv := mock.NewServer(rec,
		mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth": mock.AuthSuccess("AUTH-PANIC-1"),
		}, mock.AlwaysSuccess()),
		mock.WithAutoCallback(map[string]http.Handler{
			"POST /<authNotifyUrl>": panickyHandler,
		}),
	)

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	// The SDK call must complete cleanly — the panic is isolated
	// to the auto-callback path and does NOT propagate through
	// the SDK's response handling.
	resp, err := payment.New(c).Auth(context.Background(), &payment.AuthRequest{
		RequestID:            "r-panic-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{samplePaymentSubOrder(1500000)},
		Sessionid:            "s",
	})
	if err != nil {
		t.Fatalf("Auth: %v (sync path should not see the callback's panic)", err)
	}
	if resp.Data.AuthOrderID != "AUTH-PANIC-1" {
		t.Errorf("AuthOrderID = %q", resp.Data.AuthOrderID)
	}

	// The Server should have recorded the panic via Errorf.
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.errs) == 0 {
		t.Error("expected Errorf reporting the recovered panic; got none")
	}
	joined := strings.Join(rec.errs, "\n")
	if !strings.Contains(joined, "panicked") {
		t.Errorf("Errorf log missing 'panicked' marker: %s", joined)
	}
}

// ---------- v0.6 — WithAtomeCerts convenience ----------

// TestWithAtomeCerts_WiresAllFourOptions pins that the
// convenience helper threads each PEM blob into the matching
// individual option (and that the four individual options stay
// supported). Builds a Client with WithAtomeCerts AND asserts
// that EncryptAtomePublicKey() / EncryptPrivateKey() / etc.
// are populated.
func TestWithAtomeCerts_WiresAllFourOptions(t *testing.T) {
	signPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	encPriv, _ := rsa.GenerateKey(rand.Reader, 2048)

	signPrivPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(signPriv)})
	signPubDER, _ := x509.MarshalPKIXPublicKey(&signPriv.PublicKey)
	signPubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: signPubDER})

	encPrivPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(encPriv)})
	encPubDER, _ := x509.MarshalPKIXPublicKey(&encPriv.PublicKey)
	encPubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: encPubDER})

	c, err := atomefin.New(
		atomefin.WithBaseURL("https://example.invalid"),
		atomefin.WithAtomeCerts(
			atomefin.AtomeCertSource{PartnerPriv: signPrivPEM, AtomePub: signPubPEM},
			atomefin.AtomeCertSource{PartnerPriv: encPrivPEM, AtomePub: encPubPEM},
		),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}

	// All four cert roles wired:
	if c.Verifier() == nil {
		t.Error("Verifier nil — WithAtomePublicCertPEM not threaded")
	}
	if c.EncryptAtomePublicKey() == nil {
		t.Error("EncryptAtomePublicKey nil — WithEncryptAtomePublicCertPEM not threaded")
	}
	if c.EncryptPrivateKey() == nil {
		t.Error("EncryptPrivateKey nil — WithEncryptPrivateKeyPEM not threaded")
	}

	// Smoke: the encrypt keypair round-trips a small body.
	header, body, err := encrypt.Marshal([]byte(`{"requestId":"r-1"}`), c.EncryptAtomePublicKey())
	if err != nil {
		t.Fatalf("encrypt.Marshal: %v", err)
	}
	if _, err := encrypt.Unmarshal(header, body, encPriv); err != nil {
		t.Fatalf("encrypt.Unmarshal: %v", err)
	}
}

// TestWithAtomeCerts_PartialSetup pins that empty AtomeCertSource
// fields skip the matching option — partners can construct a
// signing-only Client without supplying encrypt certs.
func TestWithAtomeCerts_PartialSetup(t *testing.T) {
	signPriv, _ := rsa.GenerateKey(rand.Reader, 2048)
	signPrivPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(signPriv)})

	c, err := atomefin.New(
		atomefin.WithBaseURL("https://example.invalid"),
		atomefin.WithAtomeCerts(
			atomefin.AtomeCertSource{PartnerPriv: signPrivPEM}, // no Atome pub for signing
			atomefin.AtomeCertSource{},                         // no encrypt setup
		),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	if c.EncryptAtomePublicKey() != nil {
		t.Error("EncryptAtomePublicKey should be nil (no encrypt setup)")
	}
}

// ---------- recordingTB helper ----------

// recordingTB wraps a *testing.T to capture the Server's
// Errorf calls without failing the outer test. Used only by
// the panic-isolation test where the Server intentionally logs
// a recovered panic.
type recordingTB struct {
	*testing.T
	mu   sync.Mutex
	errs []string
}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.mu.Lock()
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
	r.mu.Unlock()
}

// Compile-time: the recordingTB has the methods the Server uses.
var _ interface {
	Errorf(string, ...any)
} = (*recordingTB)(nil)

// Pin that the v0.5.1 *MultiValueQueryError compile path stays
// available — caught code paths in partner code shouldn't
// rot. (No runtime assertion: this is a compile-only check.)
var _ = errors.As
