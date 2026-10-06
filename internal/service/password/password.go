// Package auth provides password hashing, validation, and comparison for
// service packages (register, login, passwordreset). Validation mirrors the
// Password::min(8)->letters()->mixedCase()->numbers()->symbols() rules.
package password

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordUnconfirmed is returned by Confirmed when raw != confirm.
var ErrPasswordUnconfirmed = errors.New("auth: password confirmation does not match")

// Hash returns the bcrypt hash of raw.
func Hash(raw string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash password: %w", err)
	}
	return string(h), nil
}

// Compare reports whether raw matches the bcrypt hash h.
func Compare(h, raw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(raw)) == nil
}

// Validate checks raw against the mirrored Laravel password rules:
// minimum 8 characters (unicode-aware), at least one letter, mixed case,
// one number, one symbol. It returns a single error listing every failed
// rule.
func Validate(raw string) error {
	var failed []string

	if len([]rune(raw)) < 8 {
		failed = append(failed, "min 8 characters")
	}

	var hasLetter, hasUpper, hasLower, hasNumber, hasSymbol bool
	for _, r := range raw {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
			hasUpper = hasUpper || unicode.IsUpper(r)
			hasLower = hasLower || unicode.IsLower(r)
		case r >= '0' && r <= '9': // Laravel \d is ASCII-only
			hasNumber = true
		case !unicode.IsSpace(r):
			hasSymbol = true
		}
	}

	if !hasLetter {
		failed = append(failed, "at least one letter")
	}
	if !hasUpper || !hasLower {
		failed = append(failed, "mixed case")
	}
	if !hasNumber {
		failed = append(failed, "at least one number")
	}
	if !hasSymbol {
		failed = append(failed, "at least one symbol")
	}

	if len(failed) > 0 {
		return fmt.Errorf("auth: password rules failed: %s", strings.Join(failed, "; "))
	}
	return nil
}

// Confirmed checks that raw and confirm match.
func Confirmed(raw, confirm string) error {
	if raw != confirm {
		return ErrPasswordUnconfirmed
	}
	return nil
}
