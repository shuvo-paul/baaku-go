// Package twofactorchallenge orchestrates the login 2FA challenge.
//
// login.Login reports TwoFactorPending without opening a session; the pending
// state rides in the session row instead — user_id NULL, payload key
// "login.two_factor" = user id (Laravel stores login.id in the session
// payload). Challenge verifies the code against twofactor.Service and, on
// success, promotes the session to authenticated (Fortify
// TwoFactorLoginController::store).
package twofactorchallenge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	sessid "github.com/shuvo-paul/baaku/internal/service/session"
)

// PendingKey is the session payload key holding the user id awaiting the 2FA
// challenge. Begin creates the pending session; handlers must not build the
// payload themselves.
const PendingKey = "login.two_factor"

// ErrNoPendingChallenge is returned when the session isn't a pending-2FA
// session (missing row, already authenticated, or no pending flag).
var ErrNoPendingChallenge = errors.New("twofactorchallenge: no pending two-factor challenge")

// SessionStore loads and promotes session rows. *session.Store satisfies it.
type SessionStore interface {
	Load(ctx context.Context, id string) (generated.Session, error)
	Promote(ctx context.Context, id string, userID int64, payload string) error
	// Create inserts a new session row (Begin's pending-login sessions).
	Create(ctx context.Context, p generated.UpsertSessionParams) error
}

// Verifier checks the challenge code. *twofactor.Service satisfies it (valid
// TOTP, or a recovery code consumed exactly once).
type Verifier interface {
	Verify(ctx context.Context, userID int64, code string) error
}

// Service orchestrates the login 2FA challenge.
type Service struct {
	sessions SessionStore
	verify   Verifier
}

func New(sessions SessionStore, verify Verifier) *Service {
	return &Service{sessions: sessions, verify: verify}
}

// PendingPayload returns the session payload for a pending-2FA login.
func PendingPayload(userID int64) string {
	b, _ := json.Marshal(map[string]int64{PendingKey: userID}) //nolint:errcheck // map is always marshalable
	return string(b)
}

// Begin opens the pending-2FA session for userID and returns its ID
// (Fortify's login response stashing login.id before the challenge). The row
// has user_id NULL — every auth guard treats it as a guest until Challenge
// promotes it. The login handler calls this on login.Login's TwoFactorPending.
func (s *Service) Begin(ctx context.Context, userID int64) (string, error) {
	id, err := sessid.NewSessionID()
	if err != nil {
		return "", err
	}
	err = s.sessions.Create(ctx, generated.UpsertSessionParams{
		ID:           id,
		Payload:      PendingPayload(userID),
		LastActivity: int32(time.Now().Unix()),
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Challenge verifies code against the pending session's user and, on success,
// promotes the session: user_id set, pending flag cleared. An invalid code
// returns the verifier's error and leaves the session untouched so the user
// can retry.
func (s *Service) Challenge(ctx context.Context, sessionID, code string) error {
	sess, err := s.sessions.Load(ctx, sessionID)
	if errors.Is(err, session.ErrNotFound) {
		return ErrNoPendingChallenge
	}
	if err != nil {
		return err
	}
	if sess.UserID != nil {
		return ErrNoPendingChallenge
	}
	userID, ok := pendingUserID(sess.Payload)
	if !ok {
		return ErrNoPendingChallenge
	}
	if err := s.verify.Verify(ctx, userID, code); err != nil {
		return err
	}
	return s.sessions.Promote(ctx, sessionID, userID, payloadWithoutPending(sess.Payload))
}

// pendingUserID reads the pending-login flag from the session payload JSON.
func pendingUserID(payload string) (int64, bool) {
	var p map[string]any
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return 0, false
	}
	switch v := p[PendingKey].(type) {
	case float64:
		return int64(v), v > 0
	case json.Number:
		n, err := v.Int64()
		return n, err == nil && n > 0
	}
	return 0, false
}

// payloadWithoutPending strips the pending-login flag, preserving every other
// payload key (e.g. flash).
func payloadWithoutPending(payload string) string {
	var p map[string]any
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "{}"
	}
	delete(p, PendingKey)
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}
