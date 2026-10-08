// Remember-me ("recaller") cookie support: Laravel's SessionGuard issues a
// long-lived cookie "id|remember_token|hmac(password_hash)" on login with
// remember=true, and re-opens sessions from it when the session is gone.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
)

// RememberDurationMinutes is SessionGuard::$rememberDuration (400 days).
const RememberDurationMinutes = 576000

// RecallerName is Laravel's remember-cookie name:
// remember_web_<sha1(SessionGuard::class)>.
func RecallerName() string {
	sum := sha1.Sum([]byte(`Illuminate\Auth\SessionGuard`))
	return "remember_web_" + hex.EncodeToString(sum[:])
}

// NewRememberToken returns a 60-char random alphanumeric token — Laravel's
// Str::random(60) for users.remember_token.
func NewRememberToken() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	tok := make([]byte, 0, 60)
	var buf [64]byte
	for len(tok) < 60 {
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		for _, c := range buf[:] {
			if int(c) >= 248 { // 62*4: keep the uniform range only
				continue
			}
			tok = append(tok, alphabet[int(c)%len(alphabet)])
			if len(tok) == 60 {
				break
			}
		}
	}
	return string(tok), nil
}

// HashPasswordForCookie HMACs the bcrypt hash for the recaller cookie
// (SessionGuard::hashPasswordForCookie), keyed with the decoded APP_KEY.
func HashPasswordForCookie(key []byte, passwordHash string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(passwordHash))
	return hex.EncodeToString(mac.Sum(nil))
}

// RecallerValue builds the recaller cookie payload:
// <user id>|<remember token>|<password-hash MAC> (queueRecallerCookie).
func RecallerValue(key []byte, userID int64, token, passwordHash string) string {
	return strconv.FormatInt(userID, 10) + "|" + token + "|" + HashPasswordForCookie(key, passwordHash)
}

// ParseRecaller splits a recaller cookie value into its three parts
// (Laravel's Recaller: id, token, password hash — all three required).
func ParseRecaller(value string) (userID int64, token, recallerHash string, ok bool) {
	parts := strings.Split(value, "|")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return 0, "", "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, "", "", false
	}
	return id, parts[1], parts[2], true
}

// RememberCookie queues the long-lived recaller cookie (createRecaller: the
// session cookie's flags, RememberDurationMinutes of validity).
func RememberCookie(cfg config.Session, key []byte, userID int64, token, passwordHash string) *http.Cookie {
	return rememberCookie(cfg, RecallerValue(key, userID, token, passwordHash))
}

// ForgetRememberCookie is the expired recaller cookie Laravel queues on
// logout (CookieJar::forget).
func ForgetRememberCookie(cfg config.Session) *http.Cookie {
	c := rememberCookie(cfg, "")
	c.Value = ""
	c.MaxAge = -1
	c.Expires = time.Unix(0, 0)
	return c
}

func rememberCookie(cfg config.Session, value string) *http.Cookie {
	return &http.Cookie{
		Name:     RecallerName(),
		Value:    value,
		Path:     "/",
		MaxAge:   RememberDurationMinutes * 60,
		Expires:  time.Now().Add(time.Duration(RememberDurationMinutes) * time.Minute),
		HttpOnly: cfg.HttpOnly,
		Secure:   cfg.Secure,
		SameSite: sameSite(cfg.SameSite),
	}
}

// RememberTokenStore persists remember tokens (users.remember_token).
type RememberTokenStore interface {
	SetRememberToken(ctx context.Context, id int64, token *string) error
}

// EnsureRememberToken mirrors SessionGuard::ensureRememberTokenIsSet: keep an
// existing token, otherwise cycle a fresh 60-char one and persist it. Returns
// the token for the recaller cookie.
func EnsureRememberToken(ctx context.Context, store RememberTokenStore, userID int64, current string) (string, error) {
	if current != "" {
		return current, nil
	}
	tok, err := NewRememberToken()
	if err != nil {
		return "", err
	}
	if err := store.SetRememberToken(ctx, userID, &tok); err != nil {
		return "", err
	}
	return tok, nil
}
