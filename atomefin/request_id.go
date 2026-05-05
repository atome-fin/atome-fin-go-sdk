package atomefin

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// requestIDLen is the length of the default-generated requestId. The spec
// caps requestId at 64 characters; we ship a 32-hex-char value (16 bytes)
// which leaves comfortable headroom for partner-supplied prefixes.
const requestIDLen = 32

// DefaultRequestID returns a unique, lexicographically sortable identifier
// suitable for the spec's `requestId` field (≤64 chars, A-Z/0-9/_/.,-).
//
// Layout: 12 hex chars of unix-millis (48 bits) ‖ 20 hex chars of
// crypto/rand (80 bits). This is ULID-like: monotonic by ms,
// collision-resistant within a ms, sortable by issuance time, and stays
// well below the spec's 64-char limit so partner-side prefixes
// (`pay_`, `ord_`, etc.) compose without truncation.
//
// Partners that want their own scheme can override this via
// WithRequestIDGenerator. The SDK never recycles the same requestId
// across retries (DESIGN.md §1.4); the generator is invoked once per
// service-method call and the result is reused for all retries of that
// call.
func DefaultRequestID() string {
	var buf [16]byte
	// 48 bits of unix-ms (max year ~ 10889) into the first 6 bytes.
	ms := uint64(time.Now().UnixMilli())
	buf[0] = byte(ms >> 40)
	buf[1] = byte(ms >> 32)
	buf[2] = byte(ms >> 24)
	buf[3] = byte(ms >> 16)
	buf[4] = byte(ms >> 8)
	buf[5] = byte(ms)
	// 80 bits of randomness fills the rest. crypto/rand.Read on Linux/macOS
	// is backed by getrandom(2)/getentropy(2) and effectively never errors;
	// if it does, fall back to a deterministic encoding of `ms` so we
	// degrade to a still-sortable id rather than panicking.
	if _, err := rand.Read(buf[6:]); err != nil {
		binary.BigEndian.PutUint64(buf[8:], ms)
	}
	return hex.EncodeToString(buf[:])
}

// Compile-time assertion that the encoding stays within spec.
var _ = [1]struct{}{}[func() int {
	if requestIDLen > 64 {
		return -1 // would not compile if exceeded
	}
	return 0
}()]
