// CSRF middleware mirroring Laravel VerifyCsrfToken for the reference app:
//
//   - XSRF-TOKEN cookie holds Encrypt(APP_KEY, token); set on any request
//     that arrives without one (JS must be able to read it → not HttpOnly).
//   - Mutating requests (POST/PUT/PATCH/DELETE) must prove knowledge of the
//     token via the X-XSRF-TOKEN header (the cookie value echoed back, as
//     axios does — decrypted server-side and compared) or a _token form field
//     (plaintext token, what @csrf blades emit). Failure → 419.
//   - The plaintext token is stashed in the request context; handlers read it
//     via TokenFromContext to render _token fields into forms.
//
// Cross-site attackers cannot read the cookie, so they cannot forge either
// proof. Exempt paths skip the check (Laravel $except hook).
package middleware

import (
	"context"
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

// CSRF resolves the request's plaintext token (from the XSRF cookie, or a
// fresh mint when absent/undecryptable), stashes it in the request context,
// and validates the XSRF proof on mutating requests using key (APP_KEY bytes)
// to decrypt header blobs. exempt lists paths exempt from the check;
// "prefix/*" matches a subtree, exact strings match one path.
func CSRF(key []byte, exempt ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := cookieToken(r, key)
			if !ok {
				token = mintToken()
				setTokenCookie(w, key, token)
			}
			ctx := withToken(r.Context(), token)
			if mutating(r.Method) && !exemptPath(r.URL.Path, exempt) && !validToken(r, token, key) {
				http.Error(w, "CSRF token mismatch", statusCSRFMismatch)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

type tokenCtxKey struct{}

func withToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, tokenCtxKey{}, token)
}

// TokenFromContext returns the plaintext CSRF token for this request — the
// value forms must carry in their _token field (or echo in X-XSRF-TOKEN).
func TokenFromContext(ctx context.Context) string {
	tok, _ := ctx.Value(tokenCtxKey{}).(string)
	return tok
}

// cookieToken returns the plaintext token inside the request's XSRF cookie.
func cookieToken(r *http.Request, key []byte) (string, bool) {
	c, err := r.Cookie(XSRFCookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	token, err := twofactor.Decrypt(key, c.Value)
	if err != nil || token == "" {
		return "", false
	}
	return token, true
}

// mintToken returns a fresh 32-byte random token (hex).
func mintToken() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "" // crypto/rand failure: request proceeds untokenized
	}
	return hex.EncodeToString(raw)
}

// setTokenCookie sets the encrypted XSRF-TOKEN cookie. Minting on every
// request is unnecessary — the cookie persists until cleared.
func setTokenCookie(w http.ResponseWriter, key []byte, token string) {
	if token == "" {
		return
	}
	sealed, err := twofactor.Encrypt(key, token)
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

// validToken reports whether the request proves knowledge of token: an echoed
// X-XSRF-TOKEN header (decrypted, so a tampered blob fails too) or a
// plaintext _token form field.
func validToken(r *http.Request, token string, key []byte) bool {
	if header := r.Header.Get(XSRFHeaderName); header != "" {
		want, err := twofactor.Decrypt(key, header)
		return err == nil && subtle.ConstantTimeCompare([]byte(want), []byte(token)) == 1
	}
	if field := r.FormValue("_token"); field != "" {
		return subtle.ConstantTimeCompare([]byte(field), []byte(token)) == 1
	}
	return false
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
