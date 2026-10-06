package session

import (
	"encoding/hex"
	"testing"
)

func TestNewSessionID(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id, err := NewSessionID()
		if err != nil {
			t.Fatal(err)
		}
		if len(id) != 64 {
			t.Fatalf("len = %d, want 64 (32 bytes hex)", len(id))
		}
		b, err := hex.DecodeString(id)
		if err != nil {
			t.Fatalf("not valid hex: %v", err)
		}
		if len(b) != 32 {
			t.Fatalf("decoded = %d bytes, want 32", len(b))
		}
		if seen[id] {
			t.Fatalf("duplicate session ID %q", id)
		}
		seen[id] = true
	}
}
