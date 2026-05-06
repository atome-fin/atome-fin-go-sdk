package transaction_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transaction"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_TransactionEndpoints drives every transaction-package
// GET against the spec server. v0.2.3 closes the rename +
// missing-arg gaps surfaced by milestone 2 of v0.2.2:
//   - /transactions: SDK now sends `transactionType` (was
//     `tradeType`) and `startDate` / `endDate` in yyyyMMdd format.
//   - /transactionDetail: TransactionDetail now takes
//     (requestID, externalReferenceUID, transactionType) — the
//     v0.2.0 signature took only `tradeID`, which the 2026-05-06
//     spec snapshot eliminated entirely.
//
// No SkipRequired entries needed.
func TestSpec_TransactionEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "GET /transactions",
			Run: func(c *atomefin.Client) error {
				_, err := transaction.New(c).Transactions(context.Background(), &transaction.TransactionsParams{
					ExternalReferenceUID: "u-spec-1",
					StartDate:            "20260401",
					EndDate:              "20260501",
					TransactionType:      transaction.TransactionTypePayment,
				})
				return err
			},
		},
		{
			Op: "GET /transactionDetail",
			Run: func(c *atomefin.Client) error {
				_, err := transaction.New(c).TransactionDetail(context.Background(),
					"r-spec-1", "u-spec-1", transaction.TransactionTypePayment)
				return err
			},
		},
	})
}
