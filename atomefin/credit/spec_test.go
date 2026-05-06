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
		EventType:            credit.EventTypeNewApplication,
		Email:                "spec@example.com",
		Country:              credit.CountryIndonesia,
		ExtendInfo: &credit.CreditInformationExtendInfo{
			Language: credit.LanguageEnglish,
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
		ApplicationEssentialInfo: &credit.ApplicationEssentialInfo{
			IndividualProfile:   &credit.IndividualProfile{IDType: "KTP"},
			PlatformInformation: &credit.PlatformInformation{},
		},
		ExtendInfo: &credit.CreditApplicationExtendInfo{
			CreditInformationRequestID: "r-spec-info",
		},
	}
}
