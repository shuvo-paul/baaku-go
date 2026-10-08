package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/middleware"
)

func TestLoginThrottle(t *testing.T) {
	var hits int
	h := middleware.LoginThrottle(3, time.Minute)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
	}))
	post := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = ip + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 1; i <= 3; i++ {
		if rec := post("203.0.113.9"); rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want 200", i, rec.Code)
		}
	}
	rec := post("203.0.113.9")
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("over-limit: status = %d, want 429", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Too many login attempts") {
		t.Errorf("body = %q, want throttle message", rec.Body.String())
	}
	if hits != 3 {
		t.Errorf("handler called %d times, want 3 (429 short-circuits)", hits)
	}
	// A different client IP is unaffected.
	if rec := post("198.51.100.7"); rec.Code != http.StatusOK {
		t.Errorf("other IP: status = %d, want 200", rec.Code)
	}
}
