package mock_test

import (
	"github.com/atome-fin/atome-fin-go-sdk/atomefin"
	"github.com/atome-fin/atome-fin-go-sdk/atomefin/payment"
)

func samplePaymentSubOrder(amount atomefin.Amount) payment.SubOrder {
	return payment.SubOrder{
		SubOrderID:       "so-1",
		MerchantID:       "merchant-1",
		MerchantName:     "KFC Sudirman",
		MerchantCategory: "FAST_FOOD",
		Amount:           amount,
	}
}

func sampleRequestExtendInfo() *payment.RequestExtendInfo {
	return &payment.RequestExtendInfo{
		OrderType:     payment.OrderTypeGrabFood,
		CreditProfile: payment.CreditProfile(`{"score":720}`),
		DeviceInfo: &payment.DeviceInfo{
			Platform: payment.PlatformAndroid,
			Device: &payment.DeviceProfile{
				DeviceID:            "dev-1",
				GoogleAdvertisingID: "gaid-1",
				UTDID:               "utdid-1",
				Build: &payment.BuildInfo{
					Board: "sdm845", Brand: "xiaomi", Device: "dipper",
					Manufacturer: "Xiaomi", Model: "MI 8", Product: "dipper",
				},
			},
			WifiList:  []payment.WifiAP{{SSID: "ssid"}},
			IPAddress: &payment.IPAddress{EthIP: "10.0.0.2", TrueIP: "203.0.113.7"},
		},
		Address: &payment.Address{
			ShippingAddress: &payment.ShippingAddress{City: "Jakarta", Address1: "Jl. Sudirman No. 1"},
			ShippingName:    "Budi Santoso",
			ShippingPhoneNo: "+6281234567890",
		},
		MainOrderExtendInfos: []payment.MainOrderExtendInfo{
			{MerchantID: "merchant-1", SkuInfos: []payment.SkuInfo{{SkuID: "sku-1", Amount: 1500000}}},
		},
	}
}
