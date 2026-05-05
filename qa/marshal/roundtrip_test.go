package marshal

import (
	"math"
	"path/filepath"
	"testing"
)

// These self-tests exercise the harness against a small set of
// representative shapes that mirror what `atomefin/payment` will hold
// once T3 lands. They use only types defined in this file so the harness
// stays compilable independently of the rest of the SDK.

type sampleSubOrder struct {
	SubOrderID string `json:"subOrderId"`
	Amount     int64  `json:"amount"`
}

type sampleExtend struct {
	Address string `json:"address,omitempty"`
}

type sampleAuthRequest struct {
	RequestID            string           `json:"requestId"`
	ExternalReferenceUID string           `json:"externalReferenceUid"`
	TotalAmount          int64            `json:"totalAmount"`
	PeriodType           int              `json:"periodType"`
	SubOrders            []sampleSubOrder `json:"subOrders"`
	ExtendInfo           *sampleExtend    `json:"extendInfo,omitempty"`
}

// A type with a deliberate bug: required field carrying ",omitempty".
type buggyRequired struct {
	RequestID string `json:"requestId,omitempty"`
}

// A type with a deliberate bug: optional field NOT carrying ",omitempty".
type buggyOptional struct {
	Extend *sampleExtend `json:"extendInfo"`
}

func TestGoldenRoundTrip_AuthRequest(t *testing.T) {
	GoldenRoundTrip[sampleAuthRequest](t,
		filepath.Join("..", "testdata", "auth_request.json"))
}

func TestGoldenRoundTrip_AuthResponseSuccess(t *testing.T) {
	type authData struct {
		RequestID   string `json:"requestId"`
		Currency    string `json:"currency"`
		AuthOrderID string `json:"authOrderId"`
		TotalAmount int64  `json:"totalAmount"`
		Status      string `json:"status"`
	}
	type authResp struct {
		Code    string    `json:"code"`
		Message string    `json:"message"`
		Data    *authData `json:"data,omitempty"`
	}
	GoldenRoundTrip[authResp](t,
		filepath.Join("..", "testdata", "auth_response_success.json"))
}

func TestStrictDecode_RejectsUnknownField(t *testing.T) {
	// A struct that does NOT know about `mysteryField`.
	type narrow struct {
		Code string `json:"code"`
	}
	// hand-crafted bytes (not on disk) — use Decode directly.
	_, err := Decode[narrow]([]byte(`{"code":"SUCCESS","mysteryField":"x"}`))
	if err == nil {
		t.Fatal("expected DisallowUnknownFields to reject mysteryField")
	}
}

func TestAssertOmitemptyZero_PassesOnGood(t *testing.T) {
	// Zero value of sampleAuthRequest must NOT emit "extendInfo".
	AssertOmitemptyZero[sampleAuthRequest](t, "extendInfo")
}

func TestAssertOmitemptyZero_FailsOnMissingOmitempty(t *testing.T) {
	// `buggyOptional` is missing ,omitempty so the key WILL appear at
	// zero value — and our helper should flag it.
	tt := &testing.T{}
	AssertOmitemptyZero[buggyOptional](tt, "extendInfo")
	if !tt.Failed() {
		t.Fatal("AssertOmitemptyZero should have failed on buggyOptional " +
			"(extendInfo lacks ,omitempty)")
	}
}

func TestAssertRequiredEmits_PassesOnGood(t *testing.T) {
	// `requestId` is required (no ,omitempty) so it must appear even at
	// zero value.
	AssertRequiredEmits[sampleAuthRequest](t,
		"requestId", "externalReferenceUid", "totalAmount", "periodType")
}

func TestAssertRequiredEmits_FailsOnAccidentalOmitempty(t *testing.T) {
	tt := &testing.T{}
	AssertRequiredEmits[buggyRequired](tt, "requestId")
	if !tt.Failed() {
		t.Fatal("AssertRequiredEmits should have failed on buggyRequired " +
			"(requestId carries ,omitempty)")
	}
}

func TestDeepEqualRoundTrip_Programmatic(t *testing.T) {
	in := sampleAuthRequest{
		RequestID:            "01HABC1234567890ABCDEFGHJK",
		ExternalReferenceUID: "user-42",
		TotalAmount:          1_500_000, // IDR 1,500,000 in minor units
		PeriodType:           3,
		SubOrders: []sampleSubOrder{
			{SubOrderID: "so-1", Amount: 1_500_000},
		},
		ExtendInfo: &sampleExtend{Address: "南京路 123 号"}, // Unicode (R9)
	}
	DeepEqualRoundTrip[sampleAuthRequest](t, in)
}

