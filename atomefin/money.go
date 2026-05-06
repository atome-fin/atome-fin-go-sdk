package atomefin

// Amount is an integer money value in the smallest currency unit (minor
// units) of the configured country — IDR rupiah, PHP centavos, etc.
//
// Per DESIGN.md §1.5 and the project-wide money policy, **every** money
// field on the public wire surface MUST be int64. No pointers (use the
// zero value 0 for "absent" and rely on `omitempty` on optional fields),
// no `json.Number`, no string. Float types are categorically banned to
// rule out scientific-notation drift on round-trip.
//
// The type is a named alias (not a distinct type) so JSON marshalling
// emits a plain integer — which is what the wire expects — without any
// custom MarshalJSON / UnmarshalJSON dance.
type Amount = int64

// Currency is an ISO-4217 currency code. As of v0.1.1 the spec
// enum-locks the supported set to **IDR** (Indonesian rupiah) — see
// DESIGN.md §13/Q10 RESOLVED 2026-05-06. The named-type form keeps
// the public surface forward-compatible with v2 currencies without
// breaking partner code: callers compare to the CurrencyIDR constant
// rather than to a string literal.
//
// Decode policy is **permissive** (forward-compat for future
// currencies the spec may add): JSON unmarshalling accepts any string
// because Currency's underlying type is string. Outbound emission is
// **strict** at the validator layer — request-side helpers reject
// non-IDR via IsValid before paying the network round-trip.
//
// Note that v0.1.0 shipped Currency as `type Currency = string`
// (alias). v0.1.1 promotes it to `type Currency string` (named). For
// most callers this is transparent — string literals assign to a
// Currency field directly. Code that bridges Currency↔string via raw
// `string(...)` casts continues to work; code that previously relied
// on implicit alias-equivalence may need an explicit conversion.
type Currency string

// Spec-defined currency literals.
const (
	// CurrencyIDR is Indonesian rupiah, the only currency supported by
	// the spec at v0.1.1.
	CurrencyIDR Currency = "IDR"
)

// IsValid reports whether c is a spec-defined currency. As of v0.1.1
// only CurrencyIDR is valid; future versions may broaden this.
//
// Decoding NEVER goes through IsValid — partners that receive an
// unexpected currency value can still inspect it (forward-compat).
// The validator on outbound request types calls IsValid to fail fast
// before transmitting a non-conformant payload.
func (c Currency) IsValid() bool {
	switch c {
	case CurrencyIDR:
		return true
	}
	return false
}

// String returns the wire literal verbatim. Implemented so a Currency
// value formats cleanly through fmt.Stringer-aware loggers.
func (c Currency) String() string { return string(c) }
