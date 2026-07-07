package mock_demo_test

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func demoSubOrder(amount atomefin.Amount) payment.SubOrder {
	return payment.SubOrder{
		SubOrderID:      "so-1",
		SkuID:           "sku-1",
		CategoryID:      "cat-1",
		CategoryOneName: "Food",
		MerchantID:      "merchant-1",
		Amount:          amount,
		Quantity:        1,
	}
}
