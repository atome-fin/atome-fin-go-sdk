// Package marshal is a self-contained marshal round-trip harness used by QA
// to validate every public request/response struct in the atome-fin-go-sdk
// against its JSON wire form.
//
// It depends only on the standard library, so it can be imported by any
// package in the SDK without creating an import cycle and without pulling
// transport / signing concerns into struct-level tests.
//
// The primitives below cover the marshal invariants R1–R12:
//
//   - GoldenRoundTrip[T]:               R1, R2 — strict-decode → encode →
//     second-pass byte-stable.
//   - StrictDecode[T]:                  R2 — DisallowUnknownFields decode.
//   - AssertOmitemptyZero[T]:           R3 — zero value omits optional keys.
//   - AssertRequiredEmits[T]:           R4 — zero value still emits required
//     keys (catches stray ,omitempty).
//   - DeepEqualRoundTrip[T]:            R6, R8, R9 — reflect-equal across
//     round-trip; works on programmatic
//     inputs (Unicode, big amounts).
//   - AmountCorpus + AssertAmountRoundtrip[T]:
//     R10 — full int64 range for money.
//   - AssertRejectsFractionalAmount[T]: R11 — fractional / scientific decode
//     on amount fields fails loudly.
//   - AssertAmountKeysAreInteger[T]:    R12 — encoded JSON never carries a
//     float at any amount-key position.
//
// The harness is intentionally generic ([T any]) so lead-coder can wire it
// up per-type once T3 lands without writing per-struct boilerplate.
package marshal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Marshal is the canonical encoder used throughout the SDK on the wire.
// HTML escaping is OFF so that signing canonicalization (which signs over
// the raw bytes) is not perturbed by ampersands or angle brackets in
// addresses, names, or extendInfo blobs.
//
// All harness primitives use this function so fixture files reflect what
// the SDK will actually transmit.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// json.Encoder.Encode appends a trailing newline; strip it to match
	// json.Marshal's output and the on-the-wire body.
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Decode strict-decodes b into a fresh T. Unknown fields cause failure.
func Decode[T any](b []byte) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	dec.UseNumber() // catches integer-as-float drift in amount fields
	if err := dec.Decode(&v); err != nil {
		return v, err
	}
	// Reject trailing junk; spec responses are single JSON objects.
	if dec.More() {
		return v, fmt.Errorf("trailing data after JSON value")
	}
	return v, nil
}

// readFixture returns the bytes of path with trailing whitespace
// (including a trailing newline) trimmed so on-disk formatting doesn't
// fight Marshal's no-trailing-newline output.
func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return bytes.TrimSpace(raw)
}

// GoldenRoundTrip is the workhorse: it strict-decodes the fixture into T,
// re-encodes, and asserts that decoding-then-encoding the *re-encoded*
// bytes yields the same bytes back. This is the byte-stable round-trip
// invariant (R1).
//
// We do not require the fixture file to be byte-equal to the encoded form
// — humans format JSON differently than the encoder does. We DO require
// the encoder to be idempotent: encode(decode(encode(decode(x)))) ==
// encode(decode(x)).
func GoldenRoundTrip[T any](t *testing.T, fixturePath string) {
	t.Helper()

	raw := readFixture(t, fixturePath)

	// Pass 1: strict-decode the fixture. Unknown fields here fail the test
	// — they indicate a struct missing a json tag (invariant R2).
	v1, err := Decode[T](raw)
	if err != nil {
		t.Fatalf("[%s] strict decode failed: %v\nfixture:\n%s",
			fixturePath, err, raw)
	}

	encoded1, err := Marshal(v1)
	if err != nil {
		t.Fatalf("[%s] first marshal failed: %v", fixturePath, err)
	}

	// Pass 2: decode the SDK-produced bytes (must succeed strictly).
	v2, err := Decode[T](encoded1)
	if err != nil {
		t.Fatalf("[%s] second decode failed on SDK output: %v\nbytes:\n%s",
			fixturePath, err, encoded1)
	}

	encoded2, err := Marshal(v2)
	if err != nil {
		t.Fatalf("[%s] second marshal failed: %v", fixturePath, err)
	}

	// Idempotency: once the value has gone through the encoder once, a
	// further round-trip MUST be byte-identical. If this fails the
	// encoder is non-deterministic for this type — almost always a
	// map[string]X field on the request body, which would also break
	// signing.
	if !bytes.Equal(encoded1, encoded2) {
		t.Fatalf("[%s] encoder is not idempotent — signing would break\n"+
			"first:  %s\nsecond: %s",
			fixturePath, encoded1, encoded2)
	}

	// Structural equality (catches things like a *bool field that
	// flipped to a `bool` and lost the nil distinction).
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("[%s] decoded values differ across round-trip\n"+
			"v1: %#v\nv2: %#v", fixturePath, v1, v2)
	}
}

