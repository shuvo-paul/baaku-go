package login_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/shuvo-paul/baaku/internal/service/login"
)

var errStore = errors.New("store down")

type fakeUsers struct {
	user        login.User
	err         error
	askedEmail  string
	rememberSet []*string
}

func (f *fakeUsers) UserByEmail(_ context.Context, email string) (login.User, error) {
	f.askedEmail = email
	return f.user, f.err
}

func (f *fakeUsers) SetRememberToken(_ context.Context, id int64, token *string) error {
	f.rememberSet = append(f.rememberSet, token)
	return nil
}

type fakeSessions struct {
	createID   string
	createErr  error
	createdFor []int64
	deleted    []string
	deleteErr  error
}

func (f *fakeSessions) CreateSession(_ context.Context, userID int64) (string, error) {
	f.createdFor = append(f.createdFor, userID)
	return f.createID, f.createErr
}

func (f *fakeSessions) DeleteSession(_ context.Context, sessionID string) error {
	f.deleted = append(f.deleted, sessionID)
	return f.deleteErr
}

type fakeTwoFactor struct {
	confirmed bool
	err       error
	askedFor  []int64
}

func (f *fakeTwoFactor) TwoFactorConfirmed(_ context.Context, userID int64) (bool, error) {
	f.askedFor = append(f.askedFor, userID)
	return f.confirmed, f.err
}

func TestLogin(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user := login.User{ID: 42, PasswordHash: string(hash)}

	cases := []struct {
		name             string
		users            *fakeUsers
		sessions         *fakeSessions
		twofa            *fakeTwoFactor
		password         string
		want             login.LoginResult
		wantErr          error
		wantSessionsFor  []int64
		wantTwoFactorFor []int64
	}{
		{
			name:             "success",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{},
			password:         "hunter2!",
			want:             login.LoginResult{UserID: 42, SessionID: "sess-1"},
			wantSessionsFor:  []int64{42},
			wantTwoFactorFor: []int64{42},
		},
		{
			name:             "bad password",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{},
			password:         "wrong",
			wantErr:          login.ErrInvalidCredentials,
			wantTwoFactorFor: nil,
		},
		{
			name:     "unknown email",
			users:    &fakeUsers{err: login.ErrUserNotFound},
			sessions: &fakeSessions{createID: "sess-1"},
			twofa:    &fakeTwoFactor{},
			password: "hunter2!",
			wantErr:  login.ErrInvalidCredentials,
		},
		{
			name:             "2FA pending",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{confirmed: true},
			password:         "hunter2!",
			want:             login.LoginResult{UserID: 42, TwoFactorPending: true},
			wantTwoFactorFor: []int64{42},
		},
		{
			name:             "2FA check failure issues no session",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{err: errStore},
			password:         "hunter2!",
			wantErr:          errStore,
			wantTwoFactorFor: []int64{42},
		},
		{
			name:             "session create failure",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createErr: errStore},
			twofa:            &fakeTwoFactor{},
			password:         "hunter2!",
			wantErr:          errStore,
			wantSessionsFor:  []int64{42},
			wantTwoFactorFor: []int64{42},
		},
		{
			name:     "lookup failure",
			users:    &fakeUsers{err: errStore},
			sessions: &fakeSessions{},
			twofa:    &fakeTwoFactor{},
			password: "hunter2!",
			wantErr:  errStore,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := login.New(c.users, c.sessions, c.twofa)
			got, err := svc.Login(context.Background(), "a@b.c", c.password, false)

			if c.wantErr != nil {
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			} else if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != c.want {
				t.Errorf("result = %+v, want %+v", got, c.want)
			}
			if c.users.askedEmail != "a@b.c" {
				t.Errorf("user lookup email = %q, want %q", c.users.askedEmail, "a@b.c")
			}
			if !slices.Equal(c.sessions.createdFor, c.wantSessionsFor) {
				t.Errorf("sessions created for %v, want %v", c.sessions.createdFor, c.wantSessionsFor)
			}
			if !slices.Equal(c.twofa.askedFor, c.wantTwoFactorFor) {
				t.Errorf("2FA asked about %v, want %v", c.twofa.askedFor, c.wantTwoFactorFor)
			}
		})
	}
}

