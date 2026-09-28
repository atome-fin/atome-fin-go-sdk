package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func pollStatus(s *atomefin.Status) atomefin.Status { return *s }

func TestPollMaxWaitStopsDuringBackoff(t *testing.T) {
	calls := 0
	processing := atomefin.StatusProcessing
	started := time.Now()
	resp, err := payment.PollUntilTerminal(context.Background(), payment.PollOptions{
		MaxWait: 30 * time.Millisecond, InitialDelay: time.Second,
	}, pollStatus, func(context.Context) (*atomefin.Status, error) {
		calls++
		return &processing, nil
	})
	var te *atomefin.TransportError
	if !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &te) || te.Op != "poll" {
		t.Fatalf("err=%v; want poll TransportError wrapping DeadlineExceeded", err)
	}
	if calls != 1 || resp != &processing {
		t.Errorf("calls=%d resp=%v; want one call and the last processing response", calls, resp)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Errorf("MaxWait=30ms took %v; slept beyond the polling budget", elapsed)
	}
}

func TestPollMaxWaitBoundsInflightCall(t *testing.T) {
	for _, lateSuccess := range []bool{false, true} {
		name := "context-aware request"
		if lateSuccess {
			name = "late success"
		}
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := time.Now()
			_, err := payment.PollUntilTerminal(parent, payment.PollOptions{MaxWait: 30 * time.Millisecond},
				pollStatus, func(ctx context.Context) (*atomefin.Status, error) {
					deadline, ok := ctx.Deadline()
					if !ok || deadline.After(started.Add(500*time.Millisecond)) {
						t.Fatal("request context did not inherit MaxWait")
					}
					<-ctx.Done()
					if lateSuccess {
						s := atomefin.StatusSuccess
						return &s, nil
					}
					return nil, ctx.Err()
				})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err=%v; want polling deadline even if request returns late success", err)
			}
			if parent.Err() != nil {
				t.Fatal("request consumed the parent budget instead of MaxWait")
			}
		})
	}
}

func TestPollParentCancellation(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		name := "during request"
		if alreadyCanceled {
			name = "before first request"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if alreadyCanceled {
				cancel()
			}
			calls := 0
			_, err := payment.PollUntilTerminal(ctx, payment.PollOptions{}, pollStatus,
				func(context.Context) (*atomefin.Status, error) {
					calls++
					cancel()
					return nil, context.Canceled
				})
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err=%v; want cancellation", err)
			}
			wantCalls := 1
			if alreadyCanceled {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Errorf("calls=%d; want %d", calls, wantCalls)
			}
		})
	}
}

func TestPollParentDeadlineWins(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := payment.PollUntilTerminal(ctx, payment.PollOptions{MaxWait: time.Second}, pollStatus,
		func(ctx context.Context) (*atomefin.Status, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})
	if err != context.DeadlineExceeded {
		t.Errorf("err=%v; want parent DeadlineExceeded", err)
	}
}
