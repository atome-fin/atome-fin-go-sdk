package repayment_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
)

// ---------- Repayment validation table ----------

func TestRepayment_Validate_TableDriven(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := repayment.New(c)

	cases := []struct {
		name      string
		req       *repayment.RepaymentParam
		wantField string
	}{
		{
			name:      "nil-request",
			req:       nil,
			wantField: "request",
		},
		{
			name: "long-requestId",
			req: &repayment.RepaymentParam{
				RequestID:            strings.Repeat("a", 65),
				ExternalReferenceUID: "u",
				RepaymentAmount:      1,
				RepaymentApplyTime:   1746662400000,
			},
			wantField: "requestId",
		},
		{
			name: "missing-externalReferenceUid",
			req: &repayment.RepaymentParam{
				RequestID:          "r",
				RepaymentAmount:    1,
				RepaymentApplyTime: 1746662400000,
			},
			wantField: "externalReferenceUid",
		},
		{
			name: "long-externalReferenceUid",
			req: &repayment.RepaymentParam{
				RequestID:            "r",
				ExternalReferenceUID: strings.Repeat("u", 65),
				RepaymentAmount:      1,
				RepaymentApplyTime:   1746662400000,
			},
			wantField: "externalReferenceUid",
		},
		{
			name: "zero-repaymentAmount",
			req: &repayment.RepaymentParam{
				RequestID:            "r",
				ExternalReferenceUID: "u",
				RepaymentAmount:      0,
				RepaymentApplyTime:   1746662400000,
			},
			wantField: "repaymentAmount",
		},
		{
			name: "negative-repaymentAmount",
			req: &repayment.RepaymentParam{
				RequestID:            "r",
				ExternalReferenceUID: "u",
				RepaymentAmount:      -1,
				RepaymentApplyTime:   1746662400000,
			},
			wantField: "repaymentAmount",
		},
		{
			name: "zero-repaymentApplyTime",
			req: &repayment.RepaymentParam{
				RequestID:            "r",
				ExternalReferenceUID: "u",
				RepaymentAmount:      1,
				RepaymentApplyTime:   0,
			},
			wantField: "repaymentApplyTime",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Repayment(context.Background(), tc.req)
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- QueryRepayment validation ----------

func TestQueryRepayment_Validate(t *testing.T) {
	c := mustClient(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server must NOT be reached when validation rejects")
	})))
	svc := repayment.New(c)

	cases := []struct {
		name      string
		reqID     string
		extUID    string
		wantField string
	}{
		{"empty-requestId", "", "u-1", "requestId"},
		{"long-requestId", strings.Repeat("a", 65), "u-1", "requestId"},
		{"empty-externalReferenceUid", "r-1", "", "externalReferenceUid"},
		{"long-externalReferenceUid", "r-1", strings.Repeat("u", 65), "externalReferenceUid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.QueryRepayment(context.Background(), tc.reqID, tc.extUID)
			if err == nil {
				t.Fatalf("QueryRepayment(%q,%q) must reject", tc.reqID, tc.extUID)
			}
			mustValidationError(t, err, tc.wantField)
		})
	}
}

// ---------- RepaymentEvent / RepaymentStatus enums ----------

func TestRepaymentEvent_IsValid(t *testing.T) {
	if !repayment.RepaymentEventNormal.IsValid() {
		t.Errorf("%q.IsValid() = false; want true", repayment.RepaymentEventNormal)
	}
	for _, e := range []repayment.RepaymentEvent{
		repayment.RepaymentEventAtomeRepayment,
		repayment.RepaymentEventOverpaidRepayment,
		repayment.RepaymentEvent("UNKNOWN"),
		repayment.RepaymentEvent(""),
	} {
		if e.IsValid() {
			t.Errorf("%q.IsValid() = true; want false", e)
		}
	}
	// String() returns wire literal verbatim.
	if got := repayment.RepaymentEventNormal.String(); got != "NORMAL" {
		t.Errorf("Normal.String() = %q, want %q", got, "NORMAL")
	}
}

func TestRepaymentStatus_IsValid(t *testing.T) {
	for _, s := range []repayment.RepaymentStatus{
		repayment.StatusRepaid,
		repayment.StatusUnpaid,
		repayment.StatusPartialRepaid,
	} {
		if !s.IsValid() {
			t.Errorf("%q.IsValid() = false; want true", s)
		}
	}
	if repayment.RepaymentStatus("UNKNOWN").IsValid() {
		t.Error("UNKNOWN.IsValid() must be false")
	}
	if got := repayment.StatusPartialRepaid.String(); got != "PARTIAL_REPAID" {
		t.Errorf("PartialRepaid.String() = %q, want %q", got, "PARTIAL_REPAID")
	}
}
