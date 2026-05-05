// Command webhook_server demonstrates the inbound callback flow: it
// listens on ATOME_FIN_LISTEN_ADDR, mounts the auth-terminal and
// capture-terminal handlers under /atome/auth and /atome/capture,
// verifies signatures with the configured Atome public cert(s), and
// emits the canonical AckResponse for each.
//
// Build & run:
//
//	go build ./examples/webhook_server/
//	ATOME_FIN_ATOME_CERT_PEM=/etc/atome/atome.crt.pem \
//	ATOME_FIN_LISTEN_ADDR=:8443 \
//	./webhook_server
//
// To exercise multi-cert rotation overlap, set
// ATOME_FIN_ATOME_CERT_PEMS to a colon-separated list of PEM file
// paths instead of ATOME_FIN_ATOME_CERT_PEM.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/callback"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func main() {
	verifier := buildVerifier()

	addr := os.Getenv("ATOME_FIN_LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	mux := http.NewServeMux()
	mux.Handle("/atome/auth", callback.AuthHandler(verifier, onAuth))
	mux.Handle("/atome/capture", callback.CaptureHandler(verifier, onCapture))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("atomefin webhook listening on %s (atome keys configured: %d)",
		addr, verifier.KeyCount())
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

// buildVerifier reads either ATOME_FIN_ATOME_CERT_PEM (single cert,
// common path) or ATOME_FIN_ATOME_CERT_PEMS (colon-separated list, used
// during cert-rotation overlap windows per the spec).
func buildVerifier() *callback.Verifier {
	if list := os.Getenv("ATOME_FIN_ATOME_CERT_PEMS"); list != "" {
		paths := strings.Split(list, ":")
		pems := make([][]byte, 0, len(paths))
		for _, p := range paths {
			b, err := os.ReadFile(p)
			if err != nil {
				log.Fatalf("read cert %q: %v", p, err)
			}
			pems = append(pems, b)
		}
		v, err := callback.FromCertPEMs(pems)
		if err != nil {
			log.Fatalf("FromCertPEMs: %v", err)
		}
		return v
	}

	path := os.Getenv("ATOME_FIN_ATOME_CERT_PEM")
	if path == "" {
		log.Fatal("set ATOME_FIN_ATOME_CERT_PEM (single cert) or ATOME_FIN_ATOME_CERT_PEMS (colon-separated, multi-cert)")
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read cert %q: %v", path, err)
	}
	v, err := callback.FromCertPEMs([][]byte{pem})
	if err != nil {
		log.Fatalf("FromCertPEMs: %v", err)
	}
	return v
}

// onAuth is the partner's auth-terminal callback. It must be idempotent
// (callbacks are at-least-once); typical pattern is to dedupe on
// event.Data.RequestID at the application layer.
//
// Returning nil → 200 + ack. Returning an error → 500 (Atome retries).
func onAuth(_ context.Context, e *payment.AuthResponse) error {
	if e == nil || e.Data == nil {
		return fmt.Errorf("auth callback missing data envelope")
	}
	d := e.Data
	log.Printf("[auth] requestId=%s authOrderId=%s status=%s failureCode=%s",
		d.RequestID, d.AuthOrderID, d.Status, d.FailureCode)
	// TODO: partner-side application logic — mark order paid, etc.
	// MUST be idempotent against duplicate deliveries.
	return nil
}

// onCapture is the partner's capture-terminal callback.
func onCapture(_ context.Context, e *payment.CaptureResponse) error {
	if e == nil || e.Data == nil {
		return fmt.Errorf("capture callback missing data envelope")
	}
	d := e.Data
	log.Printf("[capture] requestId=%s orderId=%s authOrderId=%s status=%s",
		d.RequestID, d.OrderID, d.AuthOrderID, d.Status)
	return nil
}
