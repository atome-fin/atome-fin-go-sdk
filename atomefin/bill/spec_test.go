package bill_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/bill"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_BillEndpoints drives every bill-package GET against the
// spec server. v0.2.3 closes the rename + missing-arg gaps surfaced
// by milestone 2 of v0.2.2: `startDate`/`endDate` →
// `startMonth`/`endMonth` on /bills, and BillDetail now takes both
// `billID` and `externalReferenceUID`. No SkipRequired entries
// needed.
func TestSpec_BillEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "GET /bills",
			Run: func(c *atomefin.Client) error {
				_, err := bill.New(c).Bills(context.Background(), &bill.BillsParams{
					ExternalReferenceUID: "u-spec-1",
					StartMonth:           "202604",
					EndMonth:             "202605",
				})
				return err
			},
		},
		{
			Op: "GET /billDetail",
			Run: func(c *atomefin.Client) error {
				_, err := bill.New(c).BillDetail(context.Background(), "B-spec-1", "u-spec-1")
				return err
			},
		},
		{
			Op: "GET /billUnpaid",
			Run: func(c *atomefin.Client) error {
				_, err := bill.New(c).BillsUnpaid(context.Background(), &bill.BillsUnpaidParams{
					ExternalReferenceUID: "u-spec-1",
				})
				return err
			},
		},
	})
}
