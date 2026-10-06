// Package auth implements authentication flows ported from the Laravel
// reference app (app/Actions/Fortify, app/Http/Requests).
package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

var nameRe = regexp.MustCompile(`^[A-Za-z\s]+$`)

// FieldErrors maps form field names (Laravel-style, e.g. "educations.0.end_year")
// to validation messages, mirroring the reference app's validator output.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, e[k])
	}
	return strings.Join(parts, "; ")
}

// Querier is the query surface registration needs; *generated.Queries bound
// to a transaction satisfies it.
type Querier interface {
	GetUserByEmail(ctx context.Context, email string) (generated.User, error)
	GetUserByPhone(ctx context.Context, phone *string) (generated.User, error)
	CreateUser(ctx context.Context, arg generated.CreateUserParams) (generated.User, error)
	CreateProfile(ctx context.Context, userID int64) (generated.Profile, error)
	CreateEducation(ctx context.Context, arg generated.CreateEducationParams) (generated.Education, error)
}

// EducationInput is one educations[] row from the register form.
type EducationInput struct {
	Level       string
	Institution string
	StudentID   *string
	Subject     string
	StartYear   int32
	StartMonth  *int16
	IsCurrent   bool
	EndYear     *int32
	EndMonth    *int16
}

// RegisterInput is the validated payload for user registration.
type RegisterInput struct {
	Name                 string
	Email                string
	Phone                string
	Password             string
	PasswordConfirmation string
	Educations           []EducationInput
}

// RegisterService registers new users.
type RegisterService struct {
	pool *pgxpool.Pool
}

func NewRegisterService(pool *pgxpool.Pool) *RegisterService { return &RegisterService{pool: pool} }