func TestLogout(t *testing.T) {
	sessions := &fakeSessions{}
	svc := login.New(&fakeUsers{}, sessions, &fakeTwoFactor{})

	if err := svc.Logout(context.Background(), "sess-9", nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(sessions.deleted, []string{"sess-9"}) {
		t.Errorf("deleted = %v, want [sess-9]", sessions.deleted)
	}

	sessions = &fakeSessions{deleteErr: errStore}
	svc = login.New(&fakeUsers{}, sessions, &fakeTwoFactor{})
	if err := svc.Logout(context.Background(), "sess-9", nil); !errors.Is(err, errStore) {
		t.Errorf("err = %v, want wrapped %v", err, errStore)
	}
}

func TestLogoutClearsRememberToken(t *testing.T) {
	users := &fakeUsers{}
	svc := login.New(users, &fakeSessions{}, &fakeTwoFactor{})
	uid := int64(42)

	if err := svc.Logout(context.Background(), "sess-9", &uid); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(users.rememberSet) != 1 || users.rememberSet[0] != nil {
		t.Errorf("remember token writes = %v, want one nil clear", users.rememberSet)
	}
}

func TestLoginRemember(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("hunter2!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("remember cycles a missing token and returns it", func(t *testing.T) {
		users := &fakeUsers{user: login.User{ID: 42, PasswordHash: string(hash)}}
		svc := login.New(users, &fakeSessions{createID: "s1"}, &fakeTwoFactor{})

		res, err := svc.Login(context.Background(), "a@b.c", "hunter2!", true)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if len(res.RememberToken) != 60 {
			t.Errorf("RememberToken len = %d, want 60", len(res.RememberToken))
		}
		if res.PasswordHash != string(hash) {
			t.Errorf("PasswordHash = %q, want the user's hash", res.PasswordHash)
		}
		if len(users.rememberSet) != 1 || users.rememberSet[0] == nil || *users.rememberSet[0] != res.RememberToken {
			t.Errorf("persisted token = %v, want the returned one", users.rememberSet)
		}
	})

	t.Run("remember reuses an existing token", func(t *testing.T) {
		users := &fakeUsers{user: login.User{ID: 42, PasswordHash: string(hash), RememberToken: "existing-token"}}
		svc := login.New(users, &fakeSessions{createID: "s1"}, &fakeTwoFactor{})

		res, err := svc.Login(context.Background(), "a@b.c", "hunter2!", true)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.RememberToken != "existing-token" {
			t.Errorf("RememberToken = %q, want existing-token", res.RememberToken)
		}
		if len(users.rememberSet) != 0 {
			t.Errorf("token rewritten despite existing one: %v", users.rememberSet)
		}
	})

	t.Run("no remember leaves the result bare", func(t *testing.T) {
		users := &fakeUsers{user: login.User{ID: 42, PasswordHash: string(hash)}}
		svc := login.New(users, &fakeSessions{createID: "s1"}, &fakeTwoFactor{})

		res, err := svc.Login(context.Background(), "a@b.c", "hunter2!", false)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if res.RememberToken != "" || res.PasswordHash != "" {
			t.Errorf("result = %+v, want no remember fields", res)
		}
		if len(users.rememberSet) != 0 {
			t.Errorf("token written without remember: %v", users.rememberSet)
		}
	})

	t.Run("2FA pending stashes no recaller", func(t *testing.T) {
		users := &fakeUsers{user: login.User{ID: 42, PasswordHash: string(hash)}}
		svc := login.New(users, &fakeSessions{createID: "s1"}, &fakeTwoFactor{confirmed: true})

		res, err := svc.Login(context.Background(), "a@b.c", "hunter2!", true)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !res.TwoFactorPending || res.RememberToken != "" {
			t.Errorf("result = %+v, want pending without recaller", res)
		}
	})
}
