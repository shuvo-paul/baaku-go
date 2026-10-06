// Package session implements the database-backed session store matching
// Laravel's sessions table (reference/config/session.php).
package session

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// ErrNotFound is returned by Load when no session exists for the ID.
var ErrNotFound = errors.New("session not found")

// Store persists sessions via sqlc queries.
type Store struct {
	q *generated.Queries
}

func NewStore(q *generated.Queries) *Store { return &Store{q: q} }

// Create inserts a new session row. Session IDs are 32 random bytes, so a
// collision is practically impossible; this upserts like Save rather than
// erroring on the off chance one occurs.
func (s *Store) Create(ctx context.Context, p generated.UpsertSessionParams) error {
	return s.q.UpsertSession(ctx, p)
}

// Load returns the session row for id, or ErrNotFound.
func (s *Store) Load(ctx context.Context, id string) (generated.Session, error) {
	sess, err := s.q.GetSessionByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return generated.Session{}, ErrNotFound
	}
	return sess, err
}

// Save upserts the session row (Laravel refreshes last_activity on every write).
func (s *Store) Save(ctx context.Context, p generated.UpsertSessionParams) error {
	return s.q.UpsertSession(ctx, p)
}

// Destroy deletes the session row. Deleting a missing ID is not an error.
func (s *Store) Destroy(ctx context.Context, id string) error {
	return s.q.DeleteSessionByID(ctx, id)
}

// GC deletes sessions whose last_activity is older than lifetime before now.
func (s *Store) GC(ctx context.Context, now time.Time, lifetime time.Duration) error {
	return s.q.DeleteExpiredSessions(ctx, int32(expiryCutoff(now, lifetime)))
}

// expiryCutoff returns the unix timestamp before which sessions are expired.
func expiryCutoff(now time.Time, lifetime time.Duration) int64 {
	return now.Add(-lifetime).Unix()
}
