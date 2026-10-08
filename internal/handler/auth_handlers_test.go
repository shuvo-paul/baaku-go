package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/passwordreset"
	"github.com/shuvo-paul/baaku/internal/service/register"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

var testSessCfg = config.Session{Cookie: testCookie, Lifetime: 120, HttpOnly: true, SameSite: "lax"}

// fakes

type fakeRegisterSvc struct {
	in  register.RegisterInput
	out user.User
	err error
}

func (f *fakeRegisterSvc) Register(_ context.Context, in register.RegisterInput) (user.User, error) {
	f.in = in
	return f.out, f.err
}

type fakeOpener struct {
	ids []int64
	sid string
	err error
}

func (f *fakeOpener) CreateSession(_ context.Context, userID int64) (string, error) {
	f.ids = append(f.ids, userID)
	return f.sid, f.err
}

type fakeResender struct {
	ids []int64
}

func (f *fakeResender) Resend(_ context.Context, userID int64) error {
	f.ids = append(f.ids, userID)
	return nil
}

func registerRouter(t *testing.T, svc *fakeRegisterSvc, opener *fakeOpener, resend *fakeResender, store *fakeSessions) http.Handler {
	t.Helper()
	h := handler.NewRegister(svc, opener, resend, testSessCfg, "Baaku")
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, store))
	r.Use(middleware.CSRF(csrfKey))
	r.Get("/register", h.Show)
	r.Post("/register", h.Store)
	return r
}

func validEducationForm() url.Values {
	return url.Values{
		"name":                       {"Ada Lovelace"},
		"email":                      {"ada@example.com"},
		"phone":                      {"01700000000"},
		"password":                   {"Str0ng!Pass"},
		"password_confirmation":      {"Str0ng!Pass"},
		"educations[0][level]":       {"BSc"},
		"educations[0][institution]": {"Example University"},
		"educations[0][subject]":     {"Computing"},
		"educations[0][start_year]":  {"2019"},
		"educations[0][is_current]":  {"1"},
	}
}

// tests

func TestRegisterValidationErrorsRenderJoinedMessage(t *testing.T) {
	svc := &fakeRegisterSvc{err: register.FieldErrors{"email": "The email has already been taken."}}
	router := registerRouter(t, svc, &fakeOpener{}, &fakeResender{}, &fakeSessions{})

	rec := postForm(t, router, "/register", url.Values{"name": {"x"}}, "")
	if !strings.Contains(rec.Body.String(), "The email has already been taken.") {
		t.Errorf("missing field error: %s", rec.Body.String())
	}
}

func TestRegisterSuccessOpensSessionAndResends(t *testing.T) {
	svc := &fakeRegisterSvc{out: user.User{ID: 9, Email: "ada@example.com"}}
	opener := &fakeOpener{sid: "sess-9"}
	resend := &fakeResender{}
	router := registerRouter(t, svc, opener, resend, &fakeSessions{})

	rec := postForm(t, router, "/register", validEducationForm(), "")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != middleware.DashboardPath {
		t.Errorf("Location = %q, want %q", loc, middleware.DashboardPath)
	}
	if c := sessionCookie(t, rec); c == nil || c.Value != "sess-9" {
		t.Errorf("session cookie = %+v, want sess-9", c)
	}
	if len(opener.ids) != 1 || opener.ids[0] != 9 {
		t.Errorf("CreateSession asked about %v, want [9]", opener.ids)
	}
	if len(resend.ids) != 1 || resend.ids[0] != 9 {
		t.Errorf("Resend asked about %v, want [9]", resend.ids)
	}
	// education rows parsed into the service input
	if len(svc.in.Educations) != 1 || svc.in.Educations[0].Level != "BSc" || svc.in.Educations[0].StartYear != 2019 || !svc.in.Educations[0].IsCurrent {
		t.Errorf("educations parsed = %+v", svc.in.Educations)
	}
}

