package encrypt

import (
	"crypto/rand"
	"fmt"
)

// AESKeyAlphabetSize is the count of letters allowed in a
// partner-protocol-mandated AES key. Per Q34 (RESOLVED 2026-05-06),
// the 32 bytes are restricted to printable A — Z. Used for the
// rejection-sampling cutoff below.
const AESKeyAlphabetSize = 26 // 'A'..'Z'

// aesKeyAlphabetCutoff is the largest byte value that maps to a
// uniform A — Z distribution under modulo 26. 26 * 9 = 234; bytes
// >= 234 would skew toward A — V if reduced modulo 26 directly.
// RandomAESKey rejects bytes at or above this cutoff and re-rolls
// — fixes the (small but real) modulo bias the partner reference
// at `~/Downloads/main.go` line 367-370 carries.
const aesKeyAlphabetCutoff = AESKeyAlphabetSize * 9 // = 234

// RandomAESKey returns a fresh 32-character key drawn uniformly
// from A — Z. Uses crypto/rand and rejects bytes ≥ 234 to avoid
// the modulo bias the partner sample tolerates. Expected ~8.6%
// of reads rejected (22/256); cost negligible.
//
// The output is the **string form** the partner expects on the
// wire — the same 32 ASCII bytes go straight into AES.NewCipher
// without further transformation. Convert with []byte(...) when
// passing to EncryptBody / WrapAESKey.
func RandomAESKey() (string, error) {
	out := make([]byte, aesKeyBytes)
	for i := 0; i < aesKeyBytes; i++ {
		for {
			var one [1]byte
			if _, err := rand.Read(one[:]); err != nil {
				return "", fmt.Errorf("encrypt: rand.Read: %w", err)
			}
			if one[0] < aesKeyAlphabetCutoff {
				out[i] = 'A' + (one[0] % AESKeyAlphabetSize)
				break
			}
			// reject; re-roll
		}
	}
	return string(out), nil
}

// ValidateAESKey enforces the partner-mandated alphabet and
// length: exactly 32 chars, each in A — Z. Returns an error with
// the offending position when any byte is out of range. Used by
// the inbound (Unmarshal) path to reject corrupt keys early.
func ValidateAESKey(key string) error {
	if len(key) != aesKeyBytes {
		return fmt.Errorf("encrypt: AES key must be exactly %d chars, got %d", aesKeyBytes, len(key))
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		if c < 'A' || c > 'Z' {
			return fmt.Errorf("encrypt: AES key must be uppercase A — Z; invalid byte at index %d: %q", i, c)
		}
	}
	return nil
}