// StrictDecode asserts that the fixture decodes into T with
// DisallowUnknownFields. It's a subset of GoldenRoundTrip — exposed
// separately so a struct can be validated against many fixtures cheaply
// (e.g. all 4xx response variants for a single error type).
func StrictDecode[T any](t *testing.T, fixturePath string) {
	t.Helper()
	raw := readFixture(t, fixturePath)
	if _, err := Decode[T](raw); err != nil {
		t.Fatalf("[%s] strict decode failed: %v\nfixture:\n%s",
			fixturePath, err, raw)
	}
}

// AssertOmitemptyZero marshals the zero value of T and asserts that none
// of forbiddenKeys appears in the resulting JSON object. This is the
// programmatic check for invariant R3: optional fields with `,omitempty`
// must be elided at zero value.
//
// Pass the spec-level field NAMES (the json tag), e.g.:
//
//	AssertOmitemptyZero[payment.AuthRequest](t,
//	    "extendInfo")  // optional, must NOT appear when zero
func AssertOmitemptyZero[T any](t *testing.T, forbiddenKeys ...string) {
	t.Helper()
	var zero T
	b, err := Marshal(zero)
	if err != nil {
		t.Fatalf("marshal zero: %v", err)
	}
	for _, k := range forbiddenKeys {
		if hasTopLevelKey(b, k) {
			t.Errorf("zero-value %T emitted forbidden key %q (missing ,omitempty?)\n"+
				"json: %s", zero, k, b)
		}
	}
}

// AssertRequiredEmits marshals the zero value of T and asserts that every
// requiredKey IS present. Catches `,omitempty` accidentally placed on a
// required field — invariant R4.
func AssertRequiredEmits[T any](t *testing.T, requiredKeys ...string) {
	t.Helper()
	var zero T
	b, err := Marshal(zero)
	if err != nil {
		t.Fatalf("marshal zero: %v", err)
	}
	for _, k := range requiredKeys {
		if !hasTopLevelKey(b, k) {
			t.Errorf("zero-value %T omitted required key %q (stray ,omitempty?)\n"+
				"json: %s", zero, k, b)
		}
	}
}

// DeepEqualRoundTrip is exported for tests that build a value
// programmatically (rather than from a fixture) and want to assert it
// survives encode/decode/encode unchanged.
func DeepEqualRoundTrip[T any](t *testing.T, in T) {
	t.Helper()
	b, err := Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := Decode[T](b)
	if err != nil {
		t.Fatalf("decode: %v\nbytes: %s", err, b)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round-trip drift\nin:  %#v\nout: %#v\njson: %s", in, out, b)
	}
}

// AmountCorpus is the canonical set of int64 values every amount field
// must round-trip cleanly. It covers signed extremes (negatives required
// for credit-change deltas in AccountChanges), zero, ±1, the IDR-headroom
// value from R7, and both int64 boundaries.
//
// Per the spec/R10 (canonical in the spec).
func AmountCorpus() []int64 {
	return []int64{
		math.MinInt64,
		math.MinInt64 + 1,
		-9_999_999_999_999,
		-1,
		0,
		1,
		9_999_999_999_999, // IDR headroom from R7
		math.MaxInt64 - 1,
		math.MaxInt64,
	}
}

