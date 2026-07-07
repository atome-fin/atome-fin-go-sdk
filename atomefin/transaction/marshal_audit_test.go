// Wires the qa/marshal harness against every public transaction
// struct.
package transaction_test

import (
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/transaction"
	"github.com/atome-fin/atome-fin-go-sdk/qa/marshal"
)

const fixtureRoot = "../../qa/testdata/"

// ---------- /transactions + /transactionDetail ----------

func TestTransactionsResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[transaction.TransactionsResponse](t, fixtureRoot+"transactions_response.json")
}

// Empty response round-trip pin.
func TestTransactionsResponse_Roundtrip_Empty(t *testing.T) {
	marshal.GoldenRoundTrip[transaction.TransactionsResponse](t, fixtureRoot+"transactions_response_empty.json")
}

func TestTransactionDetailResponse_Roundtrip(t *testing.T) {
	marshal.GoldenRoundTrip[transaction.TransactionDetailResponse](t, fixtureRoot+"transactionDetail_response.json")
}

func TestTransactionDetailResponse_Roundtrip_Minimal(t *testing.T) {
	marshal.GoldenRoundTrip[transaction.TransactionDetailResponse](t, fixtureRoot+"transactionDetail_response_minimal.json")
}

func TestTransactionDetailResponse_Roundtrip_Failed(t *testing.T) {
	marshal.GoldenRoundTrip[transaction.TransactionDetailResponse](t, fixtureRoot+"transactionDetail_response_failed.json")
}

// ---------- R10 — full int64 amount round-trip ----------

func TestR10_TradeRefundInfo_RefundAmount(t *testing.T) {
	marshal.AssertAmountRoundtrip[transaction.TradeRefundInfo](t, func(v int64) transaction.TradeRefundInfo {
		return transaction.TradeRefundInfo{
			RefundRequestID:  "rfd-1",
			CaptureRequestID: "cap-1",
			RefundAmount:     v,
			CreateTime:       1,
			SubOrders:        []transaction.TradeRefundSubOrder{{SubOrderID: "so-1", RefundStatus: "FULL_REFUND", PrincipalAmount: v}},
		}
	})
}

// ---------- R11 — fractional decode rejection ----------

func TestR11_RejectsFractionalAmount(t *testing.T) {
	body := []byte(`{"refundRequestId":"rfd-1","captureRequestId":"cap-1","refundAmount":1.5,"createTime":1,"subOrders":[]}`)
	marshal.AssertRejectsFractionalAmount[transaction.TradeRefundInfo](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_TradePaymentInfoDetail_IntegerLiterals(t *testing.T) {
	in := transaction.TradePaymentInfoDetail{
		CaptureRequestID: "cap-1",
		OrderID:          "ord-1",
		TotalTenor:       3,
		CreateTime:       1746084600000,
		PrincipalAmount:  1500000,
		InterestAmount:   50000,
		DiscountAmount:   10000,
	}
	marshal.AssertAmountKeysAreInteger[transaction.TradePaymentInfoDetail](t, in,
		"principalAmount", "interestAmount", "discountAmount",
	)
}
