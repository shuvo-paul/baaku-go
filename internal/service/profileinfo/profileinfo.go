// Package profileinfo ports UpdateUserProfileInformation (reference:
// app/Actions/Fortify/UpdateUserProfileInformation.php): name + email only —
// the reference action handles no photo, and this port keeps that parity.
// Name must be latin letters/spaces (validation.name_latin_only), email must
// be unique ignoring self, and an email change nulls email_verified_at.
package profileinfo

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/service/register"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// NameLatinOnlyMessage mirrors the reference validation.name_latin_only
// message (reference/lang/en/validation.php).
const NameLatinOnlyMessage = "Only letters (A–Z) and spaces are allowed."

var nameRe = regexp.MustCompile(`^[A-Za-z\s]+$`)

// FieldErrors maps form field names to Laravel-style validation messages,
// FieldErrors maps form field names to Laravel-style validation messages,
// mirroring the reference validator output. Reused from the register
// service — same shape, same semantics.
type FieldErrors = register.FieldErrors

// UserStore is the slice of the user repository Update needs. *user.Repo
// satisfies it.
type UserStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
	UpdateProfile(ctx context.Context, id int64, name, email string, emailVerifiedAt *time.Time) error
}

// Service updates profile info.
type Service struct {
	users UserStore
}

func New(users UserStore) *Service { return &Service{users: users} }

// Update validates in and persists it exactly like the reference action:
// unique email ignoring self, email change clears email_verified_at.
// Validation failures come back as FieldErrors.
func (s *Service) Update(ctx context.Context, userID int64, name, email string) error {
	if errs := validate(name, email); len(errs) > 0 {
		return errs
	}

	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}

	verifiedAt := u.EmailVerifiedAt
	if email != u.Email {
		if other, err := s.users.GetByEmail(ctx, email); err == nil && other.ID != userID {
			return FieldErrors{"email": "The email has already been taken."}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		verifiedAt = nil // reference: email change nulls email_verified_at
	}

	return s.users.UpdateProfile(ctx, userID, name, email, verifiedAt)
}

// validate mirrors the reference rules: name required/string/max:255/latin
// regex, email required/string/email/max:255.
func validate(name, email string) FieldErrors {
	errs := FieldErrors{}

	switch {
	case name == "":
		errs["name"] = "The name field is required."
	case len(name) > 255:
		errs["name"] = "The name field must not be greater than 255 characters."
	case !nameRe.MatchString(name):
		errs["name"] = NameLatinOnlyMessage
	}

	switch {
	case email == "":
		errs["email"] = "The email field is required."
	case len(email) > 255:
		errs["email"] = "The email field must not be greater than 255 characters."
	case !validEmail(email):
		errs["email"] = "The email field must be a valid email address."
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}
