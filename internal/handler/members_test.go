package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/handler"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/memberdirectory"
	"github.com/shuvo-paul/baaku/internal/service/membership"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeMemberDir struct{ page memberdirectory.Page }

func (f *fakeMemberDir) Directory(_ context.Context, _ bool, _, _ string, _ int64) (memberdirectory.Page, map[string]int64, error) {
	return f.page, map[string]int64{"pending": 1}, nil
}

func (f *fakeMemberDir) Get(_ context.Context, _ bool, _ int64) (memberdirectory.Member, error) {
	return memberdirectory.Member{}, nil
}

func (f *fakeMemberDir) ChangeState(_ context.Context, _, _ int64, _, _ string) error { return nil }

type fakeMemberships struct{}

func (fakeMemberships) LatestForUser(_ context.Context, _ int64) (membership.Membership, error) {
	return membership.Membership{}, membership.ErrNotFound
}

type fakeMemberPayments struct{}

func (fakeMemberPayments) ListForUser(_ context.Context, _, _ int64) (membershippayment.Page, error) {
	return membershippayment.Page{}, nil
}

type fakePermLister struct{ names []string }

func (f fakePermLister) UserPermissionNames(_ context.Context, _ int64) ([]string, error) {
	return f.names, nil
}

// membersIndexRouter wires the Members index behind a user + permissions
// context (reference users.index route middleware).
func membersIndexRouter(perms []string) http.Handler {
	h := handler.NewMembers(&fakeMemberDir{page: memberdirectory.Page{
		Members: []memberdirectory.MemberCard{{
			ID: 3, Name: "Ada Lovelace", Email: "ada@example.test", State: user.StatePending,
		}},
		Total: 1,
	}}, fakeMemberships{}, fakeMemberPayments{}, true, "TestApp")
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(middleware.WithUser(req.Context(), user.User{ID: 1, Name: "Admin"})))
		})
	})
	r.Use(middleware.LoadPermissions(fakePermLister{perms}))
	r.Get("/dashboard/users", h.Index)
	return r
}

// The pending filter swaps the card action for the reference's
// "Review & approve" label.
func TestMembersIndexPendingFilterShowsReviewLabel(t *testing.T) {
	srv := membersIndexRouter([]string{"manage members"})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/users?filter=pending", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Review &amp; approve") {
		t.Errorf("pending grid should render the review label, body: %s", rec.Body.String())
	}
}

// XHR requests (the debounced live search) get the grid + pagination JSON
// fragments (reference ajax() branch).
func TestMembersIndexXHRReturnsGridFragments(t *testing.T) {
	srv := membersIndexRouter([]string{"manage members"})
	req := httptest.NewRequest(http.MethodGet, "/dashboard/users?filter=pending&search=ada", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response is not JSON: %v (%s)", err, rec.Body.String())
	}
	if !strings.Contains(payload["grid"], "Ada Lovelace") {
		t.Errorf("grid = %q", payload["grid"])
	}
	if _, ok := payload["pagination"]; !ok {
		t.Errorf("missing pagination key: %v", payload)
	}
}
