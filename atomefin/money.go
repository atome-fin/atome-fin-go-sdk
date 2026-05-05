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

// Currency is an ISO-4217 currency code (e.g. "IDR", "PHP"). The SDK does
// not validate the value because the supported set is agreed per partner
// per country (DESIGN.md §13/Q10 — open). Once the partner confirms the
// concrete list we will tighten this to a named string type with a
// validator and per-currency minor-unit scale; for now the field passes
// through verbatim.
type Currency = string
