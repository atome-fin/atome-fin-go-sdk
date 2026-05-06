package encrypt_test

import (
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

func TestRandomAESKey_LengthAndAlphabet(t *testing.T) {
	for i := 0; i < 100; i++ {
		k, err := encrypt.RandomAESKey()
		if err != nil {
			t.Fatalf("RandomAESKey: %v", err)
		}
		if len(k) != 32 {
			t.Errorf("len = %d, want 32 (key %q)", len(k), k)
		}
		for j := 0; j < len(k); j++ {
			c := k[j]
			if c < 'A' || c > 'Z' {
				t.Errorf("byte at %d = %q; want A-Z (key %q)", j, c, k)
			}
		}
	}
}

// TestRandomAESKey_StatisticalUniformity guards against a future
// regression that re-introduces the modulo bias the partner sample
// carries. With rejection sampling, every letter A-Z should appear
// with probability ~1/26 = 3.846%. We sample 100,000 keys (3.2M
// chars total), then check each letter's frequency falls within
// ±1% of 1/26 — a generous bound that still flags the modulo-26
// bias (which would push A-V to ~10/256 = 3.91% and W-Z to
// ~9/256 = 3.52%, a 0.39% gap).
func TestRandomAESKey_StatisticalUniformity(t *testing.T) {
	if testing.Short() {
		t.Skip("statistical test — skip under -short")
	}
	const nKeys = 100_000
	const charsPerKey = 32
	const total = nKeys * charsPerKey
	const expected = total / 26
	const toleranceBPS = 100 // ±1.0% of expected count
	tolerance := expected * toleranceBPS / 10000

	var counts [26]int
	for i := 0; i < nKeys; i++ {
		k, err := encrypt.RandomAESKey()
		if err != nil {
			t.Fatalf("RandomAESKey iter %d: %v", i, err)
		}
		for j := 0; j < len(k); j++ {
			counts[k[j]-'A']++
		}
	}
	for letter, got := range counts {
		diff := got - expected
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("letter %q: count = %d; want %d ± %d (deviation %d > tolerance)",
				'A'+byte(letter), got, expected, tolerance, diff)
		}
	}
}

func TestValidateAESKey_AcceptsCanonicalKey(t *testing.T) {
	if err := encrypt.ValidateAESKey("ATOMEFINENCRYPTTESTKEYAEZBSPQRWX"); err != nil {
		t.Errorf("ValidateAESKey: %v", err)
	}
}

func TestValidateAESKey_RejectsBadLength(t *testing.T) {
	cases := []string{"", "A", strings.Repeat("A", 31), strings.Repeat("A", 33), strings.Repeat("A", 64)}
	for _, k := range cases {
		if err := encrypt.ValidateAESKey(k); err == nil {
			t.Errorf("ValidateAESKey(len=%d): want error, got nil", len(k))
		}
	}
}

func TestValidateAESKey_RejectsBadAlphabet(t *testing.T) {
	cases := map[string]string{
		"lowercase-a":   strings.Repeat("a", 32),
		"digit":         strings.Repeat("0", 32),
		"mixed-letters": strings.Repeat("A", 31) + "9",
		"space":         strings.Repeat("A", 31) + " ",
		"unicode":       strings.Repeat("A", 31) + "É",
	}
	for name, k := range cases {
		if err := encrypt.ValidateAESKey(k); err == nil {
			t.Errorf("ValidateAESKey %s: want error, got nil", name)
		}
	}
}
