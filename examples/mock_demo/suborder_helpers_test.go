package mock_demo_test

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func demoSubOrder(amount atomefin.Amount) payment.SubOrder {
	return payment.SubOrder{
		SubOrderID:       "so-1",
		MerchantID:       "merchant-1",
		MerchantName:     "KFC Sudirman",
		MerchantCategory: "FAST_FOOD",
		Amount:           amount,
	}
}

func demoExtendInfo(amount atomefin.Amount) *payment.RequestExtendInfo {
	return &payment.RequestExtendInfo{
		OrderType:     payment.OrderTypeGrabFood,
		CreditProfile: payment.CreditProfile(`{"score":720}`),
		MainOrderExtendInfos: []payment.MainOrderExtendInfo{{
			MerchantID: "merchant-1",
			SkuInfos:   []payment.SkuInfo{{SkuID: "sku-1", Amount: amount}},
		}},
	}
}
