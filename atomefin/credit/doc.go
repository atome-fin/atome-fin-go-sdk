// Package credit implements the outbound credit-lifecycle endpoints
// plus the inbound credit-* callback type-aliases used by
// atomefin/callback. v0.2 chunk #6 — the largest sub-package by
// schema surface; the spec exposes seven outbound surfaces and two
// inbound callbacks:
//
//	POST /credit-information           partner → atome-fin (signed body)
//	POST /credit-application           partner → atome-fin (signed body)
//	GET  /credit-result                partner → atome-fin (signed query)
//	GET  /credit-information-result    partner → atome-fin (signed query)
//	GET  /query-balance-history        partner → atome-fin (signed query)
//	POST /modify-application-info      partner → atome-fin (signed body)
//	POST /close-account                partner → atome-fin (signed body)
//	POST <creditInformationNotifyUrl>  atome-fin → partner (terminal-state webhook)
//	POST <creditApplicationNotifyUrl>  atome-fin → partner (terminal-state webhook)
//
// # Constructor pattern (mirrors payment, refund, bill, transaction)
//
// Construct the Service via credit.New(c) where c is an
// *atomefin.Client:
//
//	c, err := atomefin.New(...)
//	cr := credit.New(c)
//	res, err := cr.SubmitInformation(ctx, &credit.CreditInformationParam{...})
//
// The credit lifecycle is two-step: partners first POST
// /credit-information to register a user (returns a `requestId` and a
// jumpUrl into Atome's KYC web flow), then POST /credit-application
// linking the prior requestId via extendInfo.creditInformationRequestId
// to submit the full KYC payload. Async completion arrives via the
// two callbacks; partners that prefer polling can call the matching
// GET endpoints.
//
// # Money policy
//
// Per the project-wide money policy (atomefin/money.go) every amount
// field is bare int64; required money fields never carry ,omitempty
// so legitimate zero deltas serialise. Currency is the named type
// from atomefin (currently locked to "IDR" until the spec broadens —
// Q10 RESOLVED 2026-05-06).
//
// # Q-list (partner-pending)
//
// The credit endpoints inherit the v0.1 partner-pending Q-list and
// add no new SDK-side opens beyond Q11 (date-range TZ for
// /query-balance-history's start/count, which the SDK passes through
// as integer indexes — server is canonical) and Q26 (multi-currency
// timeline, which is a non-issue while only IDR is in the enum).
package credit
