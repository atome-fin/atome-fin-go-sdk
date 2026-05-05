package transport

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"math"
	"net/http"
	"time"
)

// RetryPolicy controls how the SDK retries failed requests. Defaults are
// conservative and tuned for the spec's behaviour: 3 attempts max
// (1 initial + 2 retries), retries on 5xx and transport-level failures,
// never on 4xx (which includes INVALID_SIGNATURE), jittered exponential
// backoff capped at 4s. See DESIGN.md §7.
//
// Zero-value policy is **not** functional — use DefaultRetryPolicy().
type RetryPolicy struct {
	// MaxAttempts is the inclusive upper bound on attempts. Must be >= 1.
	MaxAttempts int

	// Base is the unscaled backoff for the *first* retry. The actual sleep
	// is Base * 2^(attempt-1) ± Jitter, capped at Cap.
	Base time.Duration

	// Cap is the upper bound on a single backoff sleep.
	Cap time.Duration

	// Jitter is the relative jitter applied to each sleep. 0.20 means
	// "± 20% of the computed sleep". 0 disables jitter (deterministic
	// backoff — useful in tests).
	Jitter float64

	// RetryOnStatus reports whether an HTTP status code should be retried.
	// Default retries 500/502/503/504. Override to add 408 / 429 etc. once
	// rate-limit semantics are documented (DESIGN.md §13/Q9).
	RetryOnStatus func(status int) bool

	// RetryOnTransportError reports whether a transport-level error
	// (network, TLS, body-read) should be retried. Default uses
	// IsRetryableTransport from the atomefin package; transport defines
	// its own minimal version below to keep the dependency one-way.
	RetryOnTransportError func(err error) bool
}

// DefaultRetryPolicy returns a copy of the conservative default policy.
//
// Call sites: atomefin.New() when WithRetry is not passed; tests that
// want the production defaults; partners that want to start from the
// default and tweak one knob.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:           3,
		Base:                  250 * time.Millisecond,
		Cap:                   4 * time.Second,
		Jitter:                0.20,
		RetryOnStatus:         DefaultRetryOnStatus,
		RetryOnTransportError: DefaultRetryOnTransportError,
	}
}

// DefaultRetryOnStatus reports the spec-aligned set: 500/502/503/504.
// 408 and 429 are intentionally excluded until DESIGN §13/Q9 lands.
func DefaultRetryOnStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

// DefaultRetryOnTransportError is the policy default. transport keeps its
// own minimal version (no dependency on the umbrella package's
// IsRetryableTransport) so the atomefin package can wire its own
// detailed classifier in via WithRetry without forcing transport to
// import it. The default here mirrors the umbrella's behaviour:
//
//   - nil      → false
//   - any other err → true (covers timeouts, EOF, conn-reset, etc.)
//
// This is conservative-but-permissive on purpose: every retry preserves
// idempotency because the body (and therefore requestId) is unchanged
// per attempt (DESIGN.md §1.4).
func DefaultRetryOnTransportError(err error) bool {
	return err != nil
}

// Validate sanity-checks a policy. Used at Client construction time so
// configuration errors fail fast.
func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 {
		return retryError("MaxAttempts must be >= 1")
	}
	if p.Base < 0 {
		return retryError("Base must be >= 0")
	}
	if p.Cap < 0 {
		return retryError("Cap must be >= 0")
	}
	if p.Cap > 0 && p.Base > p.Cap {
		return retryError("Base must be <= Cap")
	}
	if p.Jitter < 0 || p.Jitter > 1 {
		return retryError("Jitter must be in [0,1]")
	}
	return nil
}

// Backoff computes the sleep duration before retrying after `attempt`.
// attempt is 1-indexed: Backoff(1) is the wait before the *second* attempt.
//
// Math: base * 2^(attempt-1), capped at Cap, then perturbed by ±Jitter.
// The jitter is drawn from crypto/rand to avoid the test-flake pattern
// where many clients in lockstep retry on the same millisecond boundary.
func (p RetryPolicy) Backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	// 2^(attempt-1), saturating to avoid overflow on absurd inputs.
	shift := attempt - 1
	if shift > 30 {
		shift = 30
	}
	d := p.Base << shift
	if p.Cap > 0 && d > p.Cap {
		d = p.Cap
	}
	if p.Jitter > 0 && d > 0 {
		// Draw r ∈ [-1, +1] from crypto/rand (8 bytes → uint64 → float64).
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err == nil {
			u := binary.BigEndian.Uint64(buf[:])
			// Map to [-1, +1) — bias is negligible for jitter purposes.
			r := float64(u)/float64(math.MaxUint64)*2 - 1
			d = time.Duration(float64(d) * (1 + p.Jitter*r))
			if d < 0 {
				d = 0
			}
		}
	}
	return d
}

// Sleep blocks for Backoff(attempt) or until ctx is done. Returns ctx.Err()
// if the context was cancelled, nil otherwise.
//
// Exposed for callers that drive their own retry loop; the SDK's HTTP layer
// uses it internally.
func (p RetryPolicy) Sleep(ctx context.Context, attempt int) error {
	d := p.Backoff(attempt)
	if d <= 0 {
		// Still respect ctx so a cancelled caller stops immediately.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryError is a thin wrapper so config errors carry a clear prefix
// without dragging fmt into this hot path.
type retryError string

func (e retryError) Error() string { return "atomefin/transport: retry policy: " + string(e) }
