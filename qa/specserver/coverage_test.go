package specserver_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/qa/specserver"
)

// outboundCovered is the canonical list of outbound (SDK → atome-fin)
// endpoints exercised by per-package *_spec_test.go files. Keeping
// this as data — rather than scanning test sources — gives a
// single place to grep when the spec server reports a coverage gap.
//
// Inbound callback paths (POST /<*NotifyUrl>, /repayment-callback,
// /account_change_callback) are deliberately omitted: the SDK does
// not initiate them, and the spec server is outbound-only by design
// (architect §1.7 / SPEC_ASSERTION_TEST_DESIGN.md).
var outboundCovered = []string{
	// payment/
	"POST /auth",
	"POST /capture",
	"POST /voidAuth",
	"GET /query-auth",
	"GET /query-capture",
	"GET /query-voidAuth",
	"POST /payment-precheck",
	"POST /payment-plan",

	// refund/
	"POST /refund",
	"GET /query-refund",

	// repayment/
	"POST /repayment-request",
	"GET /repayment-result",

	// bill/
	"GET /bills",
	"GET /billDetail",
	"GET /billUnpaid",

	// transaction/
	"GET /transactions",
	"GET /transactionDetail",

	// credit/
	// v0.3 re-enabled the network path on /credit-information and
	// /credit-application via the new hybrid-encrypt envelope —
	// both endpoints rejoin the case table.
	"POST /credit-information",
	"POST /credit-application",
	"GET /credit-result",
	"GET /credit-information-result",
	"GET /query-balance-history",
	"POST /modify-application-info",
	"POST /close-account",

	// atomefin/ (umbrella)
	"GET /heart-beat",
}

// inboundCallbacks are the spec's partner-hosted endpoints. The SDK
// receives these via callback.* handlers; they are out of scope for
// the spec-server framework (which validates outbound requests
// only). Listed explicitly so the coverage cross-check can subtract
// them from "spec endpoint inventory minus covered".
var inboundCallbacks = []string{
	"POST /<authNotifyUrl>",
	"POST /<captureNotifyUrl>",
	"POST /<refundNotifyUrl>",
	"POST /<creditApplicationNotifyUrl>",
	"POST /repayment-callback",
	"POST /account_change_callback",
}

// TestSpec_AllOutboundEndpointsCovered asserts that every outbound
// endpoint declared by the pinned swagger.yaml has a matching entry
// in outboundCovered. Per architect's D3 (skip-warn), missing
// endpoints log a t.Logf record but do NOT fail the test — partial
// coverage is a real artefact while the upstream spec evolves
// faster than the SDK. Bumping the pinned spec adds new endpoints
// that may not yet have SDK coverage; the warning surfaces them
// without blocking CI.
//
// Spurious entries (covered claims an endpoint the spec doesn't
// declare) ARE a hard fail — that's a stale test reference.
func TestSpec_AllOutboundEndpointsCovered(t *testing.T) {
	spec, err := specserver.LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault: %v", err)
	}

	specOps := stringSet(spec.Endpoints())
	covered := stringSet(outboundCovered)
	inbound := stringSet(inboundCallbacks)

	// Spec ⊃ covered: every covered op must exist in the spec.
	var stale []string
	for op := range covered {
		if !specOps[op] {
			stale = append(stale, op)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("outboundCovered references endpoints not in pinned spec — stale entries:\n  %s",
			strings.Join(stale, "\n  "))
	}

	// Spec − inbound − covered = uncovered outbound. Skip-warn.
	var uncovered []string
	for op := range specOps {
		if covered[op] || inbound[op] {
			continue
		}
		uncovered = append(uncovered, op)
	}
	if len(uncovered) > 0 {
		sort.Strings(uncovered)
		t.Logf("Skipped: %d outbound spec endpoints not yet covered by *_spec_test.go:\n  %s",
			len(uncovered), strings.Join(uncovered, "\n  "))
	}

	// Inbound sanity: spec ⊃ inbound. If the spec drops an inbound
	// callback path our handler still references, we want to know.
	for op := range inbound {
		if !specOps[op] {
			t.Errorf("inboundCallbacks references endpoint not in pinned spec — stale entry: %s", op)
		}
	}
}

func stringSet(s []string) map[string]bool {
	out := make(map[string]bool, len(s))
	for _, v := range s {
		out[v] = true
	}
	return out
}