func TestRegisterParseEducationsMultipleRows(t *testing.T) {
	form := validEducationForm()
	form.Set("educations[1][level]", "MSc")
	form.Set("educations[1][institution]", "Other U")
	form.Set("educations[1][subject]", "Maths")
	form.Set("educations[1][start_year]", "2021")
	form.Set("educations[1][end_year]", "2022")

	svc := &fakeRegisterSvc{out: user.User{ID: 1}}
	router := registerRouter(t, svc, &fakeOpener{sid: "s"}, &fakeResender{}, &fakeSessions{})

	postForm(t, router, "/register", form, "")
	if len(svc.in.Educations) != 2 {
		t.Fatalf("got %d education rows, want 2: %+v", len(svc.in.Educations), svc.in.Educations)
	}
	if svc.in.Educations[1].EndYear == nil || *svc.in.Educations[1].EndYear != 2022 {
		t.Errorf("row 1 end_year = %v, want 2022", svc.in.Educations[1].EndYear)
	}
}

// 2FA challenge

type fakeChallenge struct {
	sessionID string
	code      string
	err       error
	res       twofactorchallenge.ChallengeResult
}

func (f *fakeChallenge) Challenge(_ context.Context, sessionID, code string) (twofactorchallenge.ChallengeResult, error) {
	f.sessionID, f.code = sessionID, code
	return f.res, f.err
}

func challengeRouter(t *testing.T, svc *fakeChallenge, store *fakeSessions) http.Handler {
	t.Helper()
	h := handler.NewTwoFactorChallenge(svc, testSessCfg, "Baaku", csrfKey)
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, store))
	r.Use(middleware.CSRF(csrfKey))
	r.Get(handler.ChallengePath, h.Show)
	r.Post(handler.ChallengePath, h.Submit)
	return r
}

func pendingStore() *fakeSessions {
	return &fakeSessions{byID: map[string]generated.Session{
		"pend-1": {ID: "pend-1", Payload: `{"login.two_factor":7}`},
	}}
}

func TestChallengeShowWithoutPendingRedirectsToLogin(t *testing.T) {
	router := challengeRouter(t, &fakeChallenge{}, &fakeSessions{})
	rec := get(t, router, handler.ChallengePath, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != middleware.LoginPath {
		t.Errorf("got %d -> %q, want 302 -> %q", rec.Code, rec.Header().Get("Location"), middleware.LoginPath)
	}
}

func TestChallengeSubmitSuccessPromotesSession(t *testing.T) {
	svc := &fakeChallenge{}
	router := challengeRouter(t, svc, pendingStore())

	rec := postForm(t, router, handler.ChallengePath, url.Values{"code": {"123456"}}, "pend-1")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != middleware.DashboardPath {
		t.Fatalf("got %d -> %q, want 302 -> dashboard", rec.Code, rec.Header().Get("Location"))
	}
	if svc.sessionID != "pend-1" || svc.code != "123456" {
		t.Errorf("Challenge(%q, %q), want (pend-1, 123456)", svc.sessionID, svc.code)
	}
	if c := sessionCookie(t, rec); c == nil || c.Value != "pend-1" {
		t.Errorf("cookie = %+v, want pend-1", c)
	}
}

func TestChallengeSubmitRecoveryCode(t *testing.T) {
	svc := &fakeChallenge{err: twofactor.ErrInvalidCode}
	router := challengeRouter(t, svc, pendingStore())

	rec := postForm(t, router, handler.ChallengePath, url.Values{"recovery_code": {"abcd-efgh"}}, "pend-1")
	if svc.code != "abcd-efgh" {
		t.Errorf("code = %q, want recovery code", svc.code)
	}
	if !strings.Contains(rec.Body.String(), handler.TwoFactorRecoveryInvalid) {
		t.Errorf("missing recovery message: %s", rec.Body.String())
	}
}

func TestChallengeSubmitInvalidCodeShowsMessage(t *testing.T) {
	svc := &fakeChallenge{err: twofactor.ErrInvalidCode}
	router := challengeRouter(t, svc, pendingStore())
	rec := postForm(t, router, handler.ChallengePath, url.Values{"code": {"000000"}}, "pend-1")
	if !strings.Contains(rec.Body.String(), handler.TwoFactorCodeInvalid) {
		t.Errorf("missing invalid-code message: %s", rec.Body.String())
	}
}

// password reset

type fakeResetSvc struct {
	token                        string
	issueErr                     error
	completeErr                  error
	completeEmail, completeToken string
}

func (f *fakeResetSvc) Issue(_ context.Context, email string) (string, error) {
	return f.token, f.issueErr
}

func (f *fakeResetSvc) Complete(_ context.Context, email, rawToken, newPassword string) error {
	f.completeEmail, f.completeToken = email, rawToken
	return f.completeErr
}

func resetRouter(t *testing.T, svc *fakeResetSvc, store *fakeSessions, sent *[]string) http.Handler {
	t.Helper()
	send := func(email, rawToken string) error {
		*sent = append(*sent, email+"|"+rawToken)
		return nil
	}
	h := handler.NewPasswordReset(svc, send, "Baaku")
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, store))
	r.Use(middleware.CSRF(csrfKey))
	r.Get("/forgot-password", h.ShowForgot)
	r.Post("/forgot-password", h.Forgot)
	r.Get("/reset-password/{token}", h.ShowReset)
	r.Post("/reset-password", h.Reset)
	return r
}

