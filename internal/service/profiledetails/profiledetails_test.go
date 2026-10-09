package profiledetails_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/repository/profile"
	"github.com/shuvo-paul/baaku/internal/service/profiledetails"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

type fakeUsers struct {
	byID   map[int64]user.User
	emails map[string]int64
	saved  *user.User
	phone  *string
}

func (f *fakeUsers) GetByID(_ context.Context, id int64) (user.User, error) { return f.byID[id], nil }

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (user.User, error) {
	if id, ok := f.emails[email]; ok {
		return f.byID[id], nil
	}
	return user.User{}, pgx.ErrNoRows
}

func (f *fakeUsers) UpdateProfile(_ context.Context, id int64, name, email string, verifiedAt *time.Time) error {
	u := f.byID[id]
	u.Name, u.Email, u.EmailVerifiedAt = name, email, verifiedAt
	f.saved = &u
	return nil
}

func (f *fakeUsers) UpdateContact(_ context.Context, _ int64, phone *string) error {
	f.phone = phone
	return nil
}

type fakeProfiles struct {
	upserted *profile.Profile
}

func (f *fakeProfiles) Get(_ context.Context, _ int64) (profile.Profile, error) {
	return profile.Profile{}, pgx.ErrNoRows
}

func (f *fakeProfiles) UpsertDetailsFull(_ context.Context, p profile.Profile) error {
	f.upserted = &p
	return nil
}

type fakeReviewer struct{ calls int }

func (f *fakeReviewer) Submit(_ context.Context, _ int64) error { f.calls++; return nil }

func newFixture() (*profiledetails.Service, *fakeUsers, *fakeProfiles, *fakeReviewer) {
	verified := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	users := &fakeUsers{
		byID:   map[int64]user.User{1: {ID: 1, Name: "Ada", Email: "ada@example.com", EmailVerifiedAt: &verified}},
		emails: map[string]int64{"ada@example.com": 1, "bob@example.com": 2},
	}
	profiles := &fakeProfiles{}
	rev := &fakeReviewer{}
	return profiledetails.New(users, profiles, rev), users, profiles, rev
}

func validInput() profiledetails.Input {
	return profiledetails.Input{
		Name: "Ada Lovelace", Email: "ada@example.com", Phone: "01700000000",
		PresentAddress: "12 Lake Road", PermanentAddress: "12 Lake Road",
	}
}

func TestUpdateKeepsVerifiedAtWhenEmailUnchanged(t *testing.T) {
	svc, users, _, rev := newFixture()
	if err := svc.Update(context.Background(), 1, validInput()); err != nil {
		t.Fatalf("Update = %v", err)
	}
	if users.saved.EmailVerifiedAt == nil {
		t.Error("email_verified_at cleared although email unchanged")
	}
	if rev.calls != 1 {
		t.Errorf("reviewer calls = %d, want 1", rev.calls)
	}
}

func TestUpdateEmailChangeClearsVerifiedAt(t *testing.T) {
	svc, users, _, _ := newFixture()
	in := validInput()
	in.Email = "new@example.com"
	if err := svc.Update(context.Background(), 1, in); err != nil {
		t.Fatalf("Update = %v", err)
	}
	if users.saved.EmailVerifiedAt != nil {
		t.Error("email_verified_at kept although email changed")
	}
}

func TestUpdateEmailTaken(t *testing.T) {
	svc, _, _, _ := newFixture()
	in := validInput()
	in.Email = "bob@example.com" // belongs to user 2
	err := svc.Update(context.Background(), 1, in)
	var errs profiledetails.FieldErrors
	if !errors.As(err, &errs) {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if errs["email"] != "The email has already been taken." {
		t.Errorf("email error = %q", errs["email"])
	}
}

func TestUpdateValidation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*profiledetails.Input)
		wantKey string
	}{
		{"name required", func(in *profiledetails.Input) { in.Name = "" }, "name"},
		{"name non-latin", func(in *profiledetails.Input) { in.Name = "আদা" }, "name"},
		{"email invalid", func(in *profiledetails.Input) { in.Email = "nope" }, "email"},
		{"phone required", func(in *profiledetails.Input) { in.Phone = "" }, "phone"},
		{"present address required", func(in *profiledetails.Input) { in.PresentAddress = "" }, "present_address"},
		{"gender invalid", func(in *profiledetails.Input) { in.Gender = "robot" }, "gender"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			err := profiledetails.New(&fakeUsers{}, &fakeProfiles{}, &fakeReviewer{}).Update(context.Background(), 1, in)
			var errs profiledetails.FieldErrors
			if !errors.As(err, &errs) {
				t.Fatalf("err = %v, want FieldErrors", err)
			}
			if _, ok := errs[tt.wantKey]; !ok {
				t.Errorf("no error for %q; got %v", tt.wantKey, errs)
			}
		})
	}
}
