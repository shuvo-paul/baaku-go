package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

var rememberCfg = config.Session{Cookie: testCookie, Lifetime: 120, HttpOnly: true, SameSite: "lax"}

var rememberKey = []byte("0123456789abcdef0123456789abcdef")

// rememberSessions adds CreateSession to the shared fakeSessions port.
type rememberSessions struct {
	*fakeSessions
	created []int64
}

func (f *rememberSessions) CreateSession(_ context.Context, userID int64) (string, error) {
	f.created = append(f.created, userID)
	uid := userID
	f.byID["restored"] = generated.Session{ID: "restored", UserID: &uid, Payload: "{}"}
	return "restored", nil
}

func rememberUser(token, hash string) user.User {
	u := user.User{ID: 7, PasswordHash: hash}
	if token != "" {
		tok := token
		u.RememberToken = &tok
	}
	return u
}

// rememberRouter mounts Session + Remember; the leaf reports whether a user
// landed in the context and echoes the session ID.
func rememberRouter(users *fakeUsers, store *rememberSessions) http.Handler {
	leaf := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := middleware.UserFromContext(r.Context()); ok {
			w.Header().Set("X-User", "set")
		}
		if sess, ok := middleware.SessionFromContext(r.Context()); ok && sess.UserID != nil {
			w.Header().Set("X-Session", sess.ID)
		}
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.Session(testCookie, store)(
		middleware.Remember(rememberCfg, rememberKey, users, store)(leaf))
	return h
}

func recallerRequest(t *testing.T, value string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	if value != "" {
		req.AddCookie(&http.Cookie{Name: session.RecallerName(), Value: value})
	}
	return req
}

func TestRememberRestoresSessionFromRecaller(t *testing.T) {
	hash := "$2a$10$fakehash"
	users := &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}}
	store := &rememberSessions{fakeSessions: &fakeSessions{byID: map[string]generated.Session{}}}
	router := rememberRouter(users, store)

	value := session.RecallerValue(rememberKey, 7, "tok-1", hash)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, recallerRequest(t, value))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Session") != "restored" {
		t.Errorf("restored session = %q, want restored", rec.Header().Get("X-Session"))
	}
	if len(store.created) != 1 || store.created[0] != 7 {
		t.Errorf("sessions created for %v, want [7]", store.created)
	}
	var sc *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == testCookie {
			sc = c
		}
	}
	if sc == nil || sc.Value != "restored" {
		t.Errorf("session cookie = %+v, want restored", sc)
	}
}

func TestRememberRejectsBadRecallers(t *testing.T) {
	hash := "$2a$10$fakehash"
	cases := []struct {
		name  string
		value string
		users *fakeUsers
	}{
		{
			name:  "wrong token",
			value: session.RecallerValue(rememberKey, 7, "tok-EVIL", hash),
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}},
		},
		{
			name:  "no stored token",
			value: session.RecallerValue(rememberKey, 7, "tok-1", hash),
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("", hash)}},
		},
		{
			name:  "password changed since issue",
			value: session.RecallerValue(rememberKey, 7, "tok-1", "$2a$10$OLDhash"),
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}},
		},
		{
			name:  "forged MAC",
			value: session.RecallerValue(rememberKey, 7, "tok-1", hash)[:len(session.RecallerValue(rememberKey, 7, "tok-1", hash))-4] + "0000",
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}},
		},
		{
			name:  "malformed value",
			value: "7|not-three-parts",
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}},
		},
		{
			name:  "unknown user",
			value: session.RecallerValue(rememberKey, 99, "tok-1", hash),
			users: &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := &rememberSessions{fakeSessions: &fakeSessions{byID: map[string]generated.Session{}}}
			router := rememberRouter(c.users, store)

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, recallerRequest(t, c.value))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (guest passthrough)", rec.Code)
			}
			if rec.Header().Get("X-Session") != "" {
				t.Errorf("guest restored as session %q", rec.Header().Get("X-Session"))
			}
			if len(store.created) != 0 {
				t.Errorf("sessions created for rejected recaller: %v", store.created)
			}
		})
	}
}

func TestRememberSkipsAuthenticatedSessions(t *testing.T) {
	hash := "$2a$10$fakehash"
	users := &fakeUsers{byID: map[int64]user.User{7: rememberUser("tok-1", hash)}}
	uid := int64(7)
	store := &rememberSessions{fakeSessions: &fakeSessions{byID: map[string]generated.Session{
		"s1": {ID: "s1", UserID: &uid},
	}}}
	router := rememberRouter(users, store)

	value := session.RecallerValue(rememberKey, 7, "tok-1", hash)
	req := recallerRequest(t, value)
	req.AddCookie(&http.Cookie{Name: testCookie, Value: "s1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Header().Get("X-Session") != "s1" {
		t.Errorf("session = %q, want s1 (existing session wins)", rec.Header().Get("X-Session"))
	}
	if len(store.created) != 0 {
		t.Errorf("recaller re-login despite live session: %v", store.created)
	}
}

func TestRememberMissingCookieIsGuest(t *testing.T) {
	users := &fakeUsers{byID: map[int64]user.User{}}
	store := &rememberSessions{fakeSessions: &fakeSessions{byID: map[string]generated.Session{}}}
	router := rememberRouter(users, store)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, recallerRequest(t, ""))

	if rec.Header().Get("X-Session") != "" {
		t.Errorf("anonymous request restored as %q", rec.Header().Get("X-Session"))
	}
}

// A store error must 500, not silently downgrade to guest.
func TestRememberStoreErrorFails(t *testing.T) {
	users := &fakeUsers{err: errBoom}
	store := &rememberSessions{fakeSessions: &fakeSessions{byID: map[string]generated.Session{}}}
	router := rememberRouter(users, store)

	value := session.RecallerValue(rememberKey, 7, "tok-1", "hash")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, recallerRequest(t, value))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal server error") {
		t.Errorf("body = %q", rec.Body.String())
	}
}

var errBoom = errors.New("boom")
