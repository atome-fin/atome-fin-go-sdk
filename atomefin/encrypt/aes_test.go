package encrypt_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

// 32-byte key used across the AES tests. A — Z only (matches the
// partner constraint exercised by ValidateAESKey, but EncryptBody
// itself only enforces the 32-byte length).
var testAESKey = []byte("ATOMEFINENCRYPTTESTKEYAEZBSPQRWX")

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	cases := []struct {
		name  string
		plain []byte
	}{
		{"empty", []byte{}},
		{"one-byte", []byte("A")},
		{"block-aligned-16", bytes.Repeat([]byte("X"), 16)},
		{"block-aligned-32", bytes.Repeat([]byte("X"), 32)},
		{"one-short-of-block", bytes.Repeat([]byte("X"), 15)},
		{"json-shape", []byte(`{"requestId":"r-1","externalReferenceUid":"u-1"}`)},
		{"utf8", []byte("hello é ü ñ — 中文 — 🚀")},
		{"large-1kb", bytes.Repeat([]byte("X"), 1024)},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			b64, err := encrypt.EncryptBody(tc.plain, testAESKey)
			if err != nil {
				t.Fatalf("EncryptBody: %v", err)
			}
			got, err := encrypt.DecryptBody(b64, testAESKey)
			if err != nil {
				t.Fatalf("DecryptBody: %v", err)
			}
			if !bytes.Equal(got, tc.plain) {
				t.Errorf("round-trip mismatch:\n got=%q\nwant=%q", got, tc.plain)
			}
		})
	}
}

func TestEncryptBody_RejectsWrongKeyLength(t *testing.T) {
	for _, n := range []int{0, 1, 16, 24, 31, 33, 64} {
		key := bytes.Repeat([]byte("A"), n)
		if _, err := encrypt.EncryptBody([]byte("x"), key); err == nil {
			t.Errorf("EncryptBody key-len=%d: want error, got nil", n)
		} else if !strings.Contains(err.Error(), "32") {
			t.Errorf("EncryptBody key-len=%d: err = %v; want mention of 32", n, err)
		}
	}
}

func TestDecryptBody_RejectsWrongKeyLength(t *testing.T) {
	if _, err := encrypt.DecryptBody("AAAA", bytes.Repeat([]byte("A"), 16)); err == nil {
		t.Error("DecryptBody short-key: want error, got nil")
	}
}

func TestDecryptBody_RejectsBadBase64(t *testing.T) {
	if _, err := encrypt.DecryptBody("not!valid!base64", testAESKey); err == nil {
		t.Error("DecryptBody bad-base64: want error, got nil")
	}
}

func TestDecryptBody_RejectsBadBlockAlignment(t *testing.T) {
	// 17 bytes of garbage → not a multiple of AES block size.
	bad := strings.Repeat("AAAA", 5) + "AB" // 22 chars b64 → 17 bytes
	if _, err := encrypt.DecryptBody(bad, testAESKey); err == nil {
		t.Error("DecryptBody bad-alignment: want error, got nil")
	}
}

func TestDecryptBody_RejectsEmptyCiphertext(t *testing.T) {
	if _, err := encrypt.DecryptBody("", testAESKey); err == nil {
		t.Error("DecryptBody empty: want error, got nil")
	}
}

func TestDecryptBody_RejectsBadPadding(t *testing.T) {
	// Encrypt a known plaintext, then flip a byte in the LAST block
	// to corrupt the padding. AES-ECB last block decrypt produces
	// nonsense bytes including the pad-length byte.
	b64, err := encrypt.EncryptBody([]byte("hello"), testAESKey)
	if err != nil {
		t.Fatalf("EncryptBody: %v", err)
	}
	// Flip one base64 char to mutate the last block (bit-flip).
	mutated := []byte(b64)
	if mutated[len(mutated)-2] == 'X' {
		mutated[len(mutated)-2] = 'Y'
	} else {
		mutated[len(mutated)-2] = 'X'
	}
	_, err = encrypt.DecryptBody(string(mutated), testAESKey)
	if err == nil {
		t.Error("DecryptBody mutated-ciphertext: want padding error, got nil")
	}
}

// Determinism guard: same (plaintext, key) MUST produce identical
// ciphertext on every call. ECB is the partner-protocol-mandated
// cipher precisely because of this property — the external-vector
// test (external_vector_test.go) relies on it.
func TestEncryptBody_DeterministicOnFixedKey(t *testing.T) {
	plain := []byte("the quick brown fox jumps over the lazy dog")
	first, err := encrypt.EncryptBody(plain, testAESKey)
	if err != nil {
		t.Fatalf("EncryptBody #1: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := encrypt.EncryptBody(plain, testAESKey)
		if err != nil {
			t.Fatalf("EncryptBody #%d: %v", i+2, err)
		}
		if got != first {
			t.Fatalf("EncryptBody non-deterministic at iter %d:\n#1 = %q\nnow = %q", i+2, first, got)
		}
	}
}
