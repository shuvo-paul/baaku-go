// Package session implements the database-backed session store matching
// Laravel's sessions table (reference/config/session.php).
package session

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	sessid "github.com/shuvo-paul/baaku/internal/service/session"
)

// ErrNotFound is returned by Load when no session exists for the ID.
var ErrNotFound = errors.New("session not found")

// Store persists sessions via sqlc queries. lifetime is the server-side
// session validity window (SESSION_LIFETIME): Load rejects rows whose
// last_activity is older than it, matching Laravel's Store::isValid —
// a leaked session ID dies with the window regardless of the cookie.
type Store struct {
	q        *generated.Queries
	lifetime time.Duration
}

func NewStore(q *generated.Queries, lifetime time.Duration) *Store {
	return &Store{q: q, lifetime: lifetime}
}

// Create inserts a new session row. Session IDs are 32 random bytes, so a
// collision is practically impossible; this upserts like Save rather than
// erroring on the off chance one occurs.
func (s *Store) Create(ctx context.Context, p generated.UpsertSessionParams) error {
	return s.q.UpsertSession(ctx, p)
}

// Load returns the session row for id, or ErrNotFound — also for rows whose
// last_activity fell outside the lifetime window (expired server-side).
func (s *Store) Load(ctx context.Context, id string) (generated.Session, error) {
	cutoff := int32(expiryCutoff(time.Now(), s.lifetime))
	sess, err := s.q.GetSessionByID(ctx, generated.GetSessionByIDParams{ID: id, Cutoff: cutoff})
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

// CreateSession adapts Create to the login service's SessionStore port: a new
// session ID plus an empty authenticated session row for userID.
func (s *Store) CreateSession(ctx context.Context, userID int64) (string, error) {
	id, err := sessid.NewSessionID()
	if err != nil {
		return "", err
	}
	err = s.Create(ctx, generated.UpsertSessionParams{
		ID:           id,
		UserID:       &userID,
		Payload:      "{}",
		LastActivity: int32(time.Now().Unix()),
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// DeleteSession adapts Destroy to the login service's SessionStore port.
func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.Destroy(ctx, sessionID)
}

// GC deletes sessions whose last_activity is older than the store's lifetime.
// main.go sweeps it periodically (Laravel uses the per-request session lottery).
func (s *Store) GC(ctx context.Context, now time.Time) error {
	return s.q.DeleteExpiredSessions(ctx, int32(expiryCutoff(now, s.lifetime)))
}

// Promote attaches an authenticated user to a pending session row and
// replaces its payload (2FA challenge success clears the pending flag).
func (s *Store) Promote(ctx context.Context, id string, userID int64, payload string) error {
	return s.q.PromoteSession(ctx, generated.PromoteSessionParams{
		UserID:  &userID,
		Payload: payload,
		ID:      id,
	})
}

// expiryCutoff returns the unix timestamp before which sessions are expired.
func expiryCutoff(now time.Time, lifetime time.Duration) int64 {
	return now.Add(-lifetime).Unix()
}