func TestDeepEqualRoundTrip_BigAmount(t *testing.T) {
	// R7: minor-unit amount up to 9,999,999,999,999 must survive.
	in := sampleAuthRequest{
		RequestID:            "01H",
		ExternalReferenceUID: "u",
		TotalAmount:          9_999_999_999_999,
		PeriodType:           12,
		SubOrders:            []sampleSubOrder{{SubOrderID: "x", Amount: 9_999_999_999_999}},
	}
	DeepEqualRoundTrip[sampleAuthRequest](t, in)
}

func TestMarshal_NoTrailingNewline(t *testing.T) {
	b, err := Marshal(sampleSubOrder{SubOrderID: "x", Amount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 0 && b[len(b)-1] == '\n' {
		t.Fatalf("Marshal must not append trailing newline (signing canonical depends on raw bytes), got: %q", b)
	}
}

func TestHasTopLevelKey_NestedNotMatched(t *testing.T) {
	// Ensure we don't false-positive a key that lives in a nested
	// object. The harness's omitempty / required checks rely on this.
	in := []byte(`{"data":{"foo":"bar"}}`)
	if hasTopLevelKey(in, "foo") {
		t.Fatal("hasTopLevelKey matched a nested key 'foo' — must only match top-level")
	}
	if !hasTopLevelKey(in, "data") {
		t.Fatal("hasTopLevelKey failed to match top-level 'data'")
	}
}

func TestHasTopLevelKey_StringValueNotMatched(t *testing.T) {
	// A string value that *contains* the key name must not register.
	in := []byte(`{"message":"foo is great"}`)
	if hasTopLevelKey(in, "foo") {
		t.Fatal("hasTopLevelKey matched a substring inside a string value")
	}
}

// ---------------------------------------------------------------------------
// R10 / R11 / R12 — money-type invariants from the spec
// ---------------------------------------------------------------------------

// sampleAccountChanges mirrors the AccountChanges layout from the
// spec. Negative deltas are required to round-trip — credit can
// decrease as well as increase.
type sampleAccountChanges struct {
	Version              int64  `json:"version"`
	ExternalReferenceUID string `json:"externalReferenceUid"`
	PreviousStatus       string `json:"previousStatus"`
	CurrentStatus        string `json:"currentStatus"`
	TotalCreditChange    int64  `json:"totalCreditChange"`
	UsedCreditChange     int64  `json:"usedCreditChange"`
	FrozenCreditChange   int64  `json:"frozenCreditChange"`
}

func TestAmountCorpus_CoversBoundsAndZero(t *testing.T) {
	// Sanity: the canonical corpus is non-empty and covers the
	// signed extremes plus zero. R10 depends on these.
	c := AmountCorpus()
	if len(c) == 0 {
		t.Fatal("AmountCorpus returned empty slice")
	}
	have := map[int64]bool{}
	for _, v := range c {
		have[v] = true
	}
	for _, must := range []int64{math.MinInt64, -1, 0, 1, math.MaxInt64} {
		if !have[must] {
			t.Errorf("AmountCorpus missing required value %d", must)
		}
	}
}

func TestAssertAmountRoundtrip_Passes(t *testing.T) {
	// R10: every amount value in the corpus must round-trip cleanly,
	// including signed extremes for credit-change deltas.
	AssertAmountRoundtrip[sampleAccountChanges](t, func(v int64) sampleAccountChanges {
		return sampleAccountChanges{
			Version:              1700000000000,
			ExternalReferenceUID: "user-42",
			PreviousStatus:       "NORMAL",
			CurrentStatus:        "NORMAL",
			TotalCreditChange:    v,
			UsedCreditChange:     -v,
			FrozenCreditChange:   v / 2,
		}
	})
}

func TestAssertAmountRoundtrip_AlsoCoversTopLevelAmount(t *testing.T) {
	// Bind R10 to AuthRequest.TotalAmount so we exercise a
	// /auth-style top-level amount field too.
	AssertAmountRoundtrip[sampleAuthRequest](t, func(v int64) sampleAuthRequest {
		return sampleAuthRequest{
			RequestID:            "req-1",
			ExternalReferenceUID: "u",
			TotalAmount:          v,
			PeriodType:           3,
			SubOrders:            []sampleSubOrder{{SubOrderID: "so-1", Amount: v}},
		}
	})
}

func TestAssertRejectsFractionalAmount_FractionalLiteral(t *testing.T) {
	// R11: 1.5 into an int64 amount field must fail decode.
	type withAmount struct {
		Amount int64 `json:"amount"`
	}
	AssertRejectsFractionalAmount[withAmount](t, []byte(`{"amount":1.5}`))
}

func TestAssertRejectsFractionalAmount_ScientificNotation(t *testing.T) {
	// R11: 1e-3 (i.e. 0.001) into int64 also rejected.
	type withAmount struct {
		Amount int64 `json:"amount"`
	}
	AssertRejectsFractionalAmount[withAmount](t, []byte(`{"amount":1e-3}`))
}

func TestAssertRejectsFractionalAmount_NegativeCase(t *testing.T) {
	// Sanity: integer-valued JSON must NOT be flagged by R11.
	// We invoke AssertRejectsFractionalAmount on a clean integer body
	// and expect the inner test recorder to fail (i.e. the helper
	// would NOT have rejected, so the assertion does fail).
	type withAmount struct {
		Amount int64 `json:"amount"`
	}
	tt := &testing.T{}
	AssertRejectsFractionalAmount[withAmount](tt, []byte(`{"amount":12345}`))
	if !tt.Failed() {
		t.Fatal("AssertRejectsFractionalAmount must fail when given a clean integer body " +
			"(otherwise R11 would silently approve valid integers as 'errors')")
	}
}

func TestAssertAmountKeysAreInteger_Passes(t *testing.T) {
	// R12: marshalled output of a clean int64-only struct must contain
	// no '.' or 'e'/'E' at any amount-key position, even at MaxInt64.
	in := sampleAuthRequest{
		RequestID:            "req-1",
		ExternalReferenceUID: "u",
		TotalAmount:          math.MaxInt64,
		PeriodType:           12,
		SubOrders: []sampleSubOrder{
			{SubOrderID: "so-1", Amount: math.MinInt64},
			{SubOrderID: "so-2", Amount: 0},
		},
	}
	AssertAmountKeysAreInteger[sampleAuthRequest](t, in,
		"totalAmount", "amount")
}

func TestAssertAmountKeysAreInteger_FailsOnFloatField(t *testing.T) {
	// A struct that uses float64 for an amount field — R12 violation.
	// The helper must flag it.
	type bad struct {
		Amount float64 `json:"amount"`
	}
	tt := &testing.T{}
	AssertAmountKeysAreInteger[bad](tt, bad{Amount: 1.5}, "amount")
	if !tt.Failed() {
		t.Fatal("AssertAmountKeysAreInteger should have flagged a float64 amount field")
	}
}

func TestAssertAmountKeysAreInteger_FailsOnScientificNotation(t *testing.T) {
	// Go's json encoder uses 'g' format for floats; values >= 1e21 emit
	// in scientific notation (e.g. "1e+308"). Confirms R12 catches
	// scientific-form output too, not just '.'.
	type bad struct {
		Amount float64 `json:"amount"`
	}
	tt := &testing.T{}
	AssertAmountKeysAreInteger[bad](tt, bad{Amount: 1e308}, "amount")
	if !tt.Failed() {
		t.Fatal("AssertAmountKeysAreInteger should flag scientific-notation output (1e308)")
	}
}

func TestAssertAmountKeysAreInteger_IgnoresNonAmountFloat(t *testing.T) {
	// userCreditScore is a legit float (0..1 score, not money) — R12
	// must NOT flag it because we don't list it in amountKeys.
	type req struct {
		UserCreditScore float64 `json:"userCreditScore"`
		Amount          int64   `json:"amount"`
	}
	in := req{UserCreditScore: 0.875, Amount: 12345}
	AssertAmountKeysAreInteger[req](t, in, "amount")
}

func TestAssertAmountKeysAreInteger_NestedSliceAndObject(t *testing.T) {
	// AccountChanges-shaped struct nested under a top-level wrapper —
	// confirms the walker recurses into both objects and slices.
	type wrap struct {
		Data    *sampleAccountChanges  `json:"data"`
		History []sampleAccountChanges `json:"history"`
	}
	in := wrap{
		Data: &sampleAccountChanges{
			Version: 1, ExternalReferenceUID: "u",
			PreviousStatus: "NORMAL", CurrentStatus: "NORMAL",
			TotalCreditChange:  -math.MaxInt64,
			UsedCreditChange:   math.MaxInt64,
			FrozenCreditChange: 0,
		},
		History: []sampleAccountChanges{
			{Version: 2, ExternalReferenceUID: "u", PreviousStatus: "NORMAL",
				CurrentStatus: "NORMAL", TotalCreditChange: 1},
		},
	}
	AssertAmountKeysAreInteger[wrap](t, in,
		"totalCreditChange", "usedCreditChange", "frozenCreditChange")
}
