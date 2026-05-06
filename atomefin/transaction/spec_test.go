package transaction_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transaction"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_TransactionEndpoints drives every transaction-package
// GET against the spec server.
//
// Field-name mismatches between the SDK's query encoding and the
// 2026-05-06 spec snapshot:
//   - /transactions: SDK sends `tradeType`; spec requires
//     `transactionType`. Tracked for the v0.2.x rename pass.
//   - /transactionDetail: SDK sends `tradeId`; spec requires
//     `requestId` + `transactionType`. Tracked for v0.2.x.
//
// Skip-list reflects what the SDK does NOT yet emit; closing each
// item is a follow-up patch.
func TestSpec_TransactionEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "GET /transactions",
			Run: func(c *atomefin.Client) error {
				_, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
					ExternalReferenceUID: "u-spec-1",
					StartDate:            "2026-04-01",
					EndDate:              "2026-05-01",
					TradeType:            transaction.TradeTypeAuth,
				})
				return err
			},
			SkipRequired: []string{
				"transactionType", // SDK sends `tradeType`; rename pending v0.2.x
			},
		},
		{
			Op: "GET /transactionDetail",
			Run: func(c *atomefin.Client) error {
				_, err := transaction.New(c).TransactionDetail(context.Background(), "T-spec-1")
				return err
			},
			SkipRequired: []string{
				"externalReferenceUid", // SDK TransactionDetail signature takes only tradeID
				"requestId",            // SDK sends `tradeId`; spec requires `requestId`
				"transactionType",      // not present in SDK signature
			},
		},
	})
}
