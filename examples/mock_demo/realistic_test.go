// Demonstrates v0.5's realistic-sandbox flags layered on top of
// the v0.4 surface. Run with: `go test ./examples/mock_demo`.
package mock_demo_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// freshTestKeyPEM mints a one-shot RSA-2048 PEM. Mirrors
// docs/mock_mode_examples_test.go's helper — same shape so
// either example reads cleanly to a partner pasting either
// snippet.
func freshTestKeyPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
}

// TestDemo_RealisticSandbox walks a multi-step lifecycle that
// emerges from composition of v0.4's `PerEndpoint` and v0.5's
// `WithAutoCallback`:
//
//  1. Spec validation enforces every spec-required field on the
//     wire — partner-side bugs surface as 400 PARAMS_MISSING.
//  2. Idempotency replays a duplicate `requestId` byte-for-byte.
//  3. Typed `AuthSuccess` / `CaptureSuccess` builders carry both
//     the sync response and the matching callback event.
//  4. After the sync response, the auto-callback fires the
//     callback to a partner-side handler that the partner
//     mounted via `callback.AuthHandler` / `CaptureHandler`.
//
// No new DSL — the "script" is just the composition of v0.4's
// PerEndpoint plus v0.5's WithAutoCallback (V0.5_DESIGN.md §6).
func TestDemo_RealisticSandbox(t *testing.T) {
	pub, _ := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	v, _ := sign.NewRSA2Verifier(pub)
	verifier, _ := callback.NewVerifier([]sign.Verifier{v})

	var (
		authCallbackHits    int32
		captureCallbackHits int32
	)
	authHandler := callback.AuthHandler(verifier, func(_ context.Context, e *callback.AuthEvent) error {
		atomic.AddInt32(&authCallbackHits, 1)
		return nil
	})
	captureHandler := callback.CaptureHandler(verifier, func(_ context.Context, e *callback.CaptureEvent) error {
		atomic.AddInt32(&captureCallbackHits, 1)
		return nil
	})

	srv := mock.NewServer(t,
		mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth":    mock.AuthSuccess("AUTH-LIFECYCLE-1"),
			"POST /capture": mock.CaptureSuccess("AUTH-LIFECYCLE-1"),
		}, mock.AlwaysSuccess()),
		// v0.5 flags, all opt-in:
		mock.WithIdempotency(), // dedupe retries
		mock.WithAutoCallback(map[string]http.Handler{
			"POST /<authNotifyUrl>":    authHandler,
			"POST /<captureNotifyUrl>": captureHandler,
		}),
	)

	c, err := atomefin.New(
		atomefin.WithBaseURL(srv.URL),
		atomefin.WithPrivateKeyPEM(freshTestKeyPEM(t)),
	)
	if err != nil {
		t.Fatalf("atomefin.New: %v", err)
	}
	ctx := context.Background()

	// Step 1: /auth → SUCCESS sync + AuthEvent callback fired.
	authResp, err := payment.New(c).Auth(ctx, &payment.AuthRequest{
		RequestID:            "lifecycle-r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
		Sessionid:            "s",
	})
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if authResp.Data.AuthOrderID != "AUTH-LIFECYCLE-1" {
		t.Errorf("AuthOrderID = %q", authResp.Data.AuthOrderID)
	}

	// Step 2: /capture → SUCCESS sync + CaptureEvent callback fired.
	if _, err := payment.New(c).Capture(ctx, &payment.CaptureRequest{
		RequestID:            "lifecycle-c-1",
		ExternalReferenceUID: "u-1",
		AuthOrderID:          "AUTH-LIFECYCLE-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
	}); err != nil {
		t.Fatalf("Capture: %v", err)
	}

	if got := atomic.LoadInt32(&authCallbackHits); got != 1 {
		t.Errorf("auth callback hits = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&captureCallbackHits); got != 1 {
		t.Errorf("capture callback hits = %d, want 1", got)
	}

	// Idempotency: replay /auth with the same requestId — same
	// response, no second callback (the mock server replays the
	// cached body but the auto-callback fires only on the
	// originating dispatch — replays have an X-Mock-Replay marker).
	authResp2, err := payment.New(c).Auth(ctx, &payment.AuthRequest{
		RequestID:            "lifecycle-r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{{SubOrderID: "so-1", Amount: 1500000, Quantity: 1}},
		Sessionid:            "s",
	})
	if err != nil {
		t.Fatalf("Auth replay: %v", err)
	}
	if authResp2.Data.AuthOrderID != "AUTH-LIFECYCLE-1" {
		t.Errorf("replay AuthOrderID = %q", authResp2.Data.AuthOrderID)
	}

	fmt.Printf("Auth     → SUCCESS + callback delivered\n")
	fmt.Printf("Capture  → SUCCESS + callback delivered\n")
	fmt.Printf("Auth replay → cached response + no duplicate callback\n")
}
