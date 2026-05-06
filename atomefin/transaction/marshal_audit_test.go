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

// Empty-page round-trip pin — empty `items` MUST survive as `[]`,
// not `null`. Codifies the paginated-list pattern from chunk #3.
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

func TestR10_Transaction_Amount(t *testing.T) {
	marshal.AssertAmountRoundtrip[transaction.Transaction](t, func(v int64) transaction.Transaction {
		return transaction.Transaction{
			TradeID:     "TRD-1",
			TradeType:   transaction.TradeTypeAuth,
			AuthOrderID: "AUTH-1",
			Currency:    "IDR",
			Amount:      v,
			TradeStatus: "SUCCESS",
			TradeTime:   1,
		}
	})
}

// ---------- R11 — fractional decode rejection ----------

func TestR11_RejectsFractionalAmount(t *testing.T) {
	body := []byte(`{"tradeId":"TRD-1","tradeType":"AUTH","authOrderId":"A","currency":"IDR","amount":1.5,"tradeStatus":"SUCCESS","tradeTime":1}`)
	marshal.AssertRejectsFractionalAmount[transaction.Transaction](t, body)
}

// ---------- R12 — encoded amounts are integer literals only ----------

func TestR12_Transaction_IntegerLiterals(t *testing.T) {
	in := transaction.Transaction{
		TradeID:     "TRD-1",
		TradeType:   transaction.TradeTypeAuth,
		AuthOrderID: "AUTH-1",
		Currency:    "IDR",
		Amount:      1500000,
		TradeStatus: "SUCCESS",
		TradeTime:   1746084600000,
	}
	marshal.AssertAmountKeysAreInteger[transaction.Transaction](t, in,
		"amount",
	)
}
