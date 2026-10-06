package user

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Repo is a thin wrapper over the sqlc-generated users queries.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) GetByID(ctx context.Context, id int64) (User, error) {
	u, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	return fromGenerated(u), nil
}

func (r *Repo) GetByEmail(ctx context.Context, email string) (User, error) {
	u, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, err
	}
	return fromGenerated(u), nil
}

func (r *Repo) Create(ctx context.Context, name, email, passwordHash string, phone *string) (User, error) {
	u, err := r.q.CreateUser(ctx, generated.CreateUserParams{
		Name:     name,
		Email:    email,
		Password: passwordHash,
		Phone:    phone,
	})
	if err != nil {
		return User{}, err
	}
	return fromGenerated(u), nil
}

func (r *Repo) UpdateState(ctx context.Context, id int64, state UserState) error {
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

func fromGenerated(g generated.User) User {
	u := User{
		ID:            g.ID,
		Name:          g.Name,
		Email:         g.Email,
		Phone:         g.Phone,
		PasswordHash:  g.Password,
		State:         UserState(g.State),
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
