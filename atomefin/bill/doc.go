// Package bill implements the GET-only bill-query surface added in
// v0.2 chunk #3:
//
//   - GET /bills      partner → atome-fin (paginated list)
//   - GET /billDetail partner → atome-fin (single bill, billId param)
//   - GET /billUnpaid partner → atome-fin (unpaid filter view)
//
// Bill is read-only — there is no inbound webhook (atome-fin doesn't
// push bill mutations) and no signed body to marshal. Every call
// goes through Client.DoSignedGET, which signs the alphabetically-
// sorted canonical query string and sets req.URL.RawQuery to the
// SAME bytes (R13: wire ≡ signing canonical).
//
// # Constructor pattern (mirrors payment / refund)
//
// Like the other Service families, bill avoids the import-cycle that
// a typed `client.Bill` field would create. Construct via
// bill.New(c) where c is an *atomefin.Client:
//
//	c, err := atomefin.New(...)
//	bills := bill.New(c)
//	page, err := bills.Bills(ctx, &bill.BillsParams{
//	    ExternalReferenceUID: "user-42",
//	    PageNumber:           1,
//	    PageSize:             20,
//	})
//
// One extra constructor call vs. method-chaining; partners that
// don't need bill don't pay for it (tree-shake-friendly).
//
// # Pagination
//
// `Bills` and `BillsUnpaid` return one page at a time. The default
// PageNumber is 1 and the default PageSize is 20 (zero values
// trigger the defaults so callers can pass &BillsParams{} for the
// first page of 20 items).
//
// For partners that want to walk every bill, BillsAll wraps the
// page loop: it calls Bills with successively-incremented
// PageNumber until the server returns fewer rows than PageSize OR
// the cumulative Total is reached. Pure convenience — partners
// who need explicit page control should call Bills directly.
package bill
