package session_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/service/session"
)

func TestCookie(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.Session
		id           string
		wantName     string
		wantSameSite http.SameSite
		wantSecure   bool
		wantHTTPOnly bool
	}{
		{
			name:         "defaults",
			cfg:          config.Session{Cookie: "baaku-session", Lifetime: 120, HttpOnly: true, SameSite: "lax"},
			id:           "abc123",
			wantName:     "baaku-session",
			wantSameSite: http.SameSiteLaxMode,
			wantHTTPOnly: true,
		},
		{
			name:         "secure and strict",
			cfg:          config.Session{Cookie: "sid", Lifetime: 30, Secure: true, HttpOnly: true, SameSite: "strict"},
			id:           "xyz",
			wantName:     "sid",
			wantSameSite: http.SameSiteStrictMode,
			wantSecure:   true,
			wantHTTPOnly: true,
		},
		{
			name:         "same_site none",
			cfg:          config.Session{Cookie: "sid", Lifetime: 60, SameSite: "None"},
			id:           "n",
			wantName:     "sid",
			wantSameSite: http.SameSiteNoneMode,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			c := session.Cookie(tt.cfg, tt.id)
			if c.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", c.Name, tt.wantName)
			}
			if c.Value != tt.id {
				t.Errorf("Value = %q, want %q", c.Value, tt.id)
			}
			if c.Path != "/" {
				t.Errorf("Path = %q, want /", c.Path)
			}
			if c.MaxAge != tt.cfg.Lifetime*60 {
				t.Errorf("MaxAge = %d, want %d", c.MaxAge, tt.cfg.Lifetime*60)
			}
			if !c.HttpOnly != !tt.wantHTTPOnly {
				t.Errorf("HttpOnly = %v, want %v", c.HttpOnly, tt.wantHTTPOnly)
			}
			if c.Secure != tt.wantSecure {
				t.Errorf("Secure = %v, want %v", c.Secure, tt.wantSecure)
			}
			if c.SameSite != tt.wantSameSite {
				t.Errorf("SameSite = %v, want %v", c.SameSite, tt.wantSameSite)
			}
			wantExp := before.Add(time.Duration(tt.cfg.Lifetime) * time.Minute)
			if c.Expires.Before(wantExp.Add(-time.Second)) || c.Expires.After(wantExp.Add(time.Second+2*time.Second)) {
				t.Errorf("Expires = %v, want ~%v", c.Expires, wantExp)
			}
		})
	}
}
