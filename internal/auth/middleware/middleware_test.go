package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/auth/session"
	"github.com/shuvo-paul/baaku/internal/auth/user"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

const testCookie = "baaku_session"

type fakeSessions struct {
	byID map[string]generated.Session
	err  error
}

func (f *fakeSessions) Load(_ context.Context, id string) (generated.Session, error) {
	if f.err != nil {
		return generated.Session{}, f.err
	}
	s, ok := f.byID[id]
	if !ok {
		return generated.Session{}, session.ErrNotFound
	}
	return s, nil
}

type fakeUsers struct {
	byID map[int64]user.User
	err  error
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (user.User, error) {
	if f.err != nil {
		return user.User{}, f.err
	}
	u, ok := f.byID[id]
	if !ok {
		return user.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func verifiedUser(state user.UserState) user.User {
	now := time.Now()
	return user.User{ID: 1, State: state, EmailVerifiedAt: &now}
}

func requestWithCookie(id string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	if id != "" {
		req.AddCookie(&http.Cookie{Name: testCookie, Value: id})
	}
	return req
}

func TestRequireAuth(t *testing.T) {
	uid := int64(1)
	sess := generated.Session{ID: "s1", UserID: &uid}
	sessNoUser := generated.Session{ID: "s2"}

	tests := []struct {
		name     string
		req      *http.Request
		sessions SessionLoader
		users    UserLoader
		wantCode int
		wantNext bool
		wantUser *user.User
	}{
		{
			name:     "no cookie redirects to login",
			req:      requestWithCookie(""),
			sessions: &fakeSessions{},
			users:    &fakeUsers{},
			wantCode: http.StatusFound,
			wantUser: nil,
		},
		{
			name:     "unknown session redirects to login",
			req:      requestWithCookie("missing"),
			sessions: &fakeSessions{byID: map[string]generated.Session{}},
			users:    &fakeUsers{},
			wantCode: http.StatusFound,
		},
		{
			name:     "session without user redirects to login",
			req:      requestWithCookie("s2"),
			sessions: &fakeSessions{byID: map[string]generated.Session{"s2": sessNoUser}},
			users:    &fakeUsers{},
			wantCode: http.StatusFound,
		},
		{
			name:     "user row missing redirects to login",
			req:      requestWithCookie("s1"),
			sessions: &fakeSessions{byID: map[string]generated.Session{"s1": sess}},
			users:    &fakeUsers{},
			wantCode: http.StatusFound,
		},
		{
			name:     "store error is 500",
			req:      requestWithCookie("s1"),
			sessions: &fakeSessions{err: errors.New("db down")},
			users:    &fakeUsers{},
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "repo error is 500",
			req:      requestWithCookie("s1"),
			sessions: &fakeSessions{byID: map[string]generated.Session{"s1": sess}},
			users:    &fakeUsers{err: errors.New("db down")},
			wantCode: http.StatusInternalServerError,
		},
		{
			name:     "valid session continues with user in context",
			req:      requestWithCookie("s1"),
			sessions: &fakeSessions{byID: map[string]generated.Session{"s1": sess}},
			users:    &fakeUsers{byID: map[int64]user.User{1: verifiedUser(user.StateActive)}},
			wantCode: http.StatusOK,
			wantNext: true,
			wantUser: ptr(verifiedUser(user.StateActive)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if u, ok := UserFromContext(r.Context()); ok && tt.wantUser != nil && u.ID != tt.wantUser.ID {
					t.Errorf("context user ID = %d, want %d", u.ID, tt.wantUser.ID)
				}
				w.WriteHeader(http.StatusOK)
			})
			h := RequireAuth(testCookie, tt.sessions, tt.users)(next)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, tt.req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
			if tt.wantUser == nil && rec.Code == http.StatusFound && rec.Header().Get("Location") != LoginPath {
				t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), LoginPath)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestRequireVerified(t *testing.T) {
	verified := verifiedUser(user.StateActive)
	unverified := user.User{ID: 1, State: user.StateActive}

	tests := []struct {
		name     string
		user     *user.User
		wantCode int
		wantNext bool
		wantLoc  string
	}{
		{name: "guest redirects to login", wantCode: http.StatusFound, wantLoc: LoginPath},
		{name: "unverified redirects to verify", user: &unverified, wantCode: http.StatusFound, wantLoc: VerifyEmailPath},
		{name: "verified continues", user: &verified, wantCode: http.StatusOK, wantNext: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
			if tt.user != nil {
				req = req.WithContext(withUser(req.Context(), *tt.user))
			}
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			RequireVerified(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
			if tt.wantLoc != "" && rec.Header().Get("Location") != tt.wantLoc {
				t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), tt.wantLoc)
			}
		})
	}
}

func TestCheckUserApproved(t *testing.T) {
	tests := []struct {
		name     string
		user     *user.User
		wantCode int
		wantNext bool
	}{
		{name: "guest passes", wantCode: http.StatusOK, wantNext: true},
		{name: "active passes", user: ptr(verifiedUser(user.StateActive)), wantCode: http.StatusOK, wantNext: true},
		{name: "pending is 403", user: ptr(verifiedUser(user.StatePending)), wantCode: http.StatusForbidden},
		{name: "unverified is 403", user: ptr(user.User{ID: 1, State: user.StateUnverified}), wantCode: http.StatusForbidden},
		{name: "suspended is 403", user: ptr(verifiedUser(user.StateSuspended)), wantCode: http.StatusForbidden},
		{name: "rejected is 403", user: ptr(verifiedUser(user.StateRejected)), wantCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.user != nil {
				req = req.WithContext(withUser(req.Context(), *tt.user))
			}
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			CheckUserApproved(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
		})
	}
}

func TestCheckUserSuspended(t *testing.T) {
	tests := []struct {
		name     string
		user     *user.User
		wantCode int
		wantNext bool
		wantLoc  string
	}{
		{name: "guest passes", wantCode: http.StatusOK, wantNext: true},
		{name: "active passes", user: ptr(verifiedUser(user.StateActive)), wantCode: http.StatusOK, wantNext: true},
		{
			name:     "suspended redirects with flash key",
			user:     ptr(verifiedUser(user.StateSuspended)),
			wantCode: http.StatusFound,
			wantLoc:  DashboardPath + "?error=" + SuspendedErrorKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/dashboard/posts", nil)
			if tt.user != nil {
				req = req.WithContext(withUser(req.Context(), *tt.user))
			}
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			rec := httptest.NewRecorder()
			CheckUserSuspended(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if called != tt.wantNext {
				t.Errorf("next called = %v, want %v", called, tt.wantNext)
			}
			if tt.wantLoc != "" && rec.Header().Get("Location") != tt.wantLoc {
				t.Errorf("Location = %q, want %q", rec.Header().Get("Location"), tt.wantLoc)
			}
		})
	}
}
