package session

import (
	"crypto/rand"
	"encoding/hex"
)

// NewSessionID returns a 32-byte random hex string, matching Laravel's
// session ID format (64 hex chars).
func NewSessionID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
