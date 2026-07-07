package credit

import (
	"errors"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
)

// validation_internal_test.go exercises the validators for
// /credit-information and /credit-application directly, white-box.
// SubmitInformation and SubmitApplication are blocked in v0.2.x
// (see credit.go) so the validators are no longer called from the
// public path; we still want regression coverage on them so that
// when v0.3 re-enables the network path, the validation rules are
// guaranteed-good rather than rotting silently.

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

func internalValidInformationParam() *CreditInformationParam {
	return &CreditInformationParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		MobileNumber:         "+6281298000000",
		Email:                "u@example.com",
		Country:              CountryIndonesia,
		ApplicationEssentialInfo: &CreditInformationEssentialInfo{
			OCRResult: &CreditInformationOCRResult{FullName: "Test User"},
		},
	}
}

func internalValidApplicationParam() *CreditApplicationParam {
	return &CreditApplicationParam{
		RequestID:            "r-1",
		ExternalReferenceUID: "u-1",
		MobileNumber:         "+6281298000000",
		Email:                "u@example.com",
		Country:              CountryIndonesia,
		ApplicationEssentialInfo: &ApplicationEssentialInfo{
			LivenessCheck: &LivenessCheck{
				Result:        "PASS",
				SnapshotPhoto: "base64-photo",
			},
			IndividualProfile:   &IndividualProfile{IDType: "KTP"},
			PlatformInformation: &PlatformInformation{},
		},
		ExtendInfo: &CreditApplicationExtendInfo{
			CreditInformationRequestID: "info-1",
		},
	}
}

func TestValidateCreditInformation_Internal(t *testing.T) {
	cases := []struct {
		name      string
		req       *CreditInformationParam
		wantField string
	}{
		{"long-requestId", &CreditInformationParam{
			RequestID:            strings.Repeat("a", 65),
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "requestId"},
		{"long-externalReferenceUid", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: strings.Repeat("u", 65),
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "externalReferenceUid"},
		{"missing-mobileNumber", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			Email:                "e@x",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "mobileNumber"},
		{"missing-email", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "email"},
		{"long-email", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                strings.Repeat("a", 65),
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "email"},
		{"missing-country", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "country"},
		{"unsupported-country", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              Country("US"),
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
		}, "country"},
		{"missing-applicationEssentialInfo", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
		}, "applicationEssentialInfo"},
		{"missing-ocr-fullName", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{},
			},
		}, "applicationEssentialInfo.ocrResult.fullName"},
		{"extendInfo-bad-language", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{
				OCRResult: &CreditInformationOCRResult{FullName: "Test"},
			},
			ExtendInfo: &CreditInformationExtendInfo{Language: Language("zh")},
		}, "extendInfo.language"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCreditInformation(tc.req)
			mustValidateError(t, err, tc.wantField)
		})
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
		{"missing-essentialInfo", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ApplicationEssentialInfo = nil
			return req
		}, "applicationEssentialInfo"},
		{"missing-livenessCheck", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ApplicationEssentialInfo.LivenessCheck = nil
			return req
		}, "applicationEssentialInfo.livenessCheck"},
		{"missing-liveness-result", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ApplicationEssentialInfo.LivenessCheck.Result = ""
			return req
		}, "applicationEssentialInfo.livenessCheck.result"},
		{"missing-individualProfile", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ApplicationEssentialInfo.IndividualProfile = nil
			return req
		}, "individualProfile"},
		{"missing-platformInformation", func(req *CreditApplicationParam) *CreditApplicationParam {
			req.ApplicationEssentialInfo.PlatformInformation = nil
			return req
		}, "platformInformation"},
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
	if err := validateCreditInformation(internalValidInformationParam()); err != nil {
		t.Errorf("valid base request: validateCreditInformation = %v; want nil", err)
	}
}
