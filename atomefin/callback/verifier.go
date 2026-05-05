package callback

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/sign"
)

// DefaultBodyLimit is the default maximum number of bytes the handler
// will read from a callback request body before rejecting it. 1 MiB —
// symmetric to the T2 outbound response cap.
const DefaultBodyLimit = 1 << 20

// Verifier authenticates inbound callback bodies. It accepts a SLICE of
// underlying sign.Verifier implementations and returns success when ANY
// of them verifies the signature. This composition supports the cert
// rotation pattern outlined in the spec / Q3b: during a cutover
// window the partner configures both the outgoing-soon and incoming
// public keys; verification keeps passing on traffic signed by either.
//
// A single-cert deployment is the common path; pass a one-element
// slice or use FromClient for the convenience constructor.
//
// Verifier is immutable after construction and safe for concurrent use.
type Verifier struct {
	verifiers []sign.Verifier
	bodyLimit int64
	clock     func() time.Time // reserved for future replay-window checks (Q5)
}

// VerifierOption configures a Verifier at construction time.
type VerifierOption func(*Verifier)

// WithBodyLimit overrides the default 1 MiB body size cap. n must be
// positive; non-positive values are silently ignored (the existing
// limit stays in effect) so partners that do `WithBodyLimit(0)` to
// "disable" don't accidentally let through a 4 GiB body.
func WithBodyLimit(n int64) VerifierOption {
	return func(v *Verifier) {
		if n > 0 {
			v.bodyLimit = n
		}
	}
}

// WithClock injects a clock used by future replay-window checks. Today
// it is reserved; passing a stub here is harmless.
func WithClock(now func() time.Time) VerifierOption {
	return func(v *Verifier) {
		if now != nil {
			v.clock = now
		}
	}
}

// NewVerifier composes a Verifier from one or more sign.Verifier
// implementations. Returns an error when verifiers is empty (a Verifier
// with no underlying keys would silently accept every signature, which
// is the worst possible default).
//
// The slice is copied; mutating the caller's slice afterwards has no
// effect on the Verifier.
func NewVerifier(verifiers []sign.Verifier, opts ...VerifierOption) (*Verifier, error) {
	if len(verifiers) == 0 {
		return nil, errors.New("atomefin/callback: NewVerifier requires at least one sign.Verifier")
	}
	for i, sv := range verifiers {
		if sv == nil {
			return nil, fmt.Errorf("atomefin/callback: NewVerifier: verifier[%d] is nil", i)
		}
	}
	v := &Verifier{
		verifiers: append([]sign.Verifier(nil), verifiers...),
		bodyLimit: DefaultBodyLimit,
		clock:     time.Now,
	}
	for _, o := range opts {
		if o != nil {
			o(v)
		}
	}
	return v, nil
}

// FromClient builds a single-cert Verifier from the Atome public key
// configured on c (atomefin.WithAtomePublicKey or
// WithAtomePublicCertPEM). Returns an error if no key is configured —
// partners running a multi-cert overlap should use NewVerifier or
// FromCertPEMs directly.
func FromClient(c *atomefin.Client, opts ...VerifierOption) (*Verifier, error) {
	if c == nil {
		return nil, errors.New("atomefin/callback: FromClient: nil *Client")
	}
	sv := c.Verifier()
	if sv == nil {
		return nil, errors.New("atomefin/callback: FromClient: Client has no Atome public key configured (use atomefin.WithAtomePublicKey)")
	}
	return NewVerifier([]sign.Verifier{sv}, opts...)
}

// FromCertPEMs parses one or more PEM-encoded Atome public certs and
// returns a Verifier that succeeds when ANY of them verifies. Use
// during cert-rotation cutover.
//
// Each PEM is parsed via sign.LoadPublicCertPEM and wrapped in a
// PKCS#1-v1.5 / SHA-256 verifier. Partners running PSS pass
// individually-constructed sign.Verifier values to NewVerifier instead.
func FromCertPEMs(pems [][]byte, opts ...VerifierOption) (*Verifier, error) {
	if len(pems) == 0 {
		return nil, errors.New("atomefin/callback: FromCertPEMs: no PEMs provided")
	}
	verifiers := make([]sign.Verifier, 0, len(pems))
	for i, p := range pems {
		key, err := sign.LoadPublicCertPEM(p)
		if err != nil {
			return nil, fmt.Errorf("atomefin/callback: FromCertPEMs[%d]: %w", i, err)
		}
		sv, err := sign.NewRSA2Verifier(key)
		if err != nil {
			return nil, fmt.Errorf("atomefin/callback: FromCertPEMs[%d]: %w", i, err)
		}
		verifiers = append(verifiers, sv)
	}
	return NewVerifier(verifiers, opts...)
}

// Verify checks signature against body using each configured verifier
// in order. Returns nil on the FIRST success; returns the LAST error
// when every verifier rejects (so callers can errors.Is(err,
// sign.ErrSignature)).
//
// We do not return early on a non-signature error type because every
// underlying verifier produces sign.ErrSignature-wrapped failures
// today. Future ctx-cancellation handling may change this — for now
// the contract is "all verifiers are tried".
func (v *Verifier) Verify(ctx context.Context, body []byte, signature string) error {
	if v == nil || len(v.verifiers) == 0 {
		return errors.New("atomefin/callback: Verifier not configured")
	}
	if signature == "" {
		return fmt.Errorf("%w: empty signature", sign.ErrSignature)
	}
	var last error
	for _, sv := range v.verifiers {
		if err := sv.Verify(ctx, body, signature); err == nil {
			return nil
		} else {
			last = err
		}
	}
	if last == nil {
		// Defensive: should be impossible because verifiers is non-empty.
		last = sign.ErrSignature
	}
	return last
}

// BodyLimit returns the configured maximum body size in bytes.
func (v *Verifier) BodyLimit() int64 {
	if v == nil {
		return 0
	}
	return v.bodyLimit
}

// KeyCount returns the number of configured verifiers. Useful for
// asserting "we are in multi-cert overlap mode" in tests.
func (v *Verifier) KeyCount() int {
	if v == nil {
		return 0
	}
	return len(v.verifiers)
}
