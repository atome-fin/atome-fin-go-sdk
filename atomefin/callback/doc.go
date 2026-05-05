// Package callback provides webhook receiver helpers for the inbound
// auth-terminal and capture-terminal notifications described in the
// atomefin white-label "G" spec.
//
// # Quick start
//
//	v, err := callback.FromClient(c) // single-cert convenience
//	if err != nil { log.Fatal(err) }
//
//	mux := http.NewServeMux()
//	mux.Handle("/atome/auth", callback.AuthHandler(v,
//	    func(ctx context.Context, e *payment.AuthResponse) error {
//	        // idempotent application of e to your state...
//	        return nil
//	    },
//	))
//	mux.Handle("/atome/capture", callback.CaptureHandler(v, ...))
//
// # Behavioural contract (DESIGN.md §8)
//
//   - Body is read via io.LimitReader (default 1 MiB, symmetric to the
//     outbound response cap). Oversize → HTTP 400.
//   - Authorization header is verified against the RAW BODY BYTES (the
//     bytes are also the signing canonical, so the handler MUST NOT
//     parse them before verification). Bad signature → HTTP 401 with
//     `AckResponse{Code: "INVALID_SIGNATURE"}`.
//   - On handler success → HTTP 200 with `AckResponse{Code: "SUCCESS"}`.
//   - On handler error → HTTP 500 so Atome retries.
//
// # Multi-cert support
//
// Verifier accepts a slice of underlying sign.Verifier implementations.
// Verification succeeds when ANY of them verifies. This is the design
// hook for cert-rotation overlap: during the cutover window, configure
// both the outgoing and incoming public keys; verification keeps
// passing on traffic signed by either.
//
// # Idempotency
//
// Atome callbacks are at-least-once. The package's contract is that
// the user-supplied handler is invoked exactly once per invocation —
// duplicate / retry detection is the partner's responsibility (typical
// pattern: dedupe on data.requestId at the application layer).
package callback
