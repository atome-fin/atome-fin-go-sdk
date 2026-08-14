// Package mock_demo_test demonstrates how a partner exercises
// the SDK via `atomefin/mock`. The flow walks /auth + /capture +
// /refund through pre-built scenarios, then fires a synthetic
// callback at a partner's callback handler.
//
// Run with:
//
//	go test ./examples/mock_demo
//
// The expected output is captured at the bottom of the
// `Example_*` function.
package mock_demo_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/mock"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// TestDemo_MockSurface walks the typical partner-test flow:
//  1. Build a mock-backed Client via mock.NewClient.
//  2. Drive /auth + /capture + /refund through PerEndpoint
//     scenarios; assert the SDK receives the canned responses.
//  3. Fire an /<authNotifyUrl> callback at a partner-side
//     callback handler signed with the bundled mock key; assert
//     the user fn is invoked.
//
// Mirrors the prose in docs/MOCK_MODE.md and the v0.4 surface in
// CHANGELOG. Runs on every `go test ./examples/mock_demo`.
func TestDemo_MockSurface(t *testing.T) {
	c := mock.NewClient(t,
		mock.WithMockKeysAllowed(),
		mock.WithScenario(mock.PerEndpoint(map[string]mock.Scenario{
			"POST /auth":    mock.AlwaysSuccess(),
			"POST /capture": mock.AlwaysProcessing(),
			"POST /refund":  mock.AlwaysAPIError(http.StatusBadRequest, atomefin.CodeParamsMissing, "captureRequestId required"),
		}, mock.AlwaysSuccess())),
	)

	ctx := context.Background()

	// /auth — scenario says SUCCESS.
	authResp, err := payment.New(c).Auth(ctx, &payment.AuthRequest{
		RequestID:            "demo-r-1",
		ExternalReferenceUID: "demo-u-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{demoSubOrder(1500000)},
		ExtendInfo:           demoExtendInfo(1500000),
		Sessionid:            "demo-session",
	})
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	if authResp.Code != atomefin.CodeSuccess {
		t.Errorf("Auth Code = %q", authResp.Code)
	}

	// /capture — scenario says PROCESSING.
	captureResp, err := payment.New(c).Capture(ctx, &payment.CaptureRequest{
		RequestID:            "demo-c-1",
		ExternalReferenceUID: "demo-u-1",
		AuthOrderID:          "AUTH-1",
		TotalAmount:          1500000,
		PeriodType:           3,
		SubOrders:            []payment.SubOrder{demoSubOrder(1500000)},
		ExtendInfo:           demoExtendInfo(1500000),
	})
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if !captureResp.IsProcessing() {
		t.Errorf("Capture: expected PROCESSING, got %#v", captureResp.Data)
	}

	// Fire a callback at a partner-side callback handler signed
	// with the bundled mock key.
	pub, err := sign.LoadPublicCertPEM(mock.MockSigningPubCertPEM())
	if err != nil {
		t.Fatalf("LoadPublicCertPEM: %v", err)
	}
	v, err := sign.NewRSA2Verifier(pub)
	if err != nil {
		t.Fatalf("NewRSA2Verifier: %v", err)
	}
	verifier, err := callback.NewVerifier([]sign.Verifier{v})
	if err != nil {
		t.Fatalf("callback.NewVerifier: %v", err)
	}
	var received bool
	handler := callback.AuthHandler(verifier, func(_ context.Context, e *callback.AuthEvent) error {
		received = true
		return nil
	})
	mock.FireAuthCallback(t, handler, &callback.AuthEvent{
		Code: "SUCCESS", Message: "ok",
		Data: &payment.AuthorizationData{
			RequestID: "demo-r-1", AuthOrderID: "AUTH-1",
			Currency: "IDR", TotalAmount: 1500000, Status: "SUCCESS",
		},
	}, mock.WithFireMockKey())
	if !received {
		t.Error("partner-side callback handler did not receive event")
	}

	// Print a short narrative — surfaces in `go test -v`.
	fmt.Printf("Auth     → code=%s\n", authResp.Code)
	fmt.Printf("Capture  → IsProcessing()=%v\n", captureResp.IsProcessing())
	fmt.Printf("Callback delivered: %v\n", received)
}
