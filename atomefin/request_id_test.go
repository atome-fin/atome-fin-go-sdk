package atomefin

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestDefaultRequestIDLength(t *testing.T) {
	id := DefaultRequestID()
	if len(id) != requestIDLen {
		t.Errorf("DefaultRequestID length = %d, want %d", len(id), requestIDLen)
	}
	if len(id) > 64 {
		t.Errorf("DefaultRequestID exceeds spec max 64 (%d)", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("DefaultRequestID is not hex: %v", err)
	}
}

func TestDefaultRequestIDUnique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := DefaultRequestID()
		if _, dup := seen[id]; dup {
			t.Fatalf("collision after %d ids: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestDefaultRequestIDLowercaseHex(t *testing.T) {
	id := DefaultRequestID()
	if id != strings.ToLower(id) {
		t.Errorf("DefaultRequestID is not lowercase: %q", id)
	}
}
