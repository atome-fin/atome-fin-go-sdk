package encrypt_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"

	"github.com/atome-fin/atome-fin-go-sdk/atomefin/encrypt"
)

// must2048Key generates a fresh 2048-bit RSA key for each test
// that needs one. RSA key generation is non-trivial (~50 ms on
// modern hardware) so tests cache where it makes sense; the
// tests below need fresh keys for negative-path coverage so they
// generate inline.
func must2048Key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return k
}

func TestWrapUnwrap_RoundTrip(t *testing.T) {
	priv := must2048Key(t)

	// Use a fixed AES-key shape but the wrap is non-deterministic
	// so we round-trip rather than byte-compare.
	aesKey := []byte("ATOMEFINENCRYPTTESTKEYAEZBSPQRWX")
	wrapped, err := encrypt.WrapAESKey(aesKey, &priv.PublicKey)
	if err != nil {
		t.Fatalf("WrapAESKey: %v", err)
	}
	got, err := encrypt.UnwrapAESKey(wrapped, priv)
	if err != nil {
		t.Fatalf("UnwrapAESKey: %v", err)
	}
	if !bytes.Equal(got, aesKey) {
		t.Errorf("round-trip mismatch: got %q, want %q", got, aesKey)
	}
}

func TestWrapAESKey_Rejects_NilKey(t *testing.T) {
	_, err := encrypt.WrapAESKey([]byte("x"), nil)
	if err == nil {
		t.Fatal("WrapAESKey nil-key: want error, got nil")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("err = %v; want mention of nil", err)
	}
}

func TestWrapAESKey_Rejects_TooSmallKey(t *testing.T) {
	// 1024-bit RSA — below the protocol's 2048-bit floor.
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey 1024: %v", err)
	}
	if _, err := encrypt.WrapAESKey([]byte("x"), &small.PublicKey); err == nil {
		t.Error("WrapAESKey 1024-bit key: want error, got nil")
	}
}

func TestUnwrapAESKey_Rejects_NilKey(t *testing.T) {
	if _, err := encrypt.UnwrapAESKey("AAAA", nil); err == nil {
		t.Error("UnwrapAESKey nil-priv: want error, got nil")
	}
}

func TestUnwrapAESKey_Rejects_TooSmallKey(t *testing.T) {
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("rsa.GenerateKey 1024: %v", err)
	}
	if _, err := encrypt.UnwrapAESKey("AAAA", small); err == nil {
		t.Error("UnwrapAESKey 1024-bit priv: want error, got nil")
	}
}

func TestUnwrapAESKey_Rejects_BadBase64(t *testing.T) {
	priv := must2048Key(t)
	if _, err := encrypt.UnwrapAESKey("not!base64!", priv); err == nil {
		t.Error("UnwrapAESKey bad-base64: want error, got nil")
	}
}

func TestUnwrapAESKey_Rejects_GarbageCiphertext(t *testing.T) {
	priv := must2048Key(t)
	// Valid base64 but ciphertext doesn't decrypt cleanly under
	// our private key (random 256 bytes).
	garbage := make([]byte, 256)
	if _, err := rand.Read(garbage); err != nil {
		t.Fatal(err)
	}
	// Encode and pass — should fail at rsa.DecryptPKCS1v15.
	encoded := base64Std(garbage)
	if _, err := encrypt.UnwrapAESKey(encoded, priv); err == nil {
		t.Error("UnwrapAESKey garbage-cipher: want error, got nil")
	}
}

// base64Std is a tiny inline helper so we don't pull encoding/base64
// into every test file's import block.
func base64Std(b []byte) string {
	const tab = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var out []byte
	for i := 0; i < len(b); i += 3 {
		var b0, b1, b2 byte
		b0 = b[i]
		if i+1 < len(b) {
			b1 = b[i+1]
		}
		if i+2 < len(b) {
			b2 = b[i+2]
		}
		out = append(out,
			tab[b0>>2],
			tab[((b0&0x03)<<4)|(b1>>4)],
			tab[((b1&0x0F)<<2)|(b2>>6)],
			tab[b2&0x3F],
		)
	}
	rem := len(b) % 3
	if rem == 1 {
		out[len(out)-2] = '='
		out[len(out)-1] = '='
	} else if rem == 2 {
		out[len(out)-1] = '='
	}
	return string(out)
}
