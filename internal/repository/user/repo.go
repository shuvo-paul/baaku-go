package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/login"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Repo is a thin wrapper over the sqlc-generated users queries. pool backs
// multi-statement transactions (CreateRegistration); single-query methods use
// the queries bound to it.
type Repo struct {
	q    *generated.Queries
	pool *pgxpool.Pool
}

func NewRepo(q *generated.Queries, pool *pgxpool.Pool) *Repo { return &Repo{q: q, pool: pool} }

func (r *Repo) GetByID(ctx context.Context, id int64) (user.User, error) {
	u, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return user.User{}, err
	}
	return fromGenerated(u), nil
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (user.User, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return user.User{}, err
	}
	return fromGenerated(u), nil
}

// UserByEmail adapts GetByEmail to the login service's UserByEmail port:
// pgx.ErrNoRows becomes login.ErrUserNotFound so the service takes its
// constant-time failure path instead of leaking account existence.
func (r *Repo) UserByEmail(ctx context.Context, email string) (login.User, error) {
	u, err := r.GetByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return login.User{}, login.ErrUserNotFound
	}
	if err != nil {
		return login.User{}, err
	}
	return login.User{ID: u.ID, PasswordHash: u.PasswordHash}, nil
}

// TwoFactorConfirmed reports whether the user's 2FA enrolment is confirmed
// (login service's TwoFactorCheck port; users.two_factor_confirmed_at IS NOT
// NULL — a challenge is required before a session may be issued).
func (r *Repo) TwoFactorConfirmed(ctx context.Context, id int64) (bool, error) {
	u, err := r.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return u.TwoFactorConfirmedAt != nil, nil
}

func (r *Repo) Create(ctx context.Context, name, email, passwordHash string, phone *string) (user.User, error) {
	u, err := r.q.CreateUser(ctx, generated.CreateUserParams{
		Name:     name,
		Email:    email,
		Password: passwordHash,
		Phone:    phone,
	})
	if err != nil {
		return user.User{}, err
	}
	return fromGenerated(u), nil
}

func (r *Repo) UpdateState(ctx context.Context, id int64, state user.UserState) error {
	return r.q.UpdateUserState(ctx, generated.UpdateUserStateParams{ID: id, State: string(state)})
}

// SetPassword also clears remember_token, matching the reference query.
func (r *Repo) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	return r.q.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{ID: id, Password: passwordHash})
}

func (r *Repo) SetEmailVerified(ctx context.Context, id int64) error {
	return r.q.VerifyUserEmail(ctx, id)
}

// SetTwoFactor stores the 2FA secret, recovery codes, and confirmation time.
// ponytail: no NULL-clear path for disable-2FA; add a ClearTwoFactor query when
// the Fortify disable flow is ported.
// SetTwoFactor stores the 2FA secret, recovery codes, and confirmation time.
func (r *Repo) SetTwoFactor(ctx context.Context, id int64, secret, recoveryCodes string, confirmedAt *time.Time) error {
	var confirmed pgtype.Timestamp
	if confirmedAt != nil {
		confirmed = pgtype.Timestamp{Time: *confirmedAt, Valid: true}
	}
	return r.q.SetUserTwoFactor(ctx, generated.SetUserTwoFactorParams{
		ID:                     id,
		TwoFactorSecret:        &secret,
		TwoFactorRecoveryCodes: &recoveryCodes,
		TwoFactorConfirmedAt:   confirmed,
	})
}

// ClearTwoFactor wipes the 2FA secret, recovery codes, and confirmation time
// (Fortify DisableTwoFactorAuthentication).
func (r *Repo) ClearTwoFactor(ctx context.Context, id int64) error {
	return r.q.ClearUserTwoFactor(ctx, id)
}

// UpdateProfile saves name + email; a nil emailVerifiedAt clears the column
// (email changed), a non-nil value preserves it.
func (r *Repo) UpdateProfile(ctx context.Context, id int64, name, email string, emailVerifiedAt *time.Time) error {
	var verified pgtype.Timestamp
	if emailVerifiedAt != nil {
		verified = pgtype.Timestamp{Time: *emailVerifiedAt, Valid: true}
	}
	return r.q.UpdateUserProfile(ctx, generated.UpdateUserProfileParams{
		ID:              id,
		Name:            name,
		Email:           email,
		EmailVerifiedAt: verified,
	})
}

// RegistrationEducation is one educations[] row persisted with a new user.
type RegistrationEducation struct {
	Level       string
	Institution string
	StudentID   *string
	Subject     string
	IsCurrent   bool
	StartYear   int32
	StartMonth  *int16
	EndYear     *int32
	EndMonth    *int16
}

// RegistrationInput carries the rows CreateRegistration writes: user +
// profile + educations (reference: CreateNewUser + CreateProfile).
type RegistrationInput struct {
	Name         string
	Email        string
	Phone        string
	PasswordHash string
	Educations   []RegistrationEducation
}

// CreateRegistration inserts user + profile + educations in a single
// transaction. Duplicate email/phone surface as the users unique-constraint
// violation (23505), which callers map to field errors.
func (r *Repo) CreateRegistration(ctx context.Context, in RegistrationInput) (user.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return user.User{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	q := generated.New(tx)

	// state defaults to 'unverified' via column default, matching UserState::Unverified.
	u, err := q.CreateUser(ctx, generated.CreateUserParams{
		Name:     in.Name,
		Email:    in.Email,
		Password: in.PasswordHash,
		Phone:    &in.Phone,
	})
	if err != nil {
		return user.User{}, err
	}

	profile, err := q.CreateProfile(ctx, u.ID)
	if err != nil {
		return user.User{}, fmt.Errorf("create profile: %w", err)
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
			return user.User{}, fmt.Errorf("create education: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return user.User{}, fmt.Errorf("commit: %w", err)
	}
	return fromGenerated(u), nil
}

func fromGenerated(g generated.User) user.User {
	u := user.User{
		ID:            g.ID,
		Name:          g.Name,
		Email:         g.Email,
		Phone:         g.Phone,
		PasswordHash:  g.Password,
		State:         user.UserState(g.State),
		RememberToken: g.RememberToken,
	}
	if g.EmailVerifiedAt.Valid {
		t := g.EmailVerifiedAt.Time
		u.EmailVerifiedAt = &t
	}
	if g.TwoFactorSecret != nil {
		s := *g.TwoFactorSecret
		u.TwoFactorSecret = &s
	}
	if g.TwoFactorRecoveryCodes != nil {
		s := *g.TwoFactorRecoveryCodes
		u.TwoFactorRecoveryCodes = &s
	}
	if g.TwoFactorConfirmedAt.Valid {
		t := g.TwoFactorConfirmedAt.Time
		u.TwoFactorConfirmedAt = &t
	}
	return u
}
