package bill_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/bill"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_BillEndpoints drives every bill-package GET against the
// spec server.
//
// Field-name mismatches between the SDK's query encoding and the
// 2026-05-06 spec snapshot:
//   - /bills: SDK sends `startDate`/`endDate`; spec requires
//     `startMonth`/`endMonth`. Tracked for the v0.2.x rename pass.
//
// Skip-list reflects what the SDK does NOT yet emit; closing each
// item is a follow-up patch tracked against the partner Q-set.
func TestSpec_BillEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "GET /bills",
			Run: func(c *atomefin.Client) error {
				_, err := bill.New(c).Bills(context.Background(), &bill.BillsParams{
					ExternalReferenceUID: "u-spec-1",
					StartDate:            "2026-04",
					EndDate:              "2026-05",
				})
				return err
			},
			SkipRequired: []string{
				"startMonth", // SDK sends startDate; rename pending v0.2.x
				"endMonth",   // SDK sends endDate; rename pending v0.2.x
			},
		},
		{
			Op: "GET /billDetail",
			Run: func(c *atomefin.Client) error {
				_, err := bill.New(c).BillDetail(context.Background(), "B-spec-1")
				return err
			},
			SkipRequired: []string{
				"externalReferenceUid", // SDK BillDetail signature takes only billID; pending companion arg add v0.2.x
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
