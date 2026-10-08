package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeTwoFactor struct {
	enableRes  twofactor.EnableResult
	confirmErr error
	recovered  []string
	setupKey   string
	png        []byte
	enableN    int
	disableN   int
	recoverN   int
	code       string
}

func (f *fakeTwoFactor) Enable(_ context.Context, _ int64) (twofactor.EnableResult, error) {
	f.enableN++
	return f.enableRes, nil
}

func (f *fakeTwoFactor) Confirm(_ context.Context, _ int64, code string) error {
	f.code = code
	return f.confirmErr
}

func (f *fakeTwoFactor) Disable(_ context.Context, _ int64) error {
	f.disableN++
	return nil
}

func (f *fakeTwoFactor) RegenerateRecoveryCodes(_ context.Context, _ int64) ([]string, error) {
	f.recoverN++
	return f.recovered, nil
}

func (f *fakeTwoFactor) QRCodePNG(_ context.Context, _ int64) ([]byte, error) { return f.png, nil }

func (f *fakeTwoFactor) SetupKey(_ context.Context, _ int64) (string, error) { return f.setupKey, nil }

// harness

func sessionRow(id string, payload string) generated.Session {
	uid := int64(9)
	return generated.Session{ID: id, UserID: &uid, Payload: payload}
}

func twoFactorRouter(svc *fakeTwoFactor, store *fakeSessions) http.Handler {
	tfs := handler.NewTwoFactorSettings(svc)
	profp := handler.NewProfilePage(svc, "Baaku")
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, store))
	r.Use(middleware.CSRF(csrfKey))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(middleware.WithUser(req.Context(), user.User{ID: 9})))
		})
	})
	r.Post("/user/two-factor-authentication", tfs.Enable)
	r.Post("/user/confirmed-two-factor-authentication", tfs.Confirm)
	r.Delete("/user/two-factor-authentication", tfs.Disable)
	r.Get("/user/two-factor-qr-code", tfs.QRCode)
	r.Post("/user/two-factor-recovery-codes", tfs.Regenerate)
	r.Get("/dashboard/profile", profp.Show)
	return r
}

func deleteReq(t *testing.T, router http.Handler, path, sessID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	sealed := sealedCSRF(t)
	req.AddCookie(&http.Cookie{Name: middleware.XSRFCookieName, Value: sealed})
	req.Header.Set(middleware.XSRFHeaderName, sealed)
	req.AddCookie(&http.Cookie{Name: testCookie, Value: sessID})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// tests

func TestTwoFactorEnableFlashesEnableState(t *testing.T) {
	svc := &fakeTwoFactor{enableRes: twofactor.EnableResult{RecoveryCodes: []string{"a", "b"}}}
	store := &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}}
	rec := postForm(t, twoFactorRouter(svc, store), "/user/two-factor-authentication", nil, "s1")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302; body: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != middleware.DashboardPath {
		t.Errorf("Location = %q, want %q", loc, middleware.DashboardPath)
	}
	if svc.enableN != 1 {
		t.Errorf("Enable called %d times, want 1", svc.enableN)
	}
	payload := store.byID["s1"].Payload
	for _, want := range []string{
		"two-factor-authentication-enabled",
		`"confirmation":"required"`,
		"a\\nb",
	} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload %s missing %q", payload, want)
		}
	}
}

func TestTwoFactorConfirm(t *testing.T) {
	svc := &fakeTwoFactor{}
	rec := postForm(t, twoFactorRouter(svc, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}}),
		"/user/confirmed-two-factor-authentication", url.Values{"code": {"123456"}}, "s1")
	if rec.Code != http.StatusFound || svc.code != "123456" {
		t.Errorf("code=%q status=%d", svc.code, rec.Code)
	}

	svc.confirmErr = twofactor.ErrInvalidCode
	store := &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}}
	rec = postForm(t, twoFactorRouter(svc, store),
		"/user/confirmed-two-factor-authentication", url.Values{"code": {"000000"}}, "s1")
	if rec.Code != http.StatusFound {
		t.Errorf("invalid code status = %d, want 302 (error flash)", rec.Code)
	}
	if !strings.Contains(store.byID["s1"].Payload, "The provided two factor code was invalid.") {
		t.Errorf("payload %q missing invalid-code message", store.byID["s1"].Payload)
	}
}

func TestTwoFactorDisableAndRegenerate(t *testing.T) {
	svc := &fakeTwoFactor{recovered: []string{"x", "y"}}
	router := twoFactorRouter(svc, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}})

	rec := deleteReq(t, router, "/user/two-factor-authentication", "s1")
	if rec.Code != http.StatusFound || svc.disableN != 1 {
		t.Errorf("disable: status=%d calls=%d", rec.Code, svc.disableN)
	}

	rec = postForm(t, router, "/user/two-factor-recovery-codes", nil, "s1")
	if rec.Code != http.StatusFound || svc.recoverN != 1 {
		t.Errorf("regenerate: status=%d calls=%d", rec.Code, svc.recoverN)
	}
}

func TestTwoFactorQRCode(t *testing.T) {
	svc := &fakeTwoFactor{png: []byte{0x89, 'P', 'N', 'G'}}
	rec := get(t, twoFactorRouter(svc, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}}), "/user/two-factor-qr-code", "s1")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("status=%d ct=%q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

func TestProfilePageEnabledShowsQRSetupAndCodes(t *testing.T) {
	payload, _ := json.Marshal(map[string]map[string]string{"flash": {
		"status":        "two-factor-authentication-enabled",
		"confirmation":  "required",
		"recoveryCodes": "c1\nc2",
	}})
	svc := &fakeTwoFactor{setupKey: "SECRET"}
	router := twoFactorRouter(svc, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", string(payload))}})
	rec := get(t, router, "/dashboard/profile", "s1")
	body := rec.Body.String()
	for _, want := range []string{"/user/two-factor-qr-code", "SECRET", "c1", "c2", "confirmed-two-factor-authentication"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestProfilePageConfirmedShowsDisableForms(t *testing.T) {
	payload, _ := json.Marshal(map[string]map[string]string{"flash": {"status": "two-factor-authentication-confirmed"}})
	router := twoFactorRouter(&fakeTwoFactor{}, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", string(payload))}})
	rec := get(t, router, "/dashboard/profile", "s1")
	body := rec.Body.String()
	for _, want := range []string{"Disable Two-Factor Authentication", "Regenerate Recovery Codes"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
	if strings.Contains(body, "Enable Two-Factor Authentication") {
		t.Error("confirmed state should not show the enable button")
	}
}

func TestProfilePagePlainShowsEnableButton(t *testing.T) {
	router := twoFactorRouter(&fakeTwoFactor{}, &fakeSessions{byID: map[string]generated.Session{"s1": sessionRow("s1", "{}")}})
	rec := get(t, router, "/dashboard/profile", "s1")
	if !strings.Contains(rec.Body.String(), "Enable Two-Factor Authentication") {
		t.Error("body missing enable button")
	}
}
