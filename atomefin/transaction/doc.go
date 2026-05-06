// Package transaction implements the GET-only transaction-query
// surface added in v0.2 chunk #4:
//
//   - GET /transactions       partner → atome-fin (paginated list)
//   - GET /transactionDetail  partner → atome-fin (single, tradeId param)
//
// Transactions are read-only — there is no inbound webhook (atome-fin
// does not push transaction-level mutations) and no signed body to
// marshal. Every call goes through Client.DoSignedGET, which signs
// the alphabetically-sorted canonical query string and sets
// req.URL.RawQuery to the SAME bytes (R13: wire ≡ signing canonical).
//
// # Constructor pattern (mirrors payment / refund / bill)
//
// Like the other Service families, transaction avoids the
// import-cycle that a typed `client.Transaction` field would create.
// Construct via transaction.New(c) where c is an *atomefin.Client:
//
//	c, err := atomefin.New(...)
//	tx := transaction.New(c)
//	page, err := tx.Transactions(ctx, &transaction.TransactionsParams{
//	    ExternalReferenceUID: "user-42",
//	})
//
// # Pagination
//
// `Transactions` returns one page at a time. Defaults are
// PageNumber=1, PageSize=20. Convenience iterator
// `TransactionsAll` walks every page until short-page or Total.
//
// # Selective sub-types
//
// Per architect's chunk-scoping note, this package pulls ONLY the
// types reachable from /transactions and /transactionDetail. Other
// trade-shape types listed in SPEC_DELTA's domain-spread roster
// (repayment-history rows, ledger entries, etc.) live in their own
// future packages — the bill and repayment chunks own their own
// sub-types so each Service stays focused.
package transaction
