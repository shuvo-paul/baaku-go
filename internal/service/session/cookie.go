package session

import (
	"net/http"
	"strings"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
)

// Cookie returns the session cookie carrying id, per the Laravel session
// config: name, lifetime, HttpOnly, SameSite, and Secure.
func Cookie(cfg config.Session, id string) *http.Cookie {
	return &http.Cookie{
		Name:     cfg.Cookie,
		Value:    id,
		Path:     "/",
		MaxAge:   cfg.Lifetime * 60,
		Expires:  time.Now().Add(time.Duration(cfg.Lifetime) * time.Minute),
		HttpOnly: cfg.HttpOnly,
		Secure:   cfg.Secure,
		SameSite: sameSite(cfg.SameSite),
	}
}

func sameSite(s string) http.SameSite {
	switch strings.ToLower(s) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default: // "lax" per Laravel
		return http.SameSiteLaxMode
	}
}
