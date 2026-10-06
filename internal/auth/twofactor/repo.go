package twofactor

import (
	"context"
	"strconv"
	"time"

	"github.com/shuvo-paul/baaku/internal/auth/user"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Repo is the sqlc-backed Store + ReplayGuard for the twofactor service.
// User-row writes delegate to *user.Repo (SetUserTwoFactor query).
type Repo struct {
	users *user.Repo
	q     *generated.Queries
}

func NewRepo(users *user.Repo, q *generated.Queries) *Repo {
	return &Repo{users: users, q: q}
}

func (r *Repo) GetByID(ctx context.Context, id int64) (user.User, error) {
	return r.users.GetByID(ctx, id)
}

func (r *Repo) SetTwoFactor(ctx context.Context, id int64, secret, recoveryCodes string, confirmedAt *time.Time) error {
	return r.users.SetTwoFactor(ctx, id, secret, recoveryCodes, confirmedAt)
}

// Claim records the code in the cache table (Fortify's replay-guard cache).
// Returns true when the code was newly claimed.
func (r *Repo) Claim(ctx context.Context, code string, usedAt, expiresAt time.Time) (bool, error) {
	n, err := r.q.Claim2FAReplayCode(ctx, generated.Claim2FAReplayCodeParams{
		Key:        code,
		Value:      strconv.FormatInt(usedAt.Unix(), 10),
		Expiration: expiresAt.Unix(),
		UsedAt:     usedAt.Unix(),
	})
	if err != nil {
		return false, err
	}
	return n == 1, nil
}
