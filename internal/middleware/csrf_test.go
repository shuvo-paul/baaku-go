package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
)

// csrfKey is a 32-byte APP_KEY stand-in.
var csrfKey = []byte("0123456789abcdef0123456789abcdef")

func sealedToken(t *testing.T, token string) string {
	t.Helper()
	sealed, err := twofactor.Encrypt(csrfKey, token)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return sealed
}

func cookieJar(cookies []*http.Cookie) map[string]*http.Cookie {
	m := map[string]*http.Cookie{}
	for _, c := range cookies {
		m[c.Name] = c
	}
	return m
}

func TestCSRFGetSetsEncryptedCookie(t *testing.T) {
	var called bool
	h := middleware.CSRF(csrfKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if !called {
		t.Fatal("next not called on GET")
	}
	c := cookieJar(rec.Result().Cookies())[middleware.XSRFCookieName]
	if c == nil {
		t.Fatal("XSRF-TOKEN cookie not set")
	}
	if c.HttpOnly {
		t.Error("XSRF-TOKEN must be JS-readable (not HttpOnly)")
	}
	token, err := twofactor.Decrypt(csrfKey, c.Value)
	if err != nil || token == "" {
		t.Fatalf("cookie value not decryptable with app key: %v", err)
	}
}

func TestCSRFMutatingRequests(t *testing.T) {
	const token = "plaintext-token"
	cookie := sealedToken(t, token)

	tests := []struct {
		name      string
		method    string
		path      string
		cookieVal string
		header    string
		form      url.Values
		exempt    []string
		wantCode  int
		wantNext  bool
	}{
		{
			name:      "matching X-XSRF-TOKEN header passes",
			method:    http.MethodPost,
			path:      "/login",
			cookieVal: cookie,
			header:    cookie, // client echoes the cookie value, axios-style
			wantCode:  http.StatusOK,
			wantNext:  true,
		},
		{
			name:      "missing header is 419",
			method:    http.MethodPost,
			path:      "/login",
			cookieVal: cookie,
			wantCode:  status419,
		},
		{
			name:      "header for a different token is 419",
			method:    http.MethodPost,
			path:      "/login",
			cookieVal: cookie,
			header:    sealedToken(t, "attacker-guess"),
			wantCode:  status419,
		},
		{
			name:      "tampered header blob is 419",
			method:    http.MethodPost,
			path:      "/login",
			cookieVal: cookie,
			header:    cookie[:len(cookie)-4] + "AAAA",
			wantCode:  status419,
		},
		{
			name:      "plaintext _token form field passes",
			method:    http.MethodPost,
			path:      "/register",
			cookieVal: cookie,
			form:      url.Values{"_token": {token}},
			wantCode:  http.StatusOK,
			wantNext:  true,
		},
		{
			name:      "wrong _token form field is 419",
			method:    http.MethodPost,
			path:      "/register",
			cookieVal: cookie,
			form:      url.Values{"_token": {"nope"}},
			wantCode:  status419,
		},
		{
			name:     "no cookie is 419",
			method:   http.MethodPost,
			path:     "/login",
			header:   cookie,
			wantCode: status419,
		},
		{
			name:      "unparseable cookie is 419",
			method:    http.MethodPost,
			path:      "/login",
			cookieVal: "garbage-not-encrypted",
			header:    "garbage-not-encrypted",
			wantCode:  status419,
		},
		{
			name:      "PUT without header is 419",
			method:    http.MethodPut,
			path:      "/user/profile",
			cookieVal: cookie,
			wantCode:  status419,
		},
		{
			name:      "PATCH without header is 419",
			method:    http.MethodPatch,
			path:      "/user/profile",
			cookieVal: cookie,
			wantCode:  status419,
		},
		{
			name:      "DELETE without header is 419",
			method:    http.MethodDelete,
			path:      "/user/profile",
			cookieVal: cookie,
			wantCode:  status419,
		},
		{
			name:      "exempt path skips the check",
			method:    http.MethodPost,
			path:      "/webhooks/stripe",
			cookieVal: cookie,
			exempt:    []string{"/webhooks/*"},
			wantCode:  http.StatusOK,
			wantNext:  true,
		},
		{
			name:      "non-exempt sibling path still checked",
			method:    http.MethodPost,
			path:      "/webhooks-stripe",
			cookieVal: cookie,
			exempt:    []string{"/webhooks/*"},
			wantCode:  status419,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			h := middleware.CSRF(csrfKey, tt.exempt...)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			var body string
			if tt.form != nil {
				body = tt.form.Encode()
			}
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(body))
			if tt.form != nil {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			if tt.cookieVal != "" {
				req.AddCookie(&http.Cookie{Name: middleware.XSRFCookieName, Value: tt.cookieVal})
			}
			if tt.header != "" {
				req.Header.Set(middleware.XSRFHeaderName, tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
		})
	}
}

// status419 mirrors Laravel's "Page Expired" CSRF failure.
const status419 = 419
