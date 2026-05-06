// Command auth_capture demonstrates the end-to-end happy path for the
// atomefin white-label "G" SDK: build a Client, submit /auth, then
// /capture, printing the result of each call.
//
// The example reads its configuration from environment variables so it
// can `go build` without any secrets present:
//
//	ATOME_FIN_PRIV_KEY_PEM    path to the partner's RSA-2048 private key (PEM)
//	ATOME_FIN_PARTNER_ID      partner identifier — OPTIONAL log-enrichment label
//	                          (Q7 RESOLVED: not transmitted on the wire)
//	ATOME_FIN_ENV             one of test|pre|prod  (default: test)
//	ATOME_FIN_BASE_URL        explicit base URL — overrides ATOME_FIN_ENV
//	ATOME_FIN_SESSION_ID      sessionid header value for /auth (max 64 chars)
//	ATOME_FIN_EXTERNAL_UID    partner-side user identifier
//	ATOME_FIN_TOTAL_AMOUNT    integer minor units (default 1500000 → IDR 15,000)
//	ATOME_FIN_PERIOD_TYPE     installment tenor 1/3/6/9/12 (default 3)
//	ATOME_FIN_RUN_CAPTURE     "1" to also issue /capture after a SUCCESS auth
//
// Build & run:
//
//	go build ./examples/auth_capture/
//	ATOME_FIN_PRIV_KEY_PEM=/etc/atome/partner.pem \
//	ATOME_FIN_SESSION_ID=session-xyz \
//	ATOME_FIN_EXTERNAL_UID=user-42 \
//	./auth_capture
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func main() {
	privPath := mustEnv("ATOME_FIN_PRIV_KEY_PEM")

	priv, err := os.ReadFile(privPath)
	if err != nil {
		log.Fatalf("read private key %q: %v", privPath, err)
	}

	opts := []atomefin.Option{
		atomefin.WithPrivateKeyPEM(priv),
	}
	// PartnerID is optional log-enrichment metadata only (Q7 RESOLVED).
	if partnerID := os.Getenv("ATOME_FIN_PARTNER_ID"); partnerID != "" {
		opts = append(opts, atomefin.WithPartnerID(partnerID))
	}
	if base := os.Getenv("ATOME_FIN_BASE_URL"); base != "" {
		opts = append(opts, atomefin.WithBaseURL(base))
	} else {
		env := atomefin.EnvTest
		switch os.Getenv("ATOME_FIN_ENV") {
		case "pre":
			env = atomefin.EnvPre
		case "prod":
			env = atomefin.EnvProd
		case "test", "":
			env = atomefin.EnvTest
		}
		opts = append(opts, atomefin.WithEnvironment(env))
	}

	c, err := atomefin.New(opts...)
	if err != nil {
		log.Fatalf("atomefin.New: %v", err)
	}
	defer func() { _ = c.Close() }()

	svc := payment.New(c)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	authReq := &payment.AuthRequest{
		// RequestID intentionally left empty; the SDK auto-mints a
		// ULID-like idempotency key. Partners that want to embed
		// their order ID prefix should set it explicitly.
		ExternalReferenceUID: envOr("ATOME_FIN_EXTERNAL_UID", "user-42"),
		TotalAmount:          envInt64("ATOME_FIN_TOTAL_AMOUNT", 1500000),
		PeriodType:           int(envInt64("ATOME_FIN_PERIOD_TYPE", 3)),
		SubOrders: []payment.SubOrder{
			{
				SubOrderID: "so-1",
				Amount:     envInt64("ATOME_FIN_TOTAL_AMOUNT", 1500000),
				Quantity:   1,
				SkuName:    "Demo widget",
			},
		},
		Sessionid: mustEnv("ATOME_FIN_SESSION_ID"),
	}

	authResp, err := svc.Auth(ctx, authReq)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}

	fmt.Println("---- /auth ----")
	fmt.Printf("requestId   : %s\n", authReq.RequestID)
	fmt.Printf("envelope.code: %s\n", authResp.Code)
	if authResp.Data != nil {
		fmt.Printf("authOrderId : %s\n", authResp.Data.AuthOrderID)
		fmt.Printf("status      : %s\n", authResp.Data.Status)
		if authResp.Data.FailureCode != "" {
			fmt.Printf("failureCode : %s\n", authResp.Data.FailureCode)
		}
	}

	if !authResp.IsTerminal() {
		fmt.Println("status is PROCESSING — call AuthPollUntilTerminal or wait for callback.")
		return
	}
	if authResp.Data == nil || authResp.Data.Status != atomefin.StatusSuccess {
		// Terminal but not SUCCESS — nothing to capture.
		return
	}

	if os.Getenv("ATOME_FIN_RUN_CAPTURE") != "1" {
		fmt.Println("\n(set ATOME_FIN_RUN_CAPTURE=1 to also issue /capture)")
		return
	}

	captureReq := &payment.CaptureRequest{
		ExternalReferenceUID: authReq.ExternalReferenceUID,
		AuthOrderID:          authResp.Data.AuthOrderID,
		TotalAmount:          authReq.TotalAmount,
		PeriodType:           authReq.PeriodType,
		SubOrders:            authReq.SubOrders,
	}
	captureResp, err := svc.Capture(ctx, captureReq)
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	fmt.Println("\n---- /capture ----")
	fmt.Printf("requestId   : %s\n", captureReq.RequestID)
	fmt.Printf("envelope.code: %s\n", captureResp.Code)
	if captureResp.Data != nil {
		fmt.Printf("orderId     : %s\n", captureResp.Data.OrderID)
		fmt.Printf("authOrderId : %s\n", captureResp.Data.AuthOrderID)
		fmt.Printf("status      : %s\n", captureResp.Data.Status)
	}
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required environment variable %s is empty", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		log.Fatalf("env %s: %v", key, err)
	}
	return n
}
