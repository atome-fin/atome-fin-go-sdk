// Wires the qa/marshal harness against every public bill struct.
package bill_test

import (
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/bill"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /bills + /billDetail + /billUnpaid ----------

func TestBillsResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[bill.BillsResponse](t, fixtureRoot+"bills_response.json")
}

func TestBillsResponse_Roundtrip_Empty(t *testing.T) {
	marshal.GoldenRoundTrip[bill.BillsResponse](t, fixtureRoot+"bills_response_empty.json")
}

func TestBillDetailResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[bill.BillDetailResponse](t, fixtureRoot+"billDetail_response.json")
}

func TestBillDetailResponse_Roundtrip_NoDiscounts(t *testing.T) {
	marshal.GoldenRoundTrip[bill.BillDetailResponse](t, fixtureRoot+"billDetail_response_no_discounts.json")
}

func TestBillsUnpaidResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[bill.BillsResponse](t, fixtureRoot+"billsUnpaid_response.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_Bill_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.Bill](t, func(v int64) bill.Bill {
		return bill.Bill{
			BillID:      "202605",
			Currency:    "IDR",
			TotalAmount: v,
		}
	})
}

func TestR10_BillOrder_Amount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.BillOrder](t, func(v int64) bill.BillOrder {
		return bill.BillOrder{AuthOrderID: "AUTH-1", Amount: v}
	})
}

func TestR10_Discount_Amount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.Discount](t, func(v int64) bill.Discount {
		return bill.Discount{DiscountID: "D-1", Amount: v}
	})
}

// ---------- R11 — fractional decode rejection ----------

func TestR11_RejectsFractionalTotalAmount(t *testing.T) {
	body := []byte(`{"billId":"202605","currency":"IDR","totalAmount":1.5}`)
	marshal.AssertRejectsFractionalAmount[bill.Bill](t, body)
}

func TestR11_RejectsFractionalDiscountAmount(t *testing.T) {
	body := []byte(`{"discountId":"D-1","amount":1.5}`)
	marshal.AssertRejectsFractionalAmount[bill.Discount](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_Bill_IntegerLiterals(t *testing.T) {
	in := bill.Bill{
		BillID:        "202605",
		Currency:      "IDR",
		TotalAmount:   1500000,
		PaidAmount:    1000000,
		UnpaidAmount:  500000,
		OverdueStatus: bill.OverdueStatusOnTime,
	}
	marshal.AssertAmountKeysAreInteger[bill.Bill](t, in,
		"totalAmount", "paidAmount", "unpaidAmount",
	)
}

func TestR12_BillDiscounts_IntegerLiterals(t *testing.T) {
	in := bill.BillDiscounts{
		TotalDiscount: 50000,
		Items: []bill.Discount{
			{DiscountID: "D-1", Amount: 30000},
			{DiscountID: "D-2", Amount: 20000},
		},
	}
	marshal.AssertAmountKeysAreInteger[bill.BillDiscounts](t, in,
		"totalDiscount", "amount",
	)
}
