package credit

import (
	"errors"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// validation_internal_test.go exercises the remaining request
// validators (POST /credit-application and friends) white-box.
// POST /credit-information intentionally ships without client-side
// validation — the gateway owns those checks.

func mustValidateError(t *testing.T, err error, wantField string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected *ValidationError, got nil")
	}
	var ve *atomefin.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v; want *ValidationError", err)
	}
	if !strings.Contains(ve.Field, wantField) {
		t.Errorf("err.Field = %q; want substring %q", ve.Field, wantField)
	}
}

func internalValidApplicationParam() *CreditApplicationParam {
	return &CreditApplicationParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		MobileNumber:         "+6281298000000",
		Email:                "u@example.com",
		Country:              CountryIndonesia,
		ExtendInfo: &CreditApplicationExtendInfo{
			CreditInformationRequestID: "info-1",
		},
	}
}

func TestValidateCreditApplication_Internal(t *testing.T) {
	base := internalValidApplicationParam

	cases := []struct {
		name      string
		mutate    func(*CreditApplicationParam) *CreditApplicationParam
		wantField string
	}{
		{"long-requestId", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.RequestID = strings.Repeat("a", 65)
			return req
		}, "requestId"},
		{"missing-mobile", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.MobileNumber = ""
			return req
		}, "mobileNumber"},
		{"missing-email", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.Email = ""
			return req
		}, "email"},
		{"long-email", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.Email = strings.Repeat("a", 65)
			return req
		}, "email"},
		{"unsupported-country", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.Country = "PH"
			return req
		}, "country"},
		{"missing-extendInfo", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ExtendInfo = nil
			return req
		}, "extendInfo"},
		{"missing-creditInformationRequestId", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ExtendInfo.CreditInformationRequestID = ""
			return req
		}, "creditInformationRequestId"},
		{"long-creditInformationRequestId", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ExtendInfo.CreditInformationRequestID = strings.Repeat("a", 65)
			return req
		}, "creditInformationRequestId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreditApplication(tc.mutate(base()))
			mustValidateError(t, err, tc.wantField)
		})
	}

	if err := validateCreditApplication(base()); err != nil {
		t.Errorf("valid base request: validateCreditApplication = %v; want nil", err)
	}
}
