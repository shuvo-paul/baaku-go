package password_test

import (
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/password"
)

func TestValidate(t *testing.T) {
	valid := []string{
		"Passw0rd!",
		"hunter2!A",
		"密码A1b!cд", // unicode letters count, 8 runes
	}

	invalid := []string{
		"",              // empty
		"short1!",       // too short
		"password1",     // no symbol
		"PASSWORD!",     // no lowercase
		"password!",     // no number
		"Passw0rd",      // no symbol
		"nonumbers!!",   // no number
		"nosymbols1234", // no symbol
		"12345678",      // no letter
		"        ",      // whitespace only
	}

	for _, pw := range valid {
		if err := password.Validate(pw); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", pw, err)
		}
	}
	for _, pw := range invalid {
		if err := password.Validate(pw); err == nil {
			t.Errorf("Validate(%q) = nil, want error", pw)
		}
	}
}

func TestHashCompare(t *testing.T) {
	h, err := password.Hash("Passw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	if !password.Compare(h, "Passw0rd!") {
		t.Error("Compare should accept original password")
	}
	if password.Compare(h, "wrong") {
		t.Error("Compare should reject wrong password")
	}
}

func TestConfirmed(t *testing.T) {
	if err := password.Confirmed("a", "a"); err != nil {
		t.Errorf("Confirmed match = %v, want nil", err)
	}
	if err := password.Confirmed("a", "b"); err != password.ErrPasswordUnconfirmed {
		t.Errorf("Confirmed mismatch = %v, want ErrPasswordUnconfirmed", err)
	}
}
