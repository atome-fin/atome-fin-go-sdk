package payment_test

import (
	"context"
	"errors"
	"strings"
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

type logLine struct {
	level, msg string
	kv         map[string]any
}

type recLogger struct{ lines []logLine }

func (l *recLogger) add(level, msg string, kv []any) {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	l.lines = append(l.lines, logLine{level, msg, m})
}
func (l *recLogger) Debug(msg string, kv ...any) { l.add("debug", msg, kv) }
func (l *recLogger) Info(msg string, kv ...any)  { l.add("info", msg, kv) }
func (l *recLogger) Warn(msg string, kv ...any)  { l.add("warn", msg, kv) }
func (l *recLogger) Error(msg string, kv ...any) { l.add("error", msg, kv) }

func TestPollTracedLogsRoundsAndTerminalExit(t *testing.T) {
	log := &recLogger{}
	statuses := []atomefin.Status{atomefin.StatusProcessing, atomefin.StatusProcessing, atomefin.StatusSuccess}
	calls := 0
	trace := payment.PollTrace[atomefin.Status]{
		Logger: log, Op: "/refund", RequestID: "req-1",
		Code: func(*atomefin.Status) (atomefin.Code, string) { return atomefin.CodeSuccess, "ok" },
	}
	_, err := payment.PollUntilTerminalOrRejected(context.Background(), payment.PollOptions{InitialDelay: time.Millisecond},
		trace, pollStatus, nil, func(context.Context) (*atomefin.Status, error) {
			s := statuses[calls]
			calls++
			return &s, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(log.lines) != 3 {
		t.Fatalf("got %d log lines; want 2 rounds + 1 exit: %+v", len(log.lines), log.lines)
	}
	for i, l := range log.lines[:2] {
		if l.level != "debug" || l.msg != "atomefin: poll round" || l.kv["round"] != i+1 ||
			l.kv["status"] != "PROCESSING" || l.kv["next_delay"] == nil {
			t.Errorf("round line %d = %+v", i, l)
		}
	}
	exit := log.lines[2]
	if exit.level != "info" || exit.kv["reason"] != "terminal" || exit.kv["status"] != "SUCCESS" ||
		exit.kv["round"] != 3 || exit.kv["request_id"] != "req-1" || exit.kv["op"] != "/refund" ||
		exit.kv["code"] != "SUCCESS" || exit.kv["message"] != "ok" {
		t.Errorf("exit line = %+v", exit)
	}
}

func TestPollTracedLogsStopReasons(t *testing.T) {
	processing := atomefin.StatusProcessing
	empty := atomefin.Status("")
	cases := []struct {
		name   string
		ctx    func() (context.Context, context.CancelFunc)
		opts   payment.PollOptions
		reject func(*atomefin.Status) (atomefin.Code, string, bool)
		once   func(context.Context) (*atomefin.Status, error)
		reason string
	}{
		{"max wait", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			payment.PollOptions{MaxWait: 20 * time.Millisecond, InitialDelay: time.Second}, nil,
			func(context.Context) (*atomefin.Status, error) { return &processing, nil }, "max_wait_exceeded"},
		{"parent deadline", func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(context.Background(), 20*time.Millisecond)
		}, payment.PollOptions{MaxWait: time.Second, InitialDelay: time.Second}, nil,
			func(context.Context) (*atomefin.Status, error) { return &processing, nil }, "parent_context_done"},
		{"request error", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			payment.PollOptions{}, nil,
			func(context.Context) (*atomefin.Status, error) { return nil, errors.New("boom") }, "request_error"},
		{"business rejection", func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
			payment.PollOptions{},
			func(s *atomefin.Status) (atomefin.Code, string, bool) {
				return atomefin.CodeRiskReject, "rejected", payment.IsSyncRejection(atomefin.CodeRiskReject, *s)
			},
			func(context.Context) (*atomefin.Status, error) { return &empty, nil }, "business_rejection"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := tc.ctx()
			defer cancel()
			log := &recLogger{}
			_, err := payment.PollUntilTerminalOrRejected(ctx, tc.opts,
				payment.PollTrace[atomefin.Status]{Logger: log, Op: "/auth", RequestID: "req-2"}, pollStatus, tc.reject, tc.once)
			if err == nil {
				t.Fatal("want error")
			}
			exit := log.lines[len(log.lines)-1]
			if exit.level != "warn" || exit.msg != "atomefin: poll stopped" || exit.kv["reason"] != tc.reason ||
				exit.kv["request_id"] != "req-2" {
				t.Errorf("exit line = %+v; want warn reason=%s", exit, tc.reason)
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
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err=%v; want parent DeadlineExceeded", err)
	}
}

func TestPollCallerCancelIsWrapped(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("http handler returned")
	cancel(cause)
	_, err := payment.PollUntilTerminalOrRejected(ctx, payment.PollOptions{},
		payment.PollTrace[atomefin.Status]{Op: "/capture"}, pollStatus, nil,
		func(context.Context) (*atomefin.Status, error) {
			t.Fatal("once must not run on a cancelled ctx")
			return nil, nil
		})
	var te *atomefin.TransportError
	if !errors.As(err, &te) || te.Op != "poll" || te.URL != "/capture" {
		t.Fatalf("err=%v; want *TransportError{Op: poll, URL: /capture}", err)
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Errorf("err=%v; want to wrap context.Canceled and the cancel cause", err)
	}
	if te.Temporary() {
		t.Error("caller cancellation should not be temporary")
	}
	for _, want := range []string{"caller context done", "0 round(s)", "http handler returned"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err=%q; want it to mention %q", err, want)
		}
	}
}
