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
	marshal.GoldenRoundTrip[bill.BillUnpaidResponse](t, fixtureRoot+"billsUnpaid_response.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_Bill_TotalAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.Bill](t, func(v int64) bill.Bill {
		return bill.Bill{
			BillID:            "202605",
			BillMonth:         "202605",
			BillTotalAmount:   v,
			RepaidAmount:      0,
			OutstandingAmount: v,
			PrincipalAmount:   v,
			InterestAmount:    0,
			DueDate:           "20260615",
			RepaymentStatus:   "UNPAID",
			OverdueStatus:     bill.OverdueStatusNotOverdue,
		}
	})
}

func TestR10_BillMainOrder_Amount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.BillMainOrder](t, func(v int64) bill.BillMainOrder {
		return bill.BillMainOrder{
			OrderID: "ORD-1", RequestID: "REQ-1", CreateTime: 1746489600000,
			PeriodType: "3", CurrentPeriod: 1,
			TotalAmount: v, PrincipalAmount: v, InterestAmount: 0,
			RepaidAmount: 0, OutstandingAmount: v, DueDate: "20260615",
			Status: bill.BillStatusBilled, RepaymentStatus: "UNPAID", OverdueStatus: bill.OverdueStatusNotOverdue,
		}
	})
}

func TestR10_BillDiscounts_Amount(t *testing.T) {
	marshal.AssertAmountRoundtrip[bill.BillDiscounts](t, func(v int64) bill.BillDiscounts {
		return bill.BillDiscounts{DiscountAmount: v}
	})
}

// ---------- R11 — fractional decode rejection ----------

func TestR11_RejectsFractionalTotalAmount(t *testing.T) {
	body := []byte(`{"billId":"202605","billMonth":"202605","billTotalAmount":1.5}`)
	marshal.AssertRejectsFractionalAmount[bill.Bill](t, body)
}

func TestR11_RejectsFractionalDiscountAmount(t *testing.T) {
	body := []byte(`{"discountAmount":1.5}`)
	marshal.AssertRejectsFractionalAmount[bill.BillDiscounts](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_Bill_IntegerLiterals(t *testing.T) {
	in := bill.Bill{
		BillID:            "202605",
		BillMonth:         "202605",
		BillTotalAmount:   1500000,
		RepaidAmount:      1000000,
		OutstandingAmount: 500000,
		PrincipalAmount:   1450000,
		InterestAmount:    50000,
		DueDate:           "20260615",
		RepaymentStatus:   "PARTIAL_REPAID",
		OverdueStatus:     bill.OverdueStatusNotOverdue,
	}
	marshal.AssertAmountKeysAreInteger[bill.Bill](t, in,
		"billTotalAmount", "repaidAmount", "outstandingAmount", "principalAmount", "interestAmount",
	)
}

func TestR12_BillDiscounts_IntegerLiterals(t *testing.T) {
	in := bill.BillDiscounts{
		DiscountAmount:               50000,
		InterestAmountAfterDiscount:  25000,
		BillTotalAmountAfterDiscount: 1450000,
		RepaidAmountExcludeDiscount:  950000,
	}
	marshal.AssertAmountKeysAreInteger[bill.BillDiscounts](t, in,
		"discountAmount", "interestAmountAfterDiscount", "billTotalAmountAfterDiscount", "repaidAmountExcludeDiscount",
	)
}
