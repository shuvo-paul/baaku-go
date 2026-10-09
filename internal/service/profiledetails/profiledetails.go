// Package profiledetails ports ProfileDetailsController@update (reference) —
// the "profile details" tab of dashboard/profile. It updates name/email/phone
// on the parent user and every details column on the profile (including the
// json columns and photo), then re-submits a rejected profile for review.
//
// Validation mirrors ProfileDetailsRequest.
package profiledetails

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/repository/profile"
	"github.com/shuvo-paul/baaku/internal/service/register"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors = register.FieldErrors

// NameLatinOnlyMessage mirrors validation.name_latin_only.
const NameLatinOnlyMessage = "Only letters (A–Z) and spaces are allowed."

var nameRe = regexp.MustCompile(`^[A-Za-z\s]+$`)

// Values mirror app/Enums/Gender.php and app/Enums/BloodGroup.php (nullable
// here — unlike the complete-profile gate, these are optional on the details
// form).
var (
	genders     = map[string]bool{"male": true, "female": true, "other": true, "prefer_not_to_say": true}
	bloodGroups = map[string]bool{"A+": true, "A-": true, "B+": true, "B-": true, "AB+": true, "AB-": true, "O+": true, "O-": true}
)

// Input is the details form payload.
type Input struct {
	Name             string
	Email            string
	Phone            string
	DateOfBirth      string // YYYY-MM-DD, "" when unset
	Gender           string
	BloodGroup       string
	PresentAddress   string
	PermanentAddress string
	Website          string
	SocialLinks      map[string]string
	EmergencyContact map[string]string
	LocalNames       map[string]string
	PhotoPath        *string // set by the handler after storing an upload
}

// UserStore is the user-side slice the service needs.
type UserStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	GetByEmail(ctx context.Context, email string) (user.User, error)
	UpdateProfile(ctx context.Context, id int64, name, email string, emailVerifiedAt *time.Time) error
	UpdateContact(ctx context.Context, id int64, phone *string) error
}

// ProfileStore is the profile-side slice the service needs.
type ProfileStore interface {
	Get(ctx context.Context, userID int64) (profile.Profile, error)
	UpsertDetailsFull(ctx context.Context, p profile.Profile) error
}

// Reviewer is the SubmitProfileForReview port.
type Reviewer interface {
	Submit(ctx context.Context, userID int64) error
}

type Service struct {
	users    UserStore
	profiles ProfileStore
	reviewer Reviewer
}

func New(users UserStore, profiles ProfileStore, reviewer Reviewer) *Service {
	return &Service{users: users, profiles: profiles, reviewer: reviewer}
}

// Update validates in, persists the user fields then the profile fields, and
// re-submits a rejected profile for review (reference
// ProfileDetailsController@update).
func (s *Service) Update(ctx context.Context, userID int64, in Input) error {
	if errs := validate(in); len(errs) > 0 {
		return errs
	}

	// Parent user: name + email (email change clears email_verified_at) + phone.
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	verifiedAt := u.EmailVerifiedAt
	if in.Email != u.Email {
		if other, err := s.users.GetByEmail(ctx, in.Email); err == nil && other.ID != userID {
			return FieldErrors{"email": "The email has already been taken."}
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		verifiedAt = nil
	}
	if err := s.users.UpdateProfile(ctx, userID, in.Name, in.Email, verifiedAt); err != nil {
		return err
	}
	if err := s.users.UpdateContact(ctx, userID, phoneOrNull(in.Phone)); err != nil {
		return err
	}

	// Profile row (firstOrCreate semantics via upsert).
	cur, err := s.profiles.Get(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	p := profile.Profile{
		ID:               cur.ID,
		UserID:           userID,
		PhotoPath:        cur.PhotoPath,
		DateOfBirth:      in.DateOfBirth,
		Gender:           strOrNull(in.Gender),
		BloodGroup:       strOrNull(in.BloodGroup),
		PresentAddress:   &in.PresentAddress,
		PermanentAddress: &in.PermanentAddress,
		SocialLinks:      filterMap(in.SocialLinks),
		Website:          strOrNull(in.Website),
		EmergencyContact: filterMap(in.EmergencyContact),
		LocalNames:       filterMap(in.LocalNames),
	}
	if in.PhotoPath != nil {
		p.PhotoPath = in.PhotoPath
	}
	if err := s.profiles.UpsertDetailsFull(ctx, p); err != nil {
		return err
	}

	return s.reviewer.Submit(ctx, userID)
}

// PhotoPathOf returns the current stored photo path ("" when none) so the
// handler can delete a replaced file.
func (s *Service) PhotoPathOf(ctx context.Context, userID int64) (string, error) {
	p, err := s.profiles.Get(ctx, userID)
	if err != nil {
		return "", err
	}
	if p.PhotoPath == nil {
		return "", nil
	}
	return *p.PhotoPath, nil
}

// validate mirrors ProfileDetailsRequest. present/permanent addresses are
// required; name/email/phone are required+unique-ignoring-self; gender/
// blood_group/website/date_of_birth are nullable.
func validate(in Input) FieldErrors {
	errs := FieldErrors{}

	switch {
	case in.Name == "":
		errs["name"] = "The name field is required."
	case len(in.Name) > 255:
		errs["name"] = "The name field must not be greater than 255 characters."
	case !nameRe.MatchString(in.Name):
		errs["name"] = NameLatinOnlyMessage
	}

	switch {
	case in.Email == "":
		errs["email"] = "The email field is required."
	case len(in.Email) > 255:
		errs["email"] = "The email field must not be greater than 255 characters."
	case !validEmail(in.Email):
		errs["email"] = "The email field must be a valid email address."
	}

	switch {
	case in.Phone == "":
		errs["phone"] = "The phone field is required."
	case len(in.Phone) > 20:
		errs["phone"] = "The phone field must not be greater than 20 characters."
	}

	if in.Gender != "" && !genders[in.Gender] {
		errs["gender"] = "The selected gender is invalid."
	}
	if in.BloodGroup != "" && !bloodGroups[in.BloodGroup] {
		errs["blood_group"] = "The selected blood group is invalid."
	}
	if in.Website != "" && len(in.Website) > 255 {
		errs["website"] = "The website field must not be greater than 255 characters."
	}

	address("present_address", "present address", in.PresentAddress, errs)
	address("permanent_address", "permanent address", in.PermanentAddress, errs)

	return errs
}

func address(field, label, v string, errs FieldErrors) {
	switch {
	case v == "":
		errs[field] = "The " + label + " field is required."
	case len(v) > 255:
		errs[field] = "The " + label + " field must not be greater than 255 characters."
	}
}

func validEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}

func strOrNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func phoneOrNull(s string) *string { return strOrNull(s) }

// filterMap drops empty values (reference array_filter non-empty); an all-
// empty map becomes nil → SQL NULL.
func filterMap(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		if v != "" {
			out[k] = v
		}
	}
	return out
}
