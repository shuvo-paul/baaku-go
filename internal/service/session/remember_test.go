package session_test

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/service/session"
)

func TestRecallerName(t *testing.T) {
	// remember_web_<sha1(Illuminate\Auth\SessionGuard)> — Laravel's exact name.
	want := "remember_web_59ba36addc2b2f9401580f014c7f58ea4e30989d"
	if got := session.RecallerName(); got != want {
		t.Errorf("RecallerName() = %q, want %q", got, want)
	}
}

func TestNewRememberToken(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok, err := session.NewRememberToken()
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 60 {
			t.Fatalf("token len = %d, want 60", len(tok))
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9]{60}$`).MatchString(tok) {
			t.Fatalf("token %q is not alphanumeric", tok)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok] = true
	}
}

func TestRecallerValueRoundTrip(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	value := session.RecallerValue(key, 42, "tok", "$2a$10$hash")

	if !strings.HasPrefix(value, "42|tok|") {
		t.Errorf("value = %q, want id|token|mac", value)
	}
	id, token, hash, ok := session.ParseRecaller(value)
	if !ok || id != 42 || token != "tok" || hash != session.HashPasswordForCookie(key, "$2a$10$hash") {
		t.Errorf("ParseRecaller = (%d, %q, %q, %v)", id, token, hash, ok)
	}
}

func TestParseRecallerRejectsMalformed(t *testing.T) {
	bad := []string{
		"",
		"42|tok",            // two parts
		"42|tok|hash|extra", // four parts
		"|tok|hash",         // empty id
		"42||hash",          // empty token
		"42|tok|",           // empty hash
		"zero|tok|hash",     // non-numeric id
		"0|tok|hash",        // non-positive id
	}
	for _, v := range bad {
		if _, _, _, ok := session.ParseRecaller(v); ok {
			t.Errorf("ParseRecaller(%q) accepted", v)
		}
	}
}

type fakeTokenStore struct {
	sets []*string
	err  error
}

func (f *fakeTokenStore) SetRememberToken(_ context.Context, _ int64, token *string) error {
	f.sets = append(f.sets, token)
	return f.err
}

func TestEnsureRememberToken(t *testing.T) {
	t.Run("cycles when empty", func(t *testing.T) {
		store := &fakeTokenStore{}
		tok, err := session.EnsureRememberToken(context.Background(), store, 7, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(tok) != 60 {
			t.Errorf("token len = %d, want 60", len(tok))
		}
		if len(store.sets) != 1 || store.sets[0] == nil || *store.sets[0] != tok {
			t.Errorf("persisted = %v, want the returned token", store.sets)
		}
	})

	t.Run("keeps an existing token", func(t *testing.T) {
		store := &fakeTokenStore{}
		tok, err := session.EnsureRememberToken(context.Background(), store, 7, "existing")
		if err != nil {
			t.Fatal(err)
		}
		if tok != "existing" {
			t.Errorf("token = %q, want existing", tok)
		}
		if len(store.sets) != 0 {
			t.Errorf("store written despite existing token: %v", store.sets)
		}
	})

	t.Run("propagates store errors", func(t *testing.T) {
		store := &fakeTokenStore{err: errors.New("down")}
		if _, err := session.EnsureRememberToken(context.Background(), store, 7, ""); err == nil {
			t.Error("err = nil, want store error")
		}
	})
}

func TestForgetRememberCookieExpires(t *testing.T) {
	cfg := config.Session{Cookie: "sid", Lifetime: 120, HttpOnly: true, SameSite: "lax"}
	c := session.ForgetRememberCookie(cfg)
	if c.Name != session.RecallerName() {
		t.Errorf("name = %q, want %q", c.Name, session.RecallerName())
	}
	if c.Value != "" || c.MaxAge >= 0 || c.Expires.After(time.Now()) {
		t.Errorf("cookie = %+v, want emptied and expired", c)
	}
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Errorf("flags lost on forget: %+v", c)
	}
}
