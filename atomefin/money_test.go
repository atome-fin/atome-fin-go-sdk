package atomefin

import (
	"encoding/json"
	"testing"
)

// v0.1.1 promoted Currency from `type Currency = string` (alias) to
// `type Currency string` (named) and added the IDR enum. These tests
// pin the new contract: the named-type wire shape is preserved (we
// still serialise as a plain string), IsValid is strict on IDR, and
// decode policy stays permissive (forward-compat for future
// currencies).

func TestCurrency_IsValid_OnlyIDR(t *testing.T) {
	if !CurrencyIDR.IsValid() {
		t.Errorf("CurrencyIDR.IsValid() = false, want true")
	}
	for _, c := range []Currency{"USD", "PHP", "SGD", "EUR", "", "idr", "Idr"} {
		if c.IsValid() {
			t.Errorf("Currency(%q).IsValid() = true, want false (only IDR is enum-valid at v0.1.1)", c)
		}
	}
}

func TestCurrency_String_IsWireLiteral(t *testing.T) {
	if got := CurrencyIDR.String(); got != "IDR" {
		t.Errorf("CurrencyIDR.String() = %q, want %q", got, "IDR")
	}
	if got := Currency("USD").String(); got != "USD" {
		t.Errorf("Currency(\"USD\").String() = %q; named-type must round-trip the underlying string verbatim", got)
	}
}

// Permissive-decode contract: a Currency field decoded from JSON
// MUST accept any string literal even when !IsValid. The JSON layer
// does not call IsValid; forward-compat for future currencies the
// spec may add. Round-trips a tiny wrapper to surface the property.
func TestCurrency_PermissiveDecode(t *testing.T) {
	type wrapper struct {
		Currency Currency `json:"currency"`
	}
	for _, lit := range []string{"IDR", "PHP", "USD", "X-FUTURE-CURRENCY"} {
		var got wrapper
		body := []byte(`{"currency":"` + lit + `"}`)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("Unmarshal(%q): %v — Currency must be permissive on inbound", lit, err)
			continue
		}
		if string(got.Currency) != lit {
			t.Errorf("Unmarshal(%q): got %q, want %q", lit, got.Currency, lit)
		}
	}
}

// String-literal assignment to a Currency field still works (compiler
// allows untyped-string-constant → named-string conversion). This
// pins the v0.1.1 ergonomics so a partner doesn't trip over the alias
// → named-type promotion in the obvious code path.
func TestCurrency_LiteralAssignmentCompiles(t *testing.T) {
	var c Currency = "IDR" // ergonomic; would also work for the alias form
	if c != CurrencyIDR {
		t.Errorf("literal assignment != CurrencyIDR: got %q", c)
	}
}
