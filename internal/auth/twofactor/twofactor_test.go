package twofactor

import (
	"bytes"
	"encoding/base32"
	"regexp"
	"testing"
	"time"
)

// rfcSecret is the base32 form of the RFC 6238 test-secret ASCII
// "12345678901234567890".
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// rfcVectors pins RFC 6238 Appendix B SHA-1 vectors, truncated to the
// 6-digit codes Google2FA/Fortify issue (last 6 digits of the 8-digit codes).
var rfcVectors = map[int64]string{
	59:          "287082", // RFC 6238 vector 94287082
	1111111109:  "081804", // 07081804
	1111111111:  "050471", // 14050471
	1234567890:  "005924", // 89005924
	2000000000:  "279037", // 69279037
	20000000000: "353130", // 65353130
}

func TestVerifyCodeRFC6238Vectors(t *testing.T) {
	for unix, want := range rfcVectors {
		now := time.Unix(unix, 0)
		if got := hotp(mustB32(t, rfcSecret), now.Unix()/Step); got != want {
			t.Errorf("hotp(counter=%d) = %s, want %s", now.Unix()/Step, got, want)
		}
		if !VerifyCode(rfcSecret, want, now) {
			t.Errorf("VerifyCode rejected known-answer code %s at T=%d", want, unix)
		}
	}
}

func TestVerifyCodeWindow(t *testing.T) {
	// Code generated at T=59 (step 1) must verify within ±1 step.
	code := rfcVectors[59]
	for _, nowUnix := range []int64{59 - Step, 59, 59 + Step} {
		if !VerifyCode(rfcSecret, code, time.Unix(nowUnix, 0)) {
			t.Errorf("code %s rejected at T=%d, want accepted within window", code, nowUnix)
		}
	}
	// Two steps away is outside the window.
	if VerifyCode(rfcSecret, code, time.Unix(59+2*Step, 0)) {
		t.Errorf("code %s accepted two steps away, want rejected", code)
	}
}

func TestVerifyCodeRejectsGarbage(t *testing.T) {
	if VerifyCode("not-base32!!", "123456", time.Now()) {
		t.Error("invalid secret accepted")
	}
	if VerifyCode(rfcSecret, "", time.Now()) || VerifyCode(rfcSecret, "abc", time.Now()) {
		t.Error("invalid code accepted")
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two secrets are identical")
	}
	raw := mustB32(t, a)
	if len(raw) != SecretLength {
		t.Errorf("secret decodes to %d bytes, want %d", len(raw), SecretLength)
	}
	if regexp.MustCompile(`^[A-Z2-7]+$`).MatchString(a) == false {
		t.Errorf("secret %q is not base32 (no padding)", a)
	}
}

func TestProvisioningURI(t *testing.T) {
	got := ProvisioningURI("Baaku App", "user@example.com", "SECRET123")
	want := "otpauth://totp/BaakuApp:user@example.com?secret=SECRET123&issuer=BaakuApp"
	if got != want {
		t.Errorf("ProvisioningURI = %q, want %q", got, want)
	}
}

func TestNewRecoveryCodes(t *testing.T) {
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != CodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), CodeCount)
	}
	format := regexp.MustCompile(`^[A-Za-z0-9]{10}-[A-Za-z0-9]{10}$`)
	seen := map[string]bool{}
	for _, c := range codes {
		if !format.MatchString(c) {
			t.Errorf("code %q does not match Fortify RecoveryCode::generate format", c)
		}
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := testKey()
	const plain = `["aaaa1111-bbbb2222"]`
	enc, err := Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decrypt(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("round trip = %q, want %q", got, plain)
	}
	if _, err := Decrypt(bytes.Repeat([]byte{0x99}, 32), enc); err == nil {
		t.Error("decrypt with wrong key succeeded")
	}
	tampered := []byte(enc)
	tampered[len(tampered)-2] ^= 0x1
	if _, err := Decrypt(key, string(tampered)); err == nil {
		t.Error("decrypt of tampered ciphertext succeeded")
	}
}

func TestReplaceRecoveryCode(t *testing.T) {
	key := testKey()
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := EncryptCodes(key, codes)
	if err != nil {
		t.Fatal(err)
	}

	updated, found, err := ReplaceRecoveryCode(key, stored, codes[3])
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("known code not found")
	}
	got, err := DecryptCodes(key, updated)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(codes) {
		t.Fatalf("codes consumed: %d remain, want %d (Fortify replaces, not removes)", len(got), len(codes))
	}
	for i, c := range got {
		if i == 3 && c == codes[3] {
			t.Error("used code was not replaced")
		}
		if i != 3 && c != codes[i] {
			t.Errorf("code %d changed: %q, want %q", i, c, codes[i])
		}
	}

	unchanged, found, err := ReplaceRecoveryCode(key, stored, "deadbeef-deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("unknown code reported found")
	}
	if unchanged != stored {
		t.Error("stored blob changed for unknown code")
	}
}

func TestDecryptCodesEmpty(t *testing.T) {
	codes, err := DecryptCodes(testKey(), "")
	if err != nil || codes != nil {
		t.Errorf("DecryptCodes(\"\") = %v, %v; want nil, nil", codes, err)
	}
}

func mustB32(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		t.Fatalf("decode base32 %q: %v", s, err)
	}
	return raw
}

func testKey() []byte {
	return bytes.Repeat([]byte{0x42}, 32)
}
