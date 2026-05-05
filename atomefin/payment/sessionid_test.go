package payment_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

// Architect post-v0.1 audit: validator allowed empty
// Sessionid, so a partner missing the header would round-trip a
// signed body to production and get back a silent 400
// SESSION_NOT_FOUND. Validator now rejects empty client-side.

func TestAuth_Validate_RejectsEmptySessionid(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when sessionid is empty client-side")
	})))

	svc := payment.New(c)
	req := &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		// Sessionid intentionally empty — must fail validation BEFORE
		// the server is hit (pre-fix this slipped through).
	}
	_, err := svc.Auth(context.Background(), req)
	if err == nil {
		t.Fatal("expected validation error on empty sessionid")
	}
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if !strings.Contains(ve.Field, "sessionid") {
		t.Errorf("err.Field = %q; want sessionid", ve.Field)
	}
	if !strings.Contains(ve.Message, "required") {
		t.Errorf("err.Message = %q; want explanation that sessionid is required", ve.Message)
	}
}

// And the existing >64 path still works:
func TestAuth_Validate_RejectsLongSessionid_StillFires(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	svc := payment.New(c)
	req := &payment.AuthRequest{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		TotalAmount:          1,
		PeriodType:           1,
		SubOrders:            []payment.SubOrder{{SubOrderID: "s", Amount: 1, Quantity: 1}},
		Sessionid:            strings.Repeat("a", 65),
	}
	_, err := svc.Auth(context.Background(), req)
	mustValidationError(t, err, "sessionid")
}
