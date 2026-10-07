package profileinfo_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/service/profileinfo"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeUsers struct {
	byID   map[int64]user.User
	emails map[string]int64 // email → user id
	saved  map[int64]user.User
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := f.byID[id]
	if !ok {
		return user.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (user.User, error) {
	id, ok := f.emails[email]
	if !ok {
		return user.User{}, pgx.ErrNoRows
	}
	return f.byID[id], nil
}

func (f *fakeUsers) UpdateProfile(_ context.Context, id int64, name, email string, emailVerifiedAt *time.Time) error {
	u := f.byID[id]
	u.Name, u.Email, u.EmailVerifiedAt = name, email, emailVerifiedAt
	if f.saved == nil {
		f.saved = map[int64]user.User{}
	}
	f.saved[id] = u
	return nil
}

// harness

func newFixture() (*profileinfo.Service, *fakeUsers) {
	verified := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	users := &fakeUsers{
		byID: map[int64]user.User{
			1: {ID: 1, Name: "Ada", Email: "ada@example.com", EmailVerifiedAt: &verified},
			2: {ID: 2, Name: "Bob", Email: "bob@example.com"},
		},
		emails: map[string]int64{"ada@example.com": 1, "bob@example.com": 2},
	}
	return profileinfo.New(users), users
}

// tests

func TestUpdateSavesNameAndEmail(t *testing.T) {
	svc, users := newFixture()
	if err := svc.Update(context.Background(), 1, "Ada Lovelace", "ada@example.com"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	s := users.saved[1]
	if s.Name != "Ada Lovelace" || s.Email != "ada@example.com" {
		t.Errorf("saved = %+v", s)
	}
	if s.EmailVerifiedAt == nil {
		t.Error("email_verified_at cleared although email unchanged")
	}
}

func TestUpdateEmailChangeClearsVerifiedAt(t *testing.T) {
	svc, users := newFixture()
	if err := svc.Update(context.Background(), 1, "Ada", "new@example.com"); err != nil {
		t.Fatalf("Update = %v, want nil", err)
	}
	if users.saved[1].EmailVerifiedAt != nil {
		t.Error("email_verified_at kept after email change, want nil")
	}
}

func TestUpdateValidationMirrorsLaravel(t *testing.T) {
	svc, _ := newFixture()
	ctx := context.Background()

	tests := []struct {
		name, inName, inEmail string
		wantField, wantMsg    string
	}{
		{"name required", "", "ada@example.com", "name", "The name field is required."},
		{"name latin only", "Adá!", "ada@example.com", "name", profileinfo.NameLatinOnlyMessage},
		{"name max 255", strings.Repeat("a", 256), "ada@example.com", "name", "The name field must not be greater than 255 characters."},
		{"email required", "Ada", "", "email", "The email field is required."},
		{"email format", "Ada", "not-an-email", "email", "The email field must be a valid email address."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := svc.Update(ctx, 1, tt.inName, tt.inEmail)
			var errs profileinfo.FieldErrors
			if !errors.As(err, &errs) {
				t.Fatalf("Update = %v, want FieldErrors", err)
			}
			if errs[tt.wantField] != tt.wantMsg {
				t.Errorf("errs[%q] = %q, want %q", tt.wantField, errs[tt.wantField], tt.wantMsg)
			}
		})
	}
}

func TestUpdateUniqueIgnoringSelf(t *testing.T) {
	svc, users := newFixture()
	ctx := context.Background()

	// Own email is fine.
	// Own email is fine.
	if err := svc.Update(ctx, 1, "Ada", "ada@example.com"); err != nil {
		t.Fatalf("Update(own email) = %v, want nil", err)
	}
	savedBefore := len(users.saved)
	// Someone else's email is taken.
	err := svc.Update(ctx, 1, "Ada", "bob@example.com")
	var errs profileinfo.FieldErrors
	if !errors.As(err, &errs) {
		t.Fatalf("Update(other email) = %v, want FieldErrors", err)
	}
	if errs["email"] != "The email has already been taken." {
		t.Errorf("errs[email] = %q", errs["email"])
	}
	if len(users.saved) != savedBefore {
		t.Error("profile saved despite unique violation")
	}
}
