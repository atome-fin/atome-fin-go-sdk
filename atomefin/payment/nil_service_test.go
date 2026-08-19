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

// Architect post-v0.1 audit: payment.New(nil) returned nil,
// then any method call dereferenced s.c (nil) inside the auto-mint
// path before the validator could fire — panic. These tests pin the
// post-fix behaviour: every public *Service method on a nil service
// or a service with a nil Client returns a typed *ValidationError
// instead of panicking.

func mustValidationOnNilService(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("nil receiver: expected error, got nil")
	}
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if !strings.Contains(ve.Field, "service") {
		t.Errorf("err.Field = %q; want a 'service' field", ve.Field)
	}
}

// ---------- nil receiver (s == nil) ----------

func TestNilService_AuthDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Auth on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service // payment.New(nil) returns nil
	_, err := svc.Auth(context.Background(), &payment.AuthRequest{})
	mustValidationOnNilService(t, err)
}

func TestNilService_CaptureDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Capture on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service
	_, err := svc.Capture(context.Background(), &payment.CaptureRequest{})
	mustValidationOnNilService(t, err)
}

func TestNilService_RiplayDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Riplay on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service
	_, err := svc.Riplay(context.Background(), &payment.RiplayRequest{})
	mustValidationOnNilService(t, err)
}

func TestNilService_VoidAuthDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("VoidAuth on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service
	_, err := svc.VoidAuth(context.Background(), &payment.VoidAuthRequest{})
	mustValidationOnNilService(t, err)
}

func TestNilService_AuthPollUntilTerminalDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("AuthPollUntilTerminal on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service
	_, err := svc.AuthPollUntilTerminal(context.Background(), &payment.AuthRequest{}, payment.PollOptions{MaxWait: time.Second})
	mustValidationOnNilService(t, err)
}

func TestNilService_CapturePollUntilTerminalDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("CapturePollUntilTerminal on nil receiver panicked: %v", r)
		}
	}()
	var svc *payment.Service
	_, err := svc.CapturePollUntilTerminal(context.Background(), &payment.CaptureRequest{}, payment.PollOptions{MaxWait: time.Second})
	mustValidationOnNilService(t, err)
}

// ---------- payment.New(nil) round-trip ----------

func TestNew_NilClient_ReturnsNilThatRejectsCleanly(t *testing.T) {
	svc := payment.New(nil)
	if svc != nil {
		t.Fatal("payment.New(nil) must return nil")
	}
	// Reaching into a nil receiver MUST surface a typed error.
	_, err := svc.Auth(context.Background(), &payment.AuthRequest{})
	mustValidationOnNilService(t, err)
}

// ---------- Service.Client() on nil receiver ----------

func TestNilService_ClientAccessorIsSafe(t *testing.T) {
	var svc *payment.Service
	if got := svc.Client(); got != nil {
		t.Errorf("Service.Client() on nil receiver = %v, want nil", got)
	}
}
