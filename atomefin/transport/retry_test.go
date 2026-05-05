package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestDefaultRetryPolicyDefaults(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", p.MaxAttempts)
	}
	if p.Base != 250*time.Millisecond {
		t.Errorf("Base = %v, want 250ms", p.Base)
	}
	if p.Cap != 4*time.Second {
		t.Errorf("Cap = %v, want 4s", p.Cap)
	}
	if p.Jitter != 0.20 {
		t.Errorf("Jitter = %v, want 0.20", p.Jitter)
	}
	if p.RetryOnStatus == nil || p.RetryOnTransportError == nil {
		t.Error("default policy must populate the retry callbacks")
	}
}

func TestDefaultRetryOnStatus(t *testing.T) {
	yes := []int{500, 502, 503, 504}
	for _, s := range yes {
		if !DefaultRetryOnStatus(s) {
			t.Errorf("DefaultRetryOnStatus(%d) = false, want true", s)
		}
	}
	no := []int{200, 201, 204, 400, 401, 403, 404, 408, 409, 422, 429, 501, 505, 511}
	for _, s := range no {
		if DefaultRetryOnStatus(s) {
			t.Errorf("DefaultRetryOnStatus(%d) = true, want false", s)
		}
	}
	// Sanity: stdlib constants match.
	if !DefaultRetryOnStatus(http.StatusInternalServerError) {
		t.Error("expected 500 to retry")
	}
}

func TestDefaultRetryOnTransportError(t *testing.T) {
	if DefaultRetryOnTransportError(nil) {
		t.Error("nil should not be retried")
	}
	if !DefaultRetryOnTransportError(io.EOF) {
		t.Error("io.EOF should be retried")
	}
}

func TestRetryPolicyValidate(t *testing.T) {
	good := DefaultRetryPolicy()
	if err := good.Validate(); err != nil {
		t.Errorf("default policy must validate, got %v", err)
	}

	bad := []RetryPolicy{
		{MaxAttempts: 0, Base: 1, Cap: 2, Jitter: 0.1},
		{MaxAttempts: 3, Base: -1, Cap: 2, Jitter: 0.1},
		{MaxAttempts: 3, Base: 1, Cap: -1, Jitter: 0.1},
		{MaxAttempts: 3, Base: 5, Cap: 2, Jitter: 0.1},
		{MaxAttempts: 3, Base: 1, Cap: 2, Jitter: 1.5},
		{MaxAttempts: 3, Base: 1, Cap: 2, Jitter: -0.1},
	}
	for i, p := range bad {
		if err := p.Validate(); err == nil {
			t.Errorf("policy[%d] should fail Validate, got nil", i)
		}
	}
}

func TestBackoffMonotoneAndCapped(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts: 5,
		Base:        100 * time.Millisecond,
		Cap:         400 * time.Millisecond,
		// Disable jitter for deterministic comparison.
		Jitter: 0,
	}
	got := []time.Duration{
		p.Backoff(1), // 100ms
		p.Backoff(2), // 200ms
		p.Backoff(3), // 400ms (capped)
		p.Backoff(4), // 400ms (capped)
		p.Backoff(5), // 400ms (capped)
	}
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		400 * time.Millisecond,
		400 * time.Millisecond,
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("Backoff(%d) = %v, want %v", i+1, got[i], w)
		}
	}
}

func TestBackoffJitterStaysWithinBand(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts: 3,
		Base:        1 * time.Second,
		Cap:         1 * time.Second,
		Jitter:      0.20,
	}
	for i := 0; i < 200; i++ {
		d := p.Backoff(1)
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("Backoff outside ±20%% band: %v", d)
		}
	}
}

func TestBackoffSaturation(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts: 100,
		Base:        time.Millisecond,
		Cap:         time.Second,
		Jitter:      0,
	}
	// Should not overflow / panic for large attempt numbers.
	if got := p.Backoff(60); got != time.Second {
		t.Errorf("Backoff(60) = %v, want 1s (cap)", got)
	}
}

func TestSleepRespectsContext(t *testing.T) {
	p := RetryPolicy{
		MaxAttempts: 3,
		Base:        500 * time.Millisecond,
		Cap:         500 * time.Millisecond,
		Jitter:      0,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	err := p.Sleep(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Sleep err = %v, want context.Canceled", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Error("Sleep should return immediately on cancelled ctx")
	}
}

func TestSleepZeroBackoffStillReturns(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 1, Base: 0, Cap: 0, Jitter: 0}
	if err := p.Sleep(context.Background(), 1); err != nil {
		t.Errorf("Sleep zero-backoff: %v", err)
	}
}
