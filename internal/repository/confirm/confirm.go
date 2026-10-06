// Package confirm implements the password-confirmation cache behind Fortify's
// confirmPassword flow. Reference behavior (laravel/fortify v1.39
// ConfirmablePasswordController, Illuminate\Auth\Middleware\RequirePassword):
// after a user confirms their password, sensitive routes stay accessible for
// config('auth.password_timeout') seconds (default 10800). The reference stores
// the confirmation timestamp in the session; this port stores it as a row in
// the cache table (same TTL semantics).
package confirm

import (
	"context"
	"strconv"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// keyPrefix is the cache key namespace for password confirmations.
const keyPrefix = "password.confirmation."

// Repo is a thin wrapper over the sqlc-generated cache queries.
type Repo struct {
	q   *generated.Queries
	ttl time.Duration
}

// NewRepo returns a Repo whose confirmations stay valid for ttl
// (config AUTH_PASSWORD_TIMEOUT, default 3 hours).
func NewRepo(q *generated.Queries, ttl time.Duration) *Repo {
	return &Repo{q: q, ttl: ttl}
}

// MarkConfirmed records a password confirmation for userID, valid for r.ttl.
func (r *Repo) MarkConfirmed(ctx context.Context, userID int64) error {
	markedAt := time.Now()
	return r.q.UpsertPasswordConfirmation(ctx, generated.UpsertPasswordConfirmationParams{
		Key:        key(userID),
		Value:      strconv.FormatInt(markedAt.Unix(), 10),
		Expiration: markedAt.Add(r.ttl).Unix(),
	})
}

// Confirmed reports whether userID confirmed their password within the TTL.
// A missing row, expired entry, or storage error all read as unconfirmed:
// this gates sensitive routes, so it fails closed.
func (r *Repo) Confirmed(ctx context.Context, userID int64) bool {
	c, err := r.q.GetPasswordConfirmation(ctx, key(userID))
	if err != nil { // includes pgx.ErrNoRows
		return false
	}
	return ValidAt(c.Expiration, time.Now().Unix())
}

// key builds the cache row key for a user's password confirmation.
func key(userID int64) string {
	return keyPrefix + strconv.FormatInt(userID, 10)
}

// ValidAt reports whether a confirmation marked with the given unix expiration
// is still valid at now. Mirrors RequirePassword: confirmed iff
// now - markedAt < timeout, i.e. now < expiration.
func ValidAt(expiration, now int64) bool {
	return expiration > now
}
