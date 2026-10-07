package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/middleware"
)

// TestFlashReadOnce covers the done-criteria: set-flash → redirect → next
// request reads once → subsequent request sees nothing.
func TestFlashReadOnce(t *testing.T) {
	store := &fakeSessions{byID: map[string]generated.Session{"s1": {ID: "s1"}}}
	h := middleware.Session(testCookie, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			middleware.RedirectWithFlash(w, r, "/next", "status", "verification-link-sent")
		case "/next":
			f := middleware.FlashFromContext(r.Context())
			if f["status"] != "verification-link-sent" {
				t.Errorf("flash on next request = %v, want status=verification-link-sent", f)
			}
			w.WriteHeader(http.StatusOK)
		case "/after":
			if f := middleware.FlashFromContext(r.Context()); len(f) != 0 {
				t.Errorf("flash leaked into third request: %v", f)
			}
			w.WriteHeader(http.StatusOK)
		}
	}))

	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: testCookie, Value: "s1"})
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	// Request 1: redirect sets the flash in the session payload.
	rec := get("/start")
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/next" {
		t.Errorf("Location = %q, want /next", loc)
	}
	payload := store.byID["s1"].Payload
	if want := `{"flash":{"status":"verification-link-sent"}}`; payload != want {
		t.Errorf("stored payload = %s, want %s", payload, want)
	}

	// Request 2: reads the flash once; middleware ages it out of the payload.
	if rec := get("/next"); rec.Code != http.StatusOK {
		t.Fatalf("next status = %d, want 200", rec.Code)
	}
	if got := store.byID["s1"].Payload; got != "{}" {
		t.Errorf("payload after aging = %s, want {}", got)
	}

	// Request 3: nothing left.
	if rec := get("/after"); rec.Code != http.StatusOK {
		t.Fatalf("after status = %d, want 200", rec.Code)
	}
}

// TestSetFlashGuestNoOp: guests have no session to flash into; the redirect
// itself must still work.
func TestSetFlashGuestNoOp(t *testing.T) {
	store := &fakeSessions{byID: map[string]generated.Session{}}
	h := middleware.Session(testCookie, store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		middleware.RedirectWithFlash(w, r, "/login", "error", "nope")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/restricted", nil))

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if store.saves != 0 {
		t.Errorf("guest flash saved %d times, want 0", store.saves)
	}
}
