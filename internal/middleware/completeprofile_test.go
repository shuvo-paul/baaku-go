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

type fakeCompleteness struct {
	complete map[int64]bool
	err      error
}

func (f *fakeCompleteness) Complete(_ context.Context, userID int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.complete[userID], nil
}

func TestCompleteProfileCheck(t *testing.T) {
	tests := []struct {
		name     string
		user     *user.User
		check    *fakeCompleteness
		wantCode int
		wantNext bool
		wantLoc  string
	}{
		{
			name:     "guest passes through",
			check:    &fakeCompleteness{},
			wantCode: http.StatusOK,
			wantNext: true,
		},
		{
			name:     "incomplete profile redirects to complete form",
			user:     &user.User{ID: 1},
			check:    &fakeCompleteness{complete: map[int64]bool{1: false}},
			wantCode: http.StatusFound,
			wantLoc:  middleware.CompleteProfilePath,
		},
		{
			name:     "complete profile passes",
			user:     &user.User{ID: 1},
			check:    &fakeCompleteness{complete: map[int64]bool{1: true}},
			wantCode: http.StatusOK,
			wantNext: true,
		},
		{
			name:     "missing profile row reads as incomplete",
			user:     &user.User{ID: 2},
			check:    &fakeCompleteness{complete: map[int64]bool{}},
			wantCode: http.StatusFound,
			wantLoc:  middleware.CompleteProfilePath,
		},
		{
			name:     "repo error is 500",
			user:     &user.User{ID: 1},
			check:    &fakeCompleteness{err: errors.New("db down")},
			wantCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			h := middleware.CompleteProfileCheck(tt.check)(next)

			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
			if tt.user != nil {
				req = req.WithContext(middleware.WithUser(req.Context(), *tt.user))
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

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
