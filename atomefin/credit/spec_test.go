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
// /credit-information and /credit-application are NOT covered here
// — both are blocked locally in v0.2.x pending the v0.3
// hybrid-encryption envelope (see TestSubmitInformation_BlockedUntilV0_3
// in service_test.go). They will rejoin this case table when v0.3
// re-enables the network path.
func TestSpec_CreditEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
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

// (No constructors needed — POST /credit-information and POST
// /credit-application are blocked in v0.2.x; their case-rows above
// have been removed.)
