// CSRF middleware mirroring Laravel VerifyCsrfToken for the reference app:
//
//   - XSRF-TOKEN cookie holds Encrypt(APP_KEY, token); set on any request
//     that arrives without one (JS must be able to read it → not HttpOnly).
//   - Mutating requests (POST/PUT/PATCH/DELETE) must prove knowledge of the
//     token via the X-XSRF-TOKEN header (the cookie value echoed back, as
//     axios does — decrypted server-side and compared) or a _token form field
//     (plaintext token, what @csrf blades emit). Failure → 419.
//
// Cross-site attackers cannot read the cookie, so they cannot forge either
// proof. Exempt paths skip the check (Laravel $except hook).
package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/shuvo-paul/baaku/internal/service/twofactor"
)

const (
	// XSRFCookieName matches Laravel's XSRF-TOKEN cookie.
	XSRFCookieName = "XSRF-TOKEN"
	// XSRFHeaderName matches Laravel's X-XSRF-TOKEN header.
	XSRFHeaderName = "X-XSRF-TOKEN"
	// statusCSRFMismatch is Laravel's "Page Expired" (TokenMismatchException).
	statusCSRFMismatch = 419
)

// CSRF validates the XSRF proof on mutating requests using key (APP_KEY
// bytes) to decrypt the token. exempt lists paths exempt from the check;
// "prefix/*" matches a subtree, exact strings match one path.
func CSRF(key []byte, exempt ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mutating(r.Method) || exemptPath(r.URL.Path, exempt) {
				ensureTokenCookie(w, r, key)
				next.ServeHTTP(w, r)
				return
			}
			if !validToken(r, key) {
				http.Error(w, "CSRF token mismatch", statusCSRFMismatch)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// validToken reports whether the request proves knowledge of the token inside
// the XSRF-TOKEN cookie: an echoed X-XSRF-TOKEN header (decrypted, so a
// tampered blob fails too) or a plaintext _token form field.
func validToken(r *http.Request, key []byte) bool {
	c, err := r.Cookie(XSRFCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	token, err := twofactor.Decrypt(key, c.Value)
	if err != nil || token == "" {
		return false
	}
	if header := r.Header.Get(XSRFHeaderName); header != "" {
		want, err := twofactor.Decrypt(key, header)
		return err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
	}
	if field := r.FormValue("_token"); field != "" {
		return subtle.ConstantTimeCompare([]byte(field), []byte(token)) == 1
	}
	return false
}

// ensureTokenCookie sets an encrypted XSRF-TOKEN cookie when the request
// carries none. Minting on every request is unnecessary — the cookie persists
// until cleared.
func ensureTokenCookie(w http.ResponseWriter, r *http.Request, key []byte) {
	if c, err := r.Cookie(XSRFCookieName); err == nil && c.Value != "" {
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return // crypto/rand failure: request proceeds untokenized
	}
	sealed, err := twofactor.Encrypt(key, hex.EncodeToString(raw))
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     XSRFCookieName,
		Value:    sealed,
		Path:     "/",
		HttpOnly: false, // JS reads it to populate X-XSRF-TOKEN
		SameSite: http.SameSiteLaxMode,
	})
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

func exemptPath(path string, exempt []string) bool {
	for _, p := range exempt {
		if p == path {
			return true
		}
		if strings.HasSuffix(p, "/*") && strings.HasPrefix(path, strings.TrimSuffix(p, "*")) {
			return true
		}
	}
	return false
}