// Register validates in, then inserts user + profile + educations rows in a
// single transaction. Validation failures come back as FieldErrors.
func (s *RegisterService) Register(ctx context.Context, in RegisterInput) (generated.User, error) {
	if errs := in.Validate(); len(errs) > 0 {
		return generated.User{}, errs
	}

	hash, err := Hash(in.Password)
	if err != nil {
		return generated.User{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return generated.User{}, fmt.Errorf("auth: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	q := Querier(generated.New(tx))

	if _, err := q.GetUserByEmail(ctx, in.Email); err == nil {
		return generated.User{}, FieldErrors{"email": "The email has already been taken."}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return generated.User{}, fmt.Errorf("auth: check email: %w", err)
	}
	if _, err := q.GetUserByPhone(ctx, &in.Phone); err == nil {
		return generated.User{}, FieldErrors{"phone": "The phone has already been taken."}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return generated.User{}, fmt.Errorf("auth: check phone: %w", err)
	}

	// state defaults to 'unverified' via column default, matching UserState::Unverified.
	user, err := q.CreateUser(ctx, generated.CreateUserParams{
		Name:     in.Name,
		Email:    in.Email,
		Password: string(hash),
		Phone:    &in.Phone,
	})
	if err != nil {
		return generated.User{}, mapUniqueViolation(err)
	}

	profile, err := q.CreateProfile(ctx, user.ID)
	if err != nil {
		return generated.User{}, fmt.Errorf("auth: create profile: %w", err)
	}

	for _, ed := range in.Educations {
		if _, err := q.CreateEducation(ctx, generated.CreateEducationParams{
			ProfileID:   profile.ID,
			Level:       ed.Level,
			Institution: ed.Institution,
			StudentID:   ed.StudentID,
			Subject:     ed.Subject,
			IsCurrent:   ed.IsCurrent,
			StartYear:   ed.StartYear,
			StartMonth:  ed.StartMonth,
			EndYear:     ed.EndYear,
			EndMonth:    ed.EndMonth,
		}); err != nil {
			return generated.User{}, fmt.Errorf("auth: create education: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return generated.User{}, fmt.Errorf("auth: commit: %w", err)
	}
	return user, nil
}

// mapUniqueViolation turns the users unique-constraint races into field errors.
func mapUniqueViolation(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "users_email_unique":
			return FieldErrors{"email": "The email has already been taken."}
		case "users_phone_unique":
			return FieldErrors{"phone": "The phone has already been taken."}
		}
	}
	return err
}

// Validate mirrors RegisterUserRequest::rules plus the end_year-unless-current
// after-hook from CreateNewUser::create.
func (in RegisterInput) Validate() FieldErrors {
	errs := FieldErrors{}

	if in.Name == "" {
		errs["name"] = "The name field is required."
	} else if len(in.Name) > 255 {
		errs["name"] = "The name field must not be greater than 255 characters."
	} else if !nameRe.MatchString(in.Name) {
		errs["name"] = "Only letters (A–Z) and spaces are allowed."
	}

	switch {
	case in.Email == "":
		errs["email"] = "The email field is required."
	case len(in.Email) > 255:
		errs["email"] = "The email field must not be greater than 255 characters."
	case !emailRe.MatchString(in.Email):
		errs["email"] = "The email field must be a valid email address."
	}

	switch {
	case in.Phone == "":
		errs["phone"] = "The phone field is required."
	case len(in.Phone) > 20:
		errs["phone"] = "The phone field must not be greater than 20 characters."
	}

	validatePassword(in, errs)

	if len(in.Educations) < 1 {
		errs["educations"] = "The educations field must have at least 1 items."
	}
	for i, ed := range in.Educations {
		validateEducation(i, ed, errs)
	}

	if len(errs) == 0 {
		return nil
	}
	return errs
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func validatePassword(in RegisterInput, errs FieldErrors) {
	if in.Password == "" {
		errs["password"] = "The password field is required."
		return
	}
	if err := Validate(in.Password); err != nil {
		errs["password"] = err.Error()
		return
	}
	if err := Confirmed(in.Password, in.PasswordConfirmation); err != nil {
		errs["password"] = "The password field confirmation does not match."
	}
}

func validateEducation(i int, ed EducationInput, errs FieldErrors) {
	required := func(field, v string) {
		if strings.TrimSpace(v) == "" {
			errs[field] = fmt.Sprintf("The %s field is required.", field)
		} else if len(v) > 255 {
			errs[field] = fmt.Sprintf("The %s field must not be greater than 255 characters.", field)
		}
	}
	p := fmt.Sprintf("educations.%d.", i)

	required(p+"level", ed.Level)
	required(p+"institution", ed.Institution)
	required(p+"subject", ed.Subject)
	if ed.StudentID != nil && len(*ed.StudentID) > 255 {
		errs[p+"student_id"] = fmt.Sprintf("The %s field must not be greater than 255 characters.", p+"student_id")
	}

	switch {
	case ed.StartYear == 0:
		errs[p+"start_year"] = fmt.Sprintf("The %s field is required.", p+"start_year")
	case ed.StartYear < 1000 || ed.StartYear > 9999:
		errs[p+"start_year"] = fmt.Sprintf("The %s field must be 4 digits.", p+"start_year")
	}
	monthOk := func(m *int16) bool { return m == nil || (*m >= 1 && *m <= 12) }
	if !monthOk(ed.StartMonth) {
		errs[p+"start_month"] = fmt.Sprintf("The %s field must be between 1 and 12.", p+"start_month")
	}
	if !monthOk(ed.EndMonth) {
		errs[p+"end_month"] = fmt.Sprintf("The %s field must be between 1 and 12.", p+"end_month")
	}

	// end_year is nullable but required unless is_current (CreateNewUser after-hook).
	emptyYear := ed.EndYear == nil || *ed.EndYear == 0
	if emptyYear {
		if !ed.IsCurrent {
			errs[p+"end_year"] = fmt.Sprintf("The %s field is required.", p+"end_year")
		}
	} else if *ed.EndYear < 1000 || *ed.EndYear > 9999 {
		errs[p+"end_year"] = fmt.Sprintf("The %s field must be 4 digits.", p+"end_year")
	} else if ed.StartYear >= 1000 && *ed.EndYear < ed.StartYear {
		errs[p+"end_year"] = fmt.Sprintf("The %s field must be greater than or equal to %sstart_year.", p+"end_year", p)
	}
}
