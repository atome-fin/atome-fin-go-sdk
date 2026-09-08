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

func internalInfoEssential(fullName string) *CreditInformationEssentialInfo {
	return &CreditInformationEssentialInfo{
		LivenessCheck: &LivenessCheck{Result: "PASS", SnapshotPhoto: "base64-photo"},
		IndividualProfile: &IndividualProfile{
			IDType:       "KTP",
			OCRResult:    &OCRResult{FullName: fullName},
			IDFrontPhoto: "base64-id-front-photo",
		},
		PlatformInformation: &PlatformInformation{
			SceneType:           SceneCheckoutPage,
			LatestGkycTimeStamp: "1620285931000",
			UserFlag:            UserFlagGrab,
			SCDUserLevel:        SCDUserLevel2,
			CreditProfile:       `{"modelScores":[]}`,
			DeviceInfo:          internalValidDeviceInfo(),
		},
	}
}

func internalValidDeviceInfo() *DeviceInfo {
	return &DeviceInfo{
		Platform: "ANDROID",
		Device: &Device{
			DeviceID:            "device-1",
			GoogleAdvertisingID: "advertising-1",
			IsRoot:              false,
			Build: &DeviceBuild{
				Board: "board", Brand: "brand", Device: "device",
				Manufacturer: "manufacturer", Model: "model", Product: "product",
			},
		},
		WifiList:  []WifiAP{{SSID: "wifi-1"}},
		IPAddress: &IPAddress{EthIP: "192.0.2.1", TrueIP: "198.51.100.1"},
	}
}

func internalValidInformationParam() *CreditInformationParam {
	return &CreditInformationParam{
		RequestID:                "r-1",
		ExternalReferenceUID:     "u-1",
		MobileNumber:             "+6281298000000",
		Email:                    "u@example.com",
		Country:                  CountryIndonesia,
		ApplicationEssentialInfo: internalInfoEssential("Test User"),
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

func TestValidateCreditInformation_Internal(t *testing.T) {
	cases := []struct {
		name      string
		req       *CreditInformationParam
		wantField string
	}{
		{"long-requestId", &CreditInformationParam{
			RequestID:                strings.Repeat("a", 65),
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "requestId"},
		{"long-externalReferenceUid", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     strings.Repeat("u", 65),
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "externalReferenceUid"},
		{"missing-mobileNumber", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			Email:                    "e@x",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "mobileNumber"},
		{"missing-email", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "email"},
		{"long-email", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    strings.Repeat("a", 65),
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "email"},
		{"missing-country", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "country"},
		{"unsupported-country", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			Country:                  Country("US"),
			ApplicationEssentialInfo: internalInfoEssential("Test"),
		}, "country"},
		{"missing-applicationEssentialInfo", &CreditInformationParam{
			RequestID:            "r",
			ExternalReferenceUID: "u",
			MobileNumber:         "+6281298000000",
			Email:                "e@x",
			Country:              CountryIndonesia,
		}, "applicationEssentialInfo"},
		{"missing-platformInformation", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: &CreditInformationEssentialInfo{},
		}, "applicationEssentialInfo.platformInformation"},
		{"extendInfo-bad-language", &CreditInformationParam{
			RequestID:                "r",
			ExternalReferenceUID:     "u",
			MobileNumber:             "+6281298000000",
			Email:                    "e@x",
			Country:                  CountryIndonesia,
			ApplicationEssentialInfo: internalInfoEssential("Test"),
			ExtendInfo:               &CreditInformationExtendInfo{Language: Language("zh")},
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
