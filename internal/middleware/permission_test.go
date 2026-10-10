package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

type fakeChecker struct {
	perms map[int64][]string
	err   error
}

func (f *fakeChecker) Can(_ context.Context, userID int64, permission string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, p := range f.perms[userID] {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}

func servePermission(t *testing.T, checker middleware.PermissionChecker, uid *int64) *httptest.ResponseRecorder {
	t.Helper()
	h := middleware.RequirePermission(checker, "view activity log")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard/activity-log", nil)
	if uid != nil {
		req = req.WithContext(middleware.WithUser(req.Context(), user.User{ID: *uid}))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRequirePermission(t *testing.T) {
	checker := &fakeChecker{perms: map[int64][]string{1: {"view activity log"}, 2: {"manage roles"}}}

	tests := []struct {
		name string
		uid  *int64
		want int
	}{
		{"holder passes", ptr(int64(1)), http.StatusTeapot},
		{"non-holder gets 403", ptr(int64(2)), http.StatusForbidden},
		{"no user gets 403", nil, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := servePermission(t, checker, tt.uid).Code; got != tt.want {
				t.Errorf("status = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRequirePermissionStoreError(t *testing.T) {
	uid := int64(1)
	checker := &fakeChecker{err: errors.New("boom")}
	if got := servePermission(t, checker, &uid).Code; got != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", got, http.StatusInternalServerError)
	}
}

type fakeLister struct {
	names map[int64][]string
	err   error
}

func (f *fakeLister) UserPermissionNames(_ context.Context, userID int64) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.names[userID], nil
}

func TestLoadPermissionsStashesNames(t *testing.T) {
	lister := &fakeLister{names: map[int64][]string{1: {"view activity log", "manage roles"}}}
	var got []string
	h := middleware.LoadPermissions(lister)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = middleware.PermissionsFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil).
		WithContext(middleware.WithUser(context.Background(), user.User{ID: 1}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(got) != 2 || got[0] != "view activity log" || got[1] != "manage roles" {
		t.Errorf("permissions = %v, want [view activity log manage roles]", got)
	}
}

func TestLoadPermissionsGuestYieldsNil(t *testing.T) {
	lister := &fakeLister{names: map[int64][]string{}}
	var got []string
	called := false
	h := middleware.LoadPermissions(lister)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		got = middleware.PermissionsFromContext(r.Context())
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called || got != nil {
		t.Errorf("called = %v, permissions = %v, want pass-through with nil", called, got)
	}
}

func TestLoadPermissionsListerError(t *testing.T) {
	lister := &fakeLister{err: errors.New("boom")}
	h := middleware.LoadPermissions(lister)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil).
		WithContext(middleware.WithUser(context.Background(), user.User{ID: 1}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
