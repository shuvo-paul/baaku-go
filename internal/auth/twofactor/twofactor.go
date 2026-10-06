// Package twofactor implements TOTP two-factor authentication, porting the
// behavior of Laravel Fortify v1.39 (reference submodule): secret generation
// via PragmaRX Google2FA defaults, encrypted secrets and recovery codes,
// otpauth:// provisioning URIs, and consume-once recovery codes.
package twofactor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Constants mirror Fortify/Google2FA defaults.
const (
	// SecretLength is the Google2FA generateSecretKey default
	// (fortify-options.two-factor-authentication.secret-length, 16).
	SecretLength = 16
	// CodeCount is the number of recovery codes generated on enable.
	CodeCount = 8
	// Window is the Google2FA default ±step window for code verification.
	Window = 1
	// Step is the RFC 6238 time-step in seconds (Google2FA default).
	Step = 30

	alnum = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new base32 (RFC 4648, no padding) TOTP secret of
// SecretLength random bytes, matching Google2FA generateSecretKey(16).
func GenerateSecret() (string, error) {
	var b [SecretLength]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("twofactor: generate secret: %w", err)
	}
	return b32.EncodeToString(b[:]), nil
}

// VerifyCode reports whether code is a valid TOTP for secret at now, within
// ±Window steps (Google2FA verifyKey default window).
func VerifyCode(secret, code string, now time.Time) bool {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return false
	}
	code = strings.TrimSpace(code)
	step := now.Unix() / Step
	for i := -Window; i <= Window; i++ {
		if hotp(key, step+int64(i)) == code {
			return true
		}
	}
	return false
}

// hotp computes the 6-digit RFC 4226 code for counter (RFC 6238 with SHA-1,
// 30s period, 6 digits — Google2FA defaults).
func hotp(key []byte, counter int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}

// ProvisioningURI builds the otpauth:// authenticator-app URI, mirroring
// Google2FA::getQRCodeUrl: spaces stripped from the app name, label
// "AppName:email", query secret + issuer.
func ProvisioningURI(appName, email, secret string) string {
	appName = strings.ReplaceAll(appName, " ", "")
	return "otpauth://totp/" + appName + ":" + email + "?secret=" + secret + "&issuer=" + appName
}

// NewRecoveryCode returns one recovery code in Fortify RecoveryCode::generate
// format: 10 alphanumerics, dash, 10 alphanumerics.
func NewRecoveryCode() (string, error) {
	a, err := randomAlnum(10)
	if err != nil {
		return "", err
	}
	b, err := randomAlnum(10)
	if err != nil {
		return "", err
	}
	return a + "-" + b, nil
}

// NewRecoveryCodes returns CodeCount fresh recovery codes.
func NewRecoveryCodes() ([]string, error) {
	codes := make([]string, CodeCount)
	for i := range codes {
		c, err := NewRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes[i] = c
	}
	return codes, nil
}

func randomAlnum(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("twofactor: recovery code: %w", err)
	}
	for i := range b {
		b[i] = alnum[int(b[i])%len(alnum)]
	}
	return string(b), nil
}

// Encrypt encrypts plaintext with AES-256-GCM under key (the decoded 32-byte
// APP_KEY) and returns base64(nonce || ciphertext || tag).
func Encrypt(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("twofactor: encrypt: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. It fails on tampering or a wrong key.
func Decrypt(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("twofactor: decrypt: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("twofactor: decrypt: ciphertext too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("twofactor: decrypt: %w", err)
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("twofactor: APP_KEY must be 16, 24, or 32 bytes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("twofactor: new GCM: %w", err)
	}
	return gcm, nil
}

// EncryptCodes encrypts the JSON array of recovery codes for storage
// (Fortify stores encrypt(json_encode($codes))).
func EncryptCodes(key []byte, codes []string) (string, error) {
	raw, err := json.Marshal(codes)
	if err != nil {
		return "", err
	}
	return Encrypt(key, string(raw))
}

// DecryptCodes reverses EncryptCodes; a missing blob yields no codes.
func DecryptCodes(key []byte, stored string) ([]string, error) {
	if stored == "" {
		return nil, nil
	}
	plain, err := Decrypt(key, stored)
	if err != nil {
		return nil, err
	}
	var codes []string
	if err := json.Unmarshal([]byte(plain), &codes); err != nil {
		return nil, err
	}
	return codes, nil
}

// ReplaceRecoveryCode consumes a used recovery code the Fortify way
// (TwoFactorAuthenticatable::replaceRecoveryCode): the matched code is
// swapped in place for a fresh one and the blob re-encrypted. found is false
// when code is not in the stored list; stored is returned unchanged then.
func ReplaceRecoveryCode(key []byte, stored, code string) (updated string, found bool, err error) {
	codes, err := DecryptCodes(key, stored)
	if err != nil {
		return "", false, err
	}
	for i, c := range codes {
		if hmac.Equal([]byte(c), []byte(code)) {
			fresh, err := NewRecoveryCode()
			if err != nil {
				return "", false, err
			}
			codes[i] = fresh
			out, err := EncryptCodes(key, codes)
			if err != nil {
				return "", false, err
			}
			return out, true, nil
		}
	}
	return stored, false, nil
}
