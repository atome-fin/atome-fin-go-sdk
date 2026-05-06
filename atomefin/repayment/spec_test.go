package repayment_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/repayment"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// TestSpec_RepaymentEndpoints drives every repayment-package
// outbound method against the spec server.
func TestSpec_RepaymentEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /repayment-request",
			Run: func(c *atomefin.Client) error {
				_, err := repayment.New(c).Repayment(context.Background(), specSampleRepaymentParam())
				return err
			},
		},
		{
			Op: "GET /repayment-result",
			Run: func(c *atomefin.Client) error {
				_, err := repayment.New(c).QueryRepayment(context.Background(), "r-spec-1", "u-spec-1")
				return err
			},
		},
	})
}

func specSampleRepaymentParam() *repayment.RepaymentParam {
	return &repayment.RepaymentParam{
		RequestID:            "r-spec-repayment",
		ExternalReferenceUID: "u-spec-1",
		RepaymentAmount:      atomefin.Amount(2000000),
		RepaymentApplyTime:   1714972800000,
	}
}
