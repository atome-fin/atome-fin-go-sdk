package virtualaccount_test

import (
	"context"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/virtualaccount"
	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

func TestSpec_VirtualAccountEndpoints(t *testing.T) {
	specserver.RunCases(t, []specserver.Case{
		{
			Op: "POST /va/getList",
			Run: func(c *atomefin.Client) error {
				_, err := virtualaccount.New(c).ListBanks(context.Background())
				return err
			},
		},
		{
			Op: "POST /va/vaCodeByBank",
			Run: func(c *atomefin.Client) error {
				_, err := virtualaccount.New(c).GetOrCreate(context.Background(), &virtualaccount.VirtualAccountRequest{
					ExternalReferenceUID: "u-spec-1",
					BankCode:             virtualaccount.BankBCA,
				})
				return err
			},
		},
	})
}