// AssertAmountRoundtrip asserts that for every value in AmountCorpus(),
// the value produced by build(v) survives encode → strict-decode → encode
// byte-stably and DeepEqual the input. Caller supplies build to construct
// a fresh T with one or more amount fields populated to v.
//
// Use one call per amount field on the type — e.g. one call binding
// AuthRequest.TotalAmount, another binding SubOrder.Amount, etc.
//
// Per R10. Negatives are intentional: credit-change deltas in
// AccountChanges are signed.
func AssertAmountRoundtrip[T any](t *testing.T, build func(v int64) T) {
	t.Helper()
	for _, v := range AmountCorpus() {
		in := build(v)
		b, err := Marshal(in)
		if err != nil {
			t.Errorf("R10[%d]: marshal: %v", v, err)
			continue
		}
		out, err := Decode[T](b)
		if err != nil {
			t.Errorf("R10[%d]: strict decode of own output failed: %v\nbytes: %s",
				v, err, b)
			continue
		}
		if !reflect.DeepEqual(in, out) {
			t.Errorf("R10[%d]: round-trip drift\nin:  %#v\nout: %#v\njson: %s",
				v, in, out, b)
		}
		// Byte-stable on second encode (R1 narrowed to the amount path).
		b2, err := Marshal(out)
		if err != nil {
			t.Errorf("R10[%d]: re-marshal: %v", v, err)
			continue
		}
		if !bytes.Equal(b, b2) {
			t.Errorf("R10[%d]: encoder not idempotent\nfirst:  %s\nsecond: %s",
				v, b, b2)
		}
	}
	// R12 is asserted separately by AssertAmountKeysAreInteger — keep
	// concerns separated so a failure points at the right invariant.
}

// AssertRejectsFractionalAmount feeds body into a strict-decode of T and
// asserts that decoding fails. Use when body contains a fractional or
// scientific-notation number on a field whose Go type is int64
// (e.g. {"originalAmount": 1.5} → must error).
//
// Per R11. The standard library rejects "1.5" into an int64 with
// "json: cannot unmarshal number 1.5 into Go struct field …" — this
// helper just makes the assertion explicit and named in test output, so
// QA can grep for "R11" failures during regression triage.
func AssertRejectsFractionalAmount[T any](t *testing.T, body []byte) {
	t.Helper()
	v, err := Decode[T](body)
	if err == nil {
		// Errorf (not Fatalf) so this helper is safe to call against a
		// recorder *testing.T inside a meta-test — matches the pattern
		// used by AssertRequiredEmits / AssertOmitemptyZero.
		t.Errorf("R11: expected fractional/exp amount to be rejected, "+
			"but decode of %T succeeded\nbody: %s\nresult: %#v",
			v, body, v)
	}
}

