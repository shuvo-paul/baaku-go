package auth

import (
	"context"
	"errors"
	"slices"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

var errStore = errors.New("store down")

type fakeUsers struct {
	user       User
	err        error
	askedEmail string
}

func (f *fakeUsers) UserByEmail(_ context.Context, email string) (User, error) {
	f.askedEmail = email
	return f.user, f.err
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
	user := User{ID: 42, PasswordHash: string(hash)}

	cases := []struct {
		name             string
		users            *fakeUsers
		sessions         *fakeSessions
		twofa            *fakeTwoFactor
		password         string
		want             LoginResult
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
			want:             LoginResult{UserID: 42, SessionID: "sess-1"},
			wantSessionsFor:  []int64{42},
			wantTwoFactorFor: []int64{42},
		},
		{
			name:             "bad password",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{},
			password:         "wrong",
			wantErr:          ErrInvalidCredentials,
			wantTwoFactorFor: nil,
		},
		{
			name:     "unknown email",
			users:    &fakeUsers{err: ErrUserNotFound},
			sessions: &fakeSessions{createID: "sess-1"},
			twofa:    &fakeTwoFactor{},
			password: "hunter2!",
			wantErr:  ErrInvalidCredentials,
		},
		{
			name:             "2FA pending",
			users:            &fakeUsers{user: user},
			sessions:         &fakeSessions{createID: "sess-1"},
			twofa:            &fakeTwoFactor{confirmed: true},
			password:         "hunter2!",
			want:             LoginResult{UserID: 42, TwoFactorPending: true},
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
			svc := New(c.users, c.sessions, c.twofa)
			got, err := svc.Login(context.Background(), "a@b.c", c.password)

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
	svc := New(&fakeUsers{}, sessions, &fakeTwoFactor{})

	if err := svc.Logout(context.Background(), "sess-9"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !slices.Equal(sessions.deleted, []string{"sess-9"}) {
		t.Errorf("deleted = %v, want [sess-9]", sessions.deleted)
	}

	sessions = &fakeSessions{deleteErr: errStore}
	svc = New(&fakeUsers{}, sessions, &fakeTwoFactor{})
	if err := svc.Logout(context.Background(), "sess-9"); !errors.Is(err, errStore) {
		t.Errorf("err = %v, want wrapped %v", err, errStore)
	}
}
