package handler_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/service/login"
	sesssvc "github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
)

const (
	testCookie = "baaku-session"
	csrfToken  = "test-csrf-token"
)

var csrfKey = []byte("0123456789abcdef0123456789abcdef")

var errBoom = errors.New("store down")
var testCfg = config.Session{Cookie: testCookie, Lifetime: 120, HttpOnly: true, SameSite: "lax"}

// fakes

type fakeLogin struct {
	res           login.LoginResult
	err           error
	loginEmail    string
	loginPassword string
	loginRemember bool
	logoutCalls   []string
	logoutUserID  *int64
}

func (f *fakeLogin) Login(_ context.Context, email, password string, remember bool) (login.LoginResult, error) {
	f.loginEmail, f.loginPassword, f.loginRemember = email, password, remember
	return f.res, f.err
}

func (f *fakeLogin) Logout(_ context.Context, sessionID string, userID *int64) error {
	f.logoutCalls = append(f.logoutCalls, sessionID)
	f.logoutUserID = userID
	return nil
}

type fakePending struct {
	id            string
	err           error
	asked         []int64
	askedRemember []bool
}

func (f *fakePending) Begin(_ context.Context, userID int64, remember bool) (string, error) {
	f.asked = append(f.asked, userID)
	f.askedRemember = append(f.askedRemember, remember)
	return f.id, f.err
}

// fakeSessions satisfies the session middleware's Load/Save port.
type fakeSessions struct {
	byID map[string]generated.Session
}

func (f *fakeSessions) Load(_ context.Context, id string) (generated.Session, error) {
	s, ok := f.byID[id]
	if !ok {
		return generated.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Save(_ context.Context, p generated.UpsertSessionParams) error {
	f.byID[p.ID] = generated.Session{
		ID: p.ID, UserID: p.UserID, Payload: p.Payload, LastActivity: p.LastActivity,
	}
	return nil
}

// harness

func newRouter(h *handler.Login, store *fakeSessions) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, store))
	r.Use(middleware.CSRF(csrfKey))
	r.Get("/login", h.ShowLogin)
	r.Post("/login", h.Login)
	r.Post("/logout", h.Logout)
	return r
}

func sealedCSRF(t *testing.T) string {
	t.Helper()
	sealed, err := twofactor.Encrypt(csrfKey, csrfToken)
	if err != nil {
		t.Fatalf("encrypt csrf: %v", err)
	}
	return sealed
}

func get(t *testing.T, router http.Handler, path, sessID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if sessID != "" {
		req.AddCookie(&http.Cookie{Name: testCookie, Value: sessID})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func postForm(t *testing.T, router http.Handler, path string, form url.Values, sessID string) *httptest.ResponseRecorder {
	t.Helper()
	if form == nil {
		form = url.Values{}
	}
	form.Set("_token", csrfToken)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: middleware.XSRFCookieName, Value: sealedCSRF(t)})
	if sessID != "" {
		req.AddCookie(&http.Cookie{Name: testCookie, Value: sessID})
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == testCookie {
			return c
		}
	}
	return nil
}

// tests

func TestLoginShowRendersFormWithToken(t *testing.T) {
	router := newRouter(handler.NewLogin(&fakeLogin{}, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := get(t, router, "/login", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `action="/login"`) {
		t.Errorf("body missing form action: %s", body)
	}
	if !strings.Contains(body, `name="_token"`) {
		t.Errorf("body missing _token field: %s", body)
	}
	// The rendered _token must equal the plaintext inside the XSRF cookie.
	c := cookieJar(rec.Result().Cookies())[middleware.XSRFCookieName]
	if c == nil {
		t.Fatal("XSRF-TOKEN cookie not set")
	}
	plain, err := twofactor.Decrypt(csrfKey, c.Value)
	if err != nil || plain == "" {
		t.Fatalf("cookie not decryptable: %v", err)
	}
	if !strings.Contains(body, `value="`+plain+`"`) {
		t.Errorf("_token value does not match cookie plaintext %q: %s", plain, body)
	}
}

func TestLoginShowAuthenticatedRedirectsToDashboard(t *testing.T) {
	uid := int64(42)
	store := &fakeSessions{byID: map[string]generated.Session{
		"s1": {ID: "s1", UserID: &uid},
	}}
	router := newRouter(handler.NewLogin(&fakeLogin{}, &fakePending{}, testCfg, "Baaku", csrfKey), store)

	rec := get(t, router, "/login", "s1")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != middleware.DashboardPath {
		t.Errorf("Location = %q, want %q", loc, middleware.DashboardPath)
	}
}

func TestLoginSuccessSetsSessionCookie(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 42, SessionID: "sid-1"}}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"hunter2!"}}, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != middleware.DashboardPath {
		t.Errorf("Location = %q, want %q", loc, middleware.DashboardPath)
	}
	if auth.loginEmail != "a@b.c" || auth.loginPassword != "hunter2!" {
		t.Errorf("service got (%q, %q), want (a@b.c, hunter2!)", auth.loginEmail, auth.loginPassword)
	}
	if c := sessionCookie(t, rec); c == nil || c.Value != "sid-1" {
		t.Errorf("session cookie = %+v, want value sid-1", c)
	}
}

func TestLoginRememberQueuesRecallerCookie(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 42, SessionID: "sid-1", RememberToken: "tok60", PasswordHash: "hash"}}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"hunter2!"}, "remember": {"1"}}, "")
	if !auth.loginRemember {
		t.Error("remember checkbox not forwarded to the service")
	}
	want := sesssvc.RecallerValue(csrfKey, 42, "tok60", "hash")
	for _, c := range rec.Result().Cookies() {
		if c.Name == sesssvc.RecallerName() {
			if c.Value != want {
				t.Errorf("recaller value = %q, want %q", c.Value, want)
			}
			return
		}
	}
	t.Error("no recaller cookie queued on remembered login")
}