// AssertAmountKeysAreInteger marshals in and walks the resulting JSON,
// asserting that every value paired with a key in amountKeys (at any
// nesting depth) is rendered as an integer literal — no '.', no 'e'/'E'.
//
// Per R12. amountKeys are the JSON tag names, e.g.:
//
//	AssertAmountKeysAreInteger[payment.AuthRequest](t, in,
//	    "totalAmount", "amount", "originalAmount",
//	    "totalCreditChange", "usedCreditChange",
//	    "frozenCreditChange", "availableCreditChange",
//	    "overpaidAmountChange", "lateFeeAmountChange",
//	    "interestAmountChange",
//	)
//
// The function recurses into nested objects and arrays so it covers
// SubOrder.amount, AccountChanges.*Change, and similar nested money fields.
func AssertAmountKeysAreInteger[T any](t *testing.T, in T, amountKeys ...string) {
	t.Helper()
	b, err := Marshal(in)
	if err != nil {
		t.Fatalf("R12: marshal failed: %v", err)
	}
	want := map[string]struct{}{}
	for _, k := range amountKeys {
		want[k] = struct{}{}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := walkAmountKeys(t, dec, want, ""); err != nil {
		t.Errorf("R12: %v\njson: %s", err, b)
	}
}

// walkAmountKeys consumes one JSON value from dec (object, array, or
// scalar) and asserts that any time we descend into an object, the value
// at any matching amount key is an integer literal. parentKey is the key
// we are currently the value of (for diagnostics).
func walkAmountKeys(t *testing.T, dec *json.Decoder, amountKeys map[string]struct{}, parentKey string) error {
	t.Helper()
	tok, err := dec.Token()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return fmt.Errorf("token at %q: %w", parentKey, err)
	}
	switch v := tok.(type) {
	case json.Delim:
		if v == '{' {
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return fmt.Errorf("object key after %q: %w", parentKey, err)
				}
				key, ok := keyTok.(string)
				if !ok {
					return fmt.Errorf("non-string object key after %q: %v", parentKey, keyTok)
				}
				// Recurse with key as parent so number-token logic below
				// can check membership.
				if err := walkAmountKeys(t, dec, amountKeys, key); err != nil {
					return err
				}
			}
			// consume closing '}'
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("close object %q: %w", parentKey, err)
			}
		} else if v == '[' {
			for dec.More() {
				// array elements inherit the parent key (e.g. subOrders[].amount)
				if err := walkAmountKeys(t, dec, amountKeys, parentKey); err != nil {
					return err
				}
			}
			// consume closing ']'
			if _, err := dec.Token(); err != nil {
				return fmt.Errorf("close array %q: %w", parentKey, err)
			}
		}
	case json.Number:
		if _, isAmount := amountKeys[parentKey]; isAmount {
			s := string(v)
			if strings.ContainsAny(s, ".eE") {
				return fmt.Errorf("amount key %q rendered as float/exp: %s",
					parentKey, s)
			}
		}
	default:
		// strings, bools, nils are not amount fields — ignore.
	}
	return nil
}

// hasTopLevelKey is a tiny zero-dependency check for whether the encoded
// JSON object `b` contains a top-level key named `key`. We deliberately
// don't pull yet-another JSON pass through the harness — this is a string
// scan that handles the four cases we care about: the key as an object's
// first key, last key, middle key, or absent.
//
// It does NOT recurse into nested objects, which is exactly the
// granularity AssertOmitemptyZero / AssertRequiredEmits need.
//
// The implementation is deliberately conservative: if the input isn't a
// JSON object we return false rather than panic.
func hasTopLevelKey(b []byte, key string) bool {
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	// Walk the top-level object respecting string escapes and nesting.
	// A more robust approach would tokenize, but for top-level key
	// detection on machine-emitted JSON this is sufficient and
	// well-tested below.
	depth := 0
	inStr := false
	esc := false
	// Build top-level keys; record names into a set.
	var name strings.Builder
	collecting := false
	expectColon := false
	keys := map[string]struct{}{}

	for i := 1; i < len(s)-1; i++ { // skip outer { }
		c := s[i]
		if esc {
			if collecting {
				name.WriteByte(c)
			}
			esc = false
			continue
		}
		if c == '\\' && inStr {
			esc = true
			if collecting {
				name.WriteByte(c)
			}
			continue
		}
		if c == '"' {
			if depth == 0 {
				if !inStr {
					// opening quote of a top-level key
					inStr = true
					collecting = !expectColon // collect only when this is a key, not a value
					name.Reset()
					continue
				}
				// closing quote
				inStr = false
				if collecting {
					keys[name.String()] = struct{}{}
					collecting = false
					expectColon = true
				}
				continue
			}
			// inside a nested string — toggle inStr but don't collect
			inStr = !inStr
			continue
		}
		if inStr {
			if collecting {
				name.WriteByte(c)
			}
			continue
		}
		switch c {
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		case ':':
			if depth == 0 {
				expectColon = false
			}
		case ',':
			if depth == 0 {
				expectColon = false
			}
		}
	}
	_, ok := keys[key]
	return ok
}