func TestForgotPasswordSendsLink(t *testing.T) {
	svc := &fakeResetSvc{token: "tok123"}
	var sent []string
	router := resetRouter(t, svc, &fakeSessions{}, &sent)

	rec := postForm(t, router, "/forgot-password", url.Values{"email": {"ada@example.com"}}, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/forgot-password" {
		t.Fatalf("got %d -> %q", rec.Code, rec.Header().Get("Location"))
	}
	if len(sent) != 1 || sent[0] != "ada@example.com|tok123" {
		t.Errorf("sent = %v, want [ada@example.com|tok123]", sent)
	}
}

func TestForgotPasswordUnknownEmailShowsUserMessage(t *testing.T) {
	svc := &fakeResetSvc{issueErr: passwordreset.ErrUserNotFound}
	var sent []string
	router := resetRouter(t, svc, &fakeSessions{}, &sent)

	rec := postForm(t, router, "/forgot-password", url.Values{"email": {"ghost@example.com"}}, "")
	// the apostrophe is HTML-escaped by templ; match the rest of the line
	if !strings.Contains(rec.Body.String(), "find a user with that email address") {
		t.Errorf("missing passwords.user message: %s", rec.Body.String())
	}
	if len(sent) != 0 {
		t.Errorf("email sent for unknown user: %v", sent)
	}
}

func TestForgotPasswordThrottledShowsWaitMessage(t *testing.T) {
	svc := &fakeResetSvc{issueErr: passwordreset.ThrottledError{RetryAfter: 42 * time.Second}}
	var sent []string
	router := resetRouter(t, svc, &fakeSessions{}, &sent)

	rec := postForm(t, router, "/forgot-password", url.Values{"email": {"ada@example.com"}}, "")
	if !strings.Contains(rec.Body.String(), "Please wait 43 seconds before trying again.") {
		t.Errorf("missing passwords.throttled message: %s", rec.Body.String())
	}
	if len(sent) != 0 {
		t.Errorf("email sent while throttled: %v", sent)
	}
}

func TestResetPasswordInvalidTokenShowsMessage(t *testing.T) {
	svc := &fakeResetSvc{completeErr: passwordreset.ErrInvalidToken}
	var sent []string
	router := resetRouter(t, svc, &fakeSessions{}, &sent)

	rec := postForm(t, router, "/reset-password", url.Values{
		"token": {"bad"}, "email": {"ada@example.com"},
		"password": {"Str0ng!Pass"}, "password_confirmation": {"Str0ng!Pass"},
	}, "")
	if !strings.Contains(rec.Body.String(), handler.PasswordsToken) {
		t.Errorf("missing passwords.token message: %s", rec.Body.String())
	}
}

func TestResetPasswordSuccessRedirectsToLogin(t *testing.T) {
	svc := &fakeResetSvc{}
	var sent []string
	router := resetRouter(t, svc, &fakeSessions{}, &sent)

	rec := postForm(t, router, "/reset-password", url.Values{
		"token": {"tok"}, "email": {"ada@example.com"},
		"password": {"Str0ng!Pass"}, "password_confirmation": {"Str0ng!Pass"},
	}, "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != middleware.LoginPath {
		t.Fatalf("got %d -> %q, want 302 -> login", rec.Code, rec.Header().Get("Location"))
	}
	if svc.completeEmail != "ada@example.com" || svc.completeToken != "tok" {
		t.Errorf("Complete(%q, %q), want (ada@example.com, tok)", svc.completeEmail, svc.completeToken)
	}
}
