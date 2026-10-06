package twofactor_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/service/twofactor"
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

// testHotp is an independent RFC 4226 implementation: HMAC-SHA1 over the
// 8-byte big-endian counter, truncated to 6 digits. The tests pin the
// production hotp against this reference implementation and the RFC vectors.
func testHotp(key []byte, counter int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	code := (binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff) % 1_000_000
	return fmt.Sprintf("%06d", code)
}

func TestVerifyCodeRFC6238Vectors(t *testing.T) {
	for unix, want := range rfcVectors {
		now := time.Unix(unix, 0)
		if got := testHotp(mustB32(t, rfcSecret), now.Unix()/twofactor.Step); got != want {
			t.Errorf("testHotp(counter=%d) = %s, want %s", now.Unix()/twofactor.Step, got, want)
		}
		if !twofactor.VerifyCode(rfcSecret, want, now) {
			t.Errorf("VerifyCode rejected known-answer code %s at T=%d", want, unix)
		}
	}
}

func TestVerifyCodeWindow(t *testing.T) {
	// Code generated at T=59 (step 1) must verify within ±1 step.
	code := rfcVectors[59]
	for _, nowUnix := range []int64{59 - twofactor.Step, 59, 59 + twofactor.Step} {
		if !twofactor.VerifyCode(rfcSecret, code, time.Unix(nowUnix, 0)) {
			t.Errorf("code %s rejected at T=%d, want accepted within window", code, nowUnix)
		}
	}
	// Two steps away is outside the window.
	if twofactor.VerifyCode(rfcSecret, code, time.Unix(59+2*twofactor.Step, 0)) {
		t.Errorf("code %s accepted two steps away, want rejected", code)
	}
}

func TestVerifyCodeRejectsGarbage(t *testing.T) {
	if twofactor.VerifyCode("not-base32!!", "123456", time.Now()) {
		t.Error("invalid secret accepted")
	}
	if twofactor.VerifyCode(rfcSecret, "", time.Now()) || twofactor.VerifyCode(rfcSecret, "abc", time.Now()) {
		t.Error("invalid code accepted")
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := twofactor.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := twofactor.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("two secrets are identical")
	}
	raw := mustB32(t, a)
	if len(raw) != twofactor.SecretLength {
		t.Errorf("secret decodes to %d bytes, want %d", len(raw), twofactor.SecretLength)
	}
	if regexp.MustCompile(`^[A-Z2-7]+$`).MatchString(a) == false {
		t.Errorf("secret %q is not base32 (no padding)", a)
	}
}

func TestProvisioningURI(t *testing.T) {
	got := twofactor.ProvisioningURI("Baaku App", "user@example.com", "SECRET123")
	want := "otpauth://totp/BaakuApp:user@example.com?secret=SECRET123&issuer=BaakuApp"
	if got != want {
		t.Errorf("ProvisioningURI = %q, want %q", got, want)
	}
}

func TestNewRecoveryCodes(t *testing.T) {
	codes, err := twofactor.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != twofactor.CodeCount {
		t.Fatalf("got %d codes, want %d", len(codes), twofactor.CodeCount)
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
	enc, err := twofactor.Encrypt(key, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := twofactor.Decrypt(key, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Errorf("round trip = %q, want %q", got, plain)
	}
	if _, err := twofactor.Decrypt(bytes.Repeat([]byte{0x99}, 32), enc); err == nil {
		t.Error("decrypt with wrong key succeeded")
	}
	tampered := []byte(enc)
	tampered[len(tampered)-2] ^= 0x1
	if _, err := twofactor.Decrypt(key, string(tampered)); err == nil {
		t.Error("decrypt of tampered ciphertext succeeded")
	}
}

func TestReplaceRecoveryCode(t *testing.T) {
	key := testKey()
	codes, err := twofactor.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := twofactor.EncryptCodes(key, codes)
	if err != nil {
		t.Fatal(err)
	}

	updated, found, err := twofactor.ReplaceRecoveryCode(key, stored, codes[3])
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("known code not found")
	}
	got, err := twofactor.DecryptCodes(key, updated)
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

	unchanged, found, err := twofactor.ReplaceRecoveryCode(key, stored, "deadbeef-deadbeef")
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
	codes, err := twofactor.DecryptCodes(testKey(), "")
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
