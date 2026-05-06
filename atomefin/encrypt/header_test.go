package encrypt_test

import (
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

func TestBuildEncryptHeader_URLEscapesBase64Alphabet(t *testing.T) {
	// Base64 contains '+', '/', and '=' which break header values
	// without URL escaping. The protocol relies on it; the build
	// helper must apply it.
	got := encrypt.BuildEncryptHeader("a+b/c=")
	want := "symmetricKey=a%2Bb%2Fc%3D"
	if got != want {
		t.Errorf("BuildEncryptHeader = %q, want %q", got, want)
	}
}

func TestBuildAndParse_RoundTrip(t *testing.T) {
	cases := []string{
		"AAAA",
		"a+b/c=",
		"complexValueWithLotsOfChars0123456789+/=",
		// Real-shape: base64 of 256 bytes (RSA-2048 wrap output).
		strings.Repeat("ABcd+/=", 50),
	}
	for _, want := range cases {
		header := encrypt.BuildEncryptHeader(want)
		kv, err := encrypt.ParseEncryptHeader(header)
		if err != nil {
			t.Fatalf("ParseEncryptHeader(%q): %v", header, err)
		}
		got, err := encrypt.SymmetricKeyFrom(kv)
		if err != nil {
			t.Fatalf("SymmetricKeyFrom: %v", err)
		}
		if got != want {
			t.Errorf("round-trip mismatch:\n got=%q\nwant=%q", got, want)
		}
	}
}

func TestParseEncryptHeader_MultiKeyForwardCompat(t *testing.T) {
	// Future spec revision may add fields (e.g. iv= for AES-CBC).
	// The parser must surface them all so callers can decide.
	header := "symmetricKey=AAAA,iv=BBBB,alg=ECB"
	kv, err := encrypt.ParseEncryptHeader(header)
	if err != nil {
		t.Fatalf("ParseEncryptHeader: %v", err)
	}
	if kv["symmetricKey"] != "AAAA" {
		t.Errorf("symmetricKey = %q", kv["symmetricKey"])
	}
	if kv["iv"] != "BBBB" {
		t.Errorf("iv = %q", kv["iv"])
	}
	if kv["alg"] != "ECB" {
		t.Errorf("alg = %q", kv["alg"])
	}
	if len(kv) != 3 {
		t.Errorf("len(kv) = %d, want 3 (entries: %v)", len(kv), kv)
	}
}

func TestParseEncryptHeader_TolerantOfWhitespace(t *testing.T) {
	header := "  symmetricKey = AAAA , iv = BBBB  "
	kv, err := encrypt.ParseEncryptHeader(header)
	if err != nil {
		t.Fatalf("ParseEncryptHeader: %v", err)
	}
	if kv["symmetricKey"] != "AAAA" || kv["iv"] != "BBBB" {
		t.Errorf("kv = %v", kv)
	}
}

func TestParseEncryptHeader_RejectsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\t\n"} {
		if _, err := encrypt.ParseEncryptHeader(in); err == nil {
			t.Errorf("ParseEncryptHeader(%q): want error, got nil", in)
		}
	}
}

func TestParseEncryptHeader_RejectsMalformed(t *testing.T) {
	cases := []string{
		"justakey",                        // no '='
		"=valueonly",                      // empty key
		"symmetricKey=AAAA,malformedpart", // second part missing '='
	}
	for _, in := range cases {
		if _, err := encrypt.ParseEncryptHeader(in); err == nil {
			t.Errorf("ParseEncryptHeader(%q): want error, got nil", in)
		}
	}
}

func TestParseEncryptHeader_AllCommas(t *testing.T) {
	// "  ,  ," — every part is whitespace; no actual k=v pairs.
	if _, err := encrypt.ParseEncryptHeader("  ,  ,  "); err == nil {
		t.Error("ParseEncryptHeader all-commas: want error, got nil")
	}
}

func TestSymmetricKeyFrom_RejectsMissingOrEmpty(t *testing.T) {
	if _, err := encrypt.SymmetricKeyFrom(map[string]string{}); err == nil {
		t.Error("SymmetricKeyFrom empty-map: want error, got nil")
	}
	if _, err := encrypt.SymmetricKeyFrom(map[string]string{"symmetricKey": ""}); err == nil {
		t.Error("SymmetricKeyFrom empty-value: want error, got nil")
	}
	if _, err := encrypt.SymmetricKeyFrom(map[string]string{"iv": "AAAA"}); err == nil {
		t.Error("SymmetricKeyFrom missing-symmetricKey: want error, got nil")
	}
}

func TestEncryptHeaderName_ExactSpelling(t *testing.T) {
	if encrypt.EncryptHeaderName != "Encrypt" {
		t.Errorf("EncryptHeaderName = %q; want %q (the upstream gateway is case-sensitive)",
			encrypt.EncryptHeaderName, "Encrypt")
	}
	if encrypt.SymmetricKeyField != "symmetricKey" {
		t.Errorf("SymmetricKeyField = %q; want %q", encrypt.SymmetricKeyField, "symmetricKey")
	}
}
