package credit_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/credit"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_CreditEndpoints drives every credit-package outbound
// method against the spec server. Account-ops endpoints
// (modify-application-info, close-account) are co-located here per
// the v0.2 design choice and tested alongside the lifecycle ops.
//
// v0.3 reactivated /credit-information and /credit-application —
// both route through Client.DoEncryptedSigned. The spec server
// validates the Encrypt header presence and skips body-shape
// checks (the body is AES-ECB ciphertext that the spec server
// can't decrypt; R-invariants on the plaintext shape live in
// `marshal_audit_test.go` and the e2e test in
// `encrypted_e2e_test.go`).
func TestSpec_CreditEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /credit-information",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).SubmitInformation(context.Background(), specSampleCreditInformationParam())
				return err
			},
		},
		{
			Op: "POST /credit-application",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).SubmitApplication(context.Background(), specSampleCreditApplicationParam())
				return err
			},
		},
		{
			Op: "GET /credit-result",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).QueryResult(context.Background(), "u-spec-1")
				return err
			},
		},
		{
			Op: "GET /credit-information-result",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).QueryInformationResult(context.Background(), "u-spec-1", "r-spec-1")
				return err
			},
		},
		{
			Op: "GET /query-balance-history",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).BalanceHistory(context.Background(), &credit.BalanceHistoryParams{
					ExternalReferenceUID: "u-spec-1",
					Type:                 credit.BalanceHistoryTypeOverpaidChange,
				})
				return err
			},
		},
		{
			Op: "POST /modify-application-info",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).ModifyApplicationInfo(context.Background(), &credit.CreditApplicationChangeParam{
					RequestID:            "r-spec-modify",
					ExternalReferenceUID: "u-spec-1",
					MobileNumber:         "+6281298000000",
				})
				return err
			},
		},
		{
			Op: "POST /close-account",
			Run: func(c *atomefin.Client) error {
				_, err := credit.New(c).CloseAccount(context.Background(), &credit.CloseAccountParam{
					RequestID:            "r-spec-close",
					ExternalReferenceUID: "u-spec-1",
				})
				return err
			},
		},
	})
}

// ---------- minimal request constructors ----------

func specSampleCreditInformationParam() *credit.CreditInformationParam {
	return &credit.CreditInformationParam{
		RequestID:            "r-spec-info",
		ExternalReferenceUID: "u-spec-1",
		MobileNumber:         "+6281298000000",
		Email:                "spec@example.com",
		Country:              credit.CountryIndonesia,
		ApplicationEssentialInfo: &credit.CreditInformationEssentialInfo{
			LivenessCheck: &credit.LivenessCheck{Result: "PASS", SnapshotPhoto: "base64-photo"},
			IndividualProfile: &credit.IndividualProfile{
				IDType:       "KTP",
				OCRResult:    &credit.OCRResult{FullName: "Spec User"},
				IDFrontPhoto: "base64-id-front-photo",
			},
			PlatformInformation: &credit.PlatformInformation{
				SceneType:           credit.SceneCheckoutPage,
				LatestGkycTimeStamp: "1620285931000",
				UserFlag:            credit.UserFlagGrab,
				SCDUserLevel:        credit.SCDUserLevel2,
				CreditProfile:       `{"modelScores":[]}`,
				DeviceInfo: &credit.DeviceInfo{
					Platform: "ANDROID",
					Device: &credit.Device{
						DeviceID:            "device-spec",
						GoogleAdvertisingID: "advertising-spec",
						IsRoot:              false,
						Build: &credit.DeviceBuild{
							Board: "board", Brand: "brand", Device: "device",
							Manufacturer: "manufacturer", Model: "model", Product: "product",
						},
					},
					WifiList:  []credit.WifiAP{{SSID: "wifi-spec"}},
					IPAddress: &credit.IPAddress{EthIP: "192.0.2.1", TrueIP: "198.51.100.1"},
				},
			},
		},
	}
}

func specSampleCreditApplicationParam() *credit.CreditApplicationParam {
	return &credit.CreditApplicationParam{
		RequestID:            "r-spec-app",
		ExternalReferenceUID: "u-spec-1",
		MobileNumber:         "+6281298000000",
		Email:                "spec@example.com",
		Country:              credit.CountryIndonesia,
		ExtendInfo: &credit.CreditApplicationExtendInfo{
			CreditInformationRequestID: "r-spec-info",
		},
	}
}