func TestLoginWithoutRememberQueuesNoRecaller(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 42, SessionID: "sid-1"}}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"hunter2!"}}, "")
	if auth.loginRemember {
		t.Error("remember requested without the checkbox")
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sesssvc.RecallerName() {
			t.Errorf("recaller cookie queued on plain login: %+v", c)
		}
	}
}

func TestLogin2FAStashesRememberFlag(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 7, TwoFactorPending: true}}
	pending := &fakePending{id: "pend-1"}
	router := newRouter(handler.NewLogin(auth, pending, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"x"}, "remember": {"1"}}, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != handler.ChallengePath {
		t.Fatalf("got %d -> %q, want challenge redirect", rec.Code, rec.Header().Get("Location"))
	}
	if len(pending.askedRemember) != 1 || !pending.askedRemember[0] {
		t.Errorf("Begin remember flags = %v, want [true]", pending.askedRemember)
	}
}

func TestLoginIntendedRedirect(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantLoc string
	}{
		{"stashed intended URL", `{"url.intended":"/reports"}`, "/reports"},
		{"off-host intended rejected", `{"url.intended":"//evil.example"}`, middleware.DashboardPath},
		{"no intended falls back", `{}`, middleware.DashboardPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeSessions{byID: map[string]generated.Session{
				"s1": {ID: "s1", Payload: tt.payload},
			}}
			auth := &fakeLogin{res: login.LoginResult{UserID: 42, SessionID: "sid-1"}}
			router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), store)

			rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"x"}}, "s1")
			if rec.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302", rec.Code)
			}
			if loc := rec.Header().Get("Location"); loc != tt.wantLoc {
				t.Errorf("Location = %q, want %q", loc, tt.wantLoc)
			}
		})
	}
}

func TestLoginInvalidCredentialsRendersMessage(t *testing.T) {
	auth := &fakeLogin{err: login.ErrInvalidCredentials}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"wrong"}}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, handler.AuthFailedMessage) {
		t.Errorf("body missing auth.failed message: %s", body)
	}
	if !strings.Contains(body, `value="a@b.c"`) {
		t.Errorf("body missing repopulated email: %s", body)
	}
	if sessionCookie(t, rec) != nil {
		t.Error("session cookie set on failed login")
	}
}

func TestLoginValidationMessages(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 1, SessionID: "s"}}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"password": {"x"}}, "")
	if !strings.Contains(rec.Body.String(), "The email field is required.") {
		t.Errorf("missing email-required message: %s", rec.Body.String())
	}
	rec = postForm(t, router, "/login", url.Values{"email": {"a@b.c"}}, "")
	if !strings.Contains(rec.Body.String(), "The password field is required.") {
		t.Errorf("missing password-required message: %s", rec.Body.String())
	}
	if auth.loginEmail != "" {
		t.Error("service called despite validation failure")
	}
}

func TestLoginTwoFactorPendingIssuesPendingSession(t *testing.T) {
	auth := &fakeLogin{res: login.LoginResult{UserID: 7, TwoFactorPending: true}}
	pending := &fakePending{id: "pend-1"}
	router := newRouter(handler.NewLogin(auth, pending, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"x"}}, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != handler.ChallengePath {
		t.Errorf("Location = %q, want %q", loc, handler.ChallengePath)
	}
	// The cookie carries the PENDING session (user_id NULL, per
	// TestBeginCreatesPendingSession), never an authenticated one.
	if c := sessionCookie(t, rec); c == nil || c.Value != "pend-1" {
		t.Errorf("session cookie = %+v, want pending session pend-1", c)
	}
	if len(pending.asked) != 1 || pending.asked[0] != 7 {
		t.Errorf("Begin asked about %v, want [7]", pending.asked)
	}
}

func TestLoginServiceErrorIs500(t *testing.T) {
	router := newRouter(handler.NewLogin(&fakeLogin{err: errBoom}, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/login", url.Values{"email": {"a@b.c"}, "password": {"x"}}, "")
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
}

func TestLogoutDestroysSessionAndClearsCookie(t *testing.T) {
	store := &fakeSessions{byID: map[string]generated.Session{"sid-9": {ID: "sid-9"}}}
	auth := &fakeLogin{}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), store)

	rec := postForm(t, router, "/logout", nil, "sid-9")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != middleware.LoginPath {
		t.Errorf("Location = %q, want %q", loc, middleware.LoginPath)
	}
	if len(auth.logoutCalls) != 1 || auth.logoutCalls[0] != "sid-9" {
		t.Errorf("Logout called with %v, want [sid-9]", auth.logoutCalls)
	}
	c := sessionCookie(t, rec)
	if c == nil {
		t.Fatal("session cookie not cleared")
	}
	if c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("cookie = %+v, want emptied with MaxAge < 0", c)
	}
}

func TestLogoutWithoutSessionStillRedirects(t *testing.T) {
	auth := &fakeLogin{}
	router := newRouter(handler.NewLogin(auth, &fakePending{}, testCfg, "Baaku", csrfKey), &fakeSessions{})

	rec := postForm(t, router, "/logout", nil, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != middleware.LoginPath {
		t.Errorf("Location = %q, want %q", loc, middleware.LoginPath)
	}
	if len(auth.logoutCalls) != 0 {
		t.Errorf("Logout called without a session: %v", auth.logoutCalls)
	}
}

func cookieJar(cookies []*http.Cookie) map[string]*http.Cookie {
	m := map[string]*http.Cookie{}
	for _, c := range cookies {
		m[c.Name] = c
	}
	return m
}
