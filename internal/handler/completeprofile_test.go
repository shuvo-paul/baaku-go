package handler_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/completeprofile"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeCompleteness struct {
	complete bool
}

func (f *fakeCompleteness) Complete(_ context.Context, _ int64) (bool, error) {
	return f.complete, nil
}

type fakeCompleteSvc struct {
	in   completeprofile.Input
	user int64
	err  error
}

func (f *fakeCompleteSvc) Complete(_ context.Context, userID int64, in completeprofile.Input) error {
	f.user, f.in = userID, in
	return f.err
}

// harness

func completeProfileRouter(svc *fakeCompleteSvc, profiles *fakeCompleteness) http.Handler {
	h := handler.NewCompleteProfile(svc, profiles, "Baaku")
	r := chi.NewRouter()
	r.Use(middleware.Session(testCookie, &fakeSessions{}))
	r.Use(middleware.CSRF(csrfKey))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(middleware.WithUser(req.Context(), user.User{ID: 9})))
		})
	})
	r.Get("/profile/complete", h.Show)
	r.Post("/profile/complete", h.Store)
	return r
}

func completeForm() url.Values {
	return url.Values{
		"gender":            {"male"},
		"blood_group":       {"A+"},
		"present_address":   {"12 Lake Road"},
		"permanent_address": {"12 Lake Road"},
	}
}

// tests

func TestCompleteProfileShowRendersForm(t *testing.T) {
	rec := get(t, completeProfileRouter(&fakeCompleteSvc{}, &fakeCompleteness{}), "/profile/complete", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Complete profile") {
		t.Errorf("got %d, body missing form", rec.Code)
	}
}

func TestCompleteProfileShowBouncesCompleteProfiles(t *testing.T) {
	rec := get(t, completeProfileRouter(&fakeCompleteSvc{}, &fakeCompleteness{complete: true}), "/profile/complete", "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != middleware.DashboardPath {
		t.Errorf("got %d -> %q, want 302 -> %q", rec.Code, rec.Header().Get("Location"), middleware.DashboardPath)
	}
}

func TestCompleteProfileStoreValidatesAndSaves(t *testing.T) {
	svc := &fakeCompleteSvc{}
	rec := postForm(t, completeProfileRouter(svc, &fakeCompleteness{}), "/profile/complete", completeForm(), "")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != middleware.DashboardPath {
		t.Fatalf("got %d -> %q, want 302 -> %q; body: %s", rec.Code, rec.Header().Get("Location"), middleware.DashboardPath, rec.Body.String())
	}
	if svc.user != 9 || svc.in.Gender != "male" || svc.in.BloodGroup != "A+" {
		t.Errorf("service input = %+v for user %d", svc.in, svc.user)
	}
}

func TestCompleteProfileStoreRendersFieldErrors(t *testing.T) {
	svc := &fakeCompleteSvc{err: completeprofile.FieldErrors{"gender": "The gender field is required."}}
	rec := postForm(t, completeProfileRouter(svc, &fakeCompleteness{}), "/profile/complete", completeForm(), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "The gender field is required.") {
		t.Errorf("got %d, body missing field error", rec.Code)
	}
}
