package mock_test

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func samplePaymentSubOrder(amount atomefin.Amount) payment.SubOrder {
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

func sampleRequestExtendInfo() *payment.RequestExtendInfo {
	return &payment.RequestExtendInfo{OrderType: payment.OrderTypeGrabFood}
}
