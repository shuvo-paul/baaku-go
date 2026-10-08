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
	"fmt"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	sessid "github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// PendingKey is the session payload key holding the user id awaiting the 2FA
// challenge. Begin creates the pending session; handlers must not build the
// payload themselves.
const PendingKey = "login.two_factor"

// RememberKey is the payload flag the login form stashes when the user asked
// to be remembered (Fortify's login.remember); Challenge turns it into the
// recaller cookie after the code checks out.
const RememberKey = "login.remember"

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

// RememberStore is the user persistence remember-me needs (*user.Repo
// satisfies it).
type RememberStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	SetRememberToken(ctx context.Context, id int64, token *string) error
}

// ChallengeResult is the outcome of a successful Challenge. RememberToken +
// PasswordHash are set only when the pending login asked to be remembered —
// the handler queues the recaller cookie from them.
type ChallengeResult struct {
	UserID        int64
	RememberToken string
	PasswordHash  string
}

// Service orchestrates the login 2FA challenge.
type Service struct {
	sessions SessionStore
	verify   Verifier
	users    RememberStore
}

func New(sessions SessionStore, verify Verifier, users RememberStore) *Service {
	return &Service{sessions: sessions, verify: verify, users: users}
}

// PendingPayload returns the session payload for a pending-2FA login.
func PendingPayload(userID int64, remember bool) string {
	m := map[string]any{PendingKey: userID}
	if remember {
		m[RememberKey] = true
	}
	b, _ := json.Marshal(m) //nolint:errcheck // map is always marshalable
	return string(b)
}

// Begin opens the pending-2FA session for userID and returns its ID
// (Fortify's login response stashing login.id before the challenge). The row
// has user_id NULL — every auth guard treats it as a guest until Challenge
// promotes it. The login handler calls this on login.Login's TwoFactorPending.
func (s *Service) Begin(ctx context.Context, userID int64, remember bool) (string, error) {
	id, err := sessid.NewSessionID()
	if err != nil {
		return "", err
	}
	err = s.sessions.Create(ctx, generated.UpsertSessionParams{
		ID:           id,
		Payload:      PendingPayload(userID, remember),
		LastActivity: int32(time.Now().Unix()),
	})
	if err != nil {
		return "", err
	}
	return id, nil
}

// Challenge verifies code against the pending session's user and, on success,
// promotes the session: user_id set, pending flags cleared. An invalid code
// returns the verifier's error and leaves the session untouched so the user
// can retry. When the pending login asked to be remembered, the recaller
// token is ensured before the promote so a storage failure leaves the
// challenge retryable (Fortify queues the recaller cookie only after the
// challenge succeeds).
func (s *Service) Challenge(ctx context.Context, sessionID, code string) (ChallengeResult, error) {
	sess, err := s.sessions.Load(ctx, sessionID)
	if errors.Is(err, session.ErrNotFound) {
		return ChallengeResult{}, ErrNoPendingChallenge
	}
	if err != nil {
		return ChallengeResult{}, err
	}
	if sess.UserID != nil {
		return ChallengeResult{}, ErrNoPendingChallenge
	}
	userID, ok := pendingUserID(sess.Payload)
	if !ok {
		return ChallengeResult{}, ErrNoPendingChallenge
	}
	if err := s.verify.Verify(ctx, userID, code); err != nil {
		return ChallengeResult{}, err
	}
	res := ChallengeResult{UserID: userID}
	if rememberRequested(sess.Payload) {
		u, err := s.users.GetByID(ctx, userID)
		if err != nil {
			return ChallengeResult{}, fmt.Errorf("twofactorchallenge: get user: %w", err)
		}
		token, err := sessid.EnsureRememberToken(ctx, s.users, userID, derefToken(u.RememberToken))
		if err != nil {
			return ChallengeResult{}, fmt.Errorf("twofactorchallenge: remember token: %w", err)
		}
		res.RememberToken, res.PasswordHash = token, u.PasswordHash
	}
	if err := s.sessions.Promote(ctx, sessionID, userID, payloadWithoutPending(sess.Payload)); err != nil {
		return ChallengeResult{}, err
	}
	return res, nil
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

// rememberRequested reads the remember-me flag the login form stashed in the
// pending session payload.
func rememberRequested(payload string) bool {
	var p map[string]any
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return false
	}
	remember, _ := p[RememberKey].(bool)
	return remember
}

func derefToken(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// payloadWithoutPending strips the pending-login flags, preserving every other
// payload key (e.g. flash).
func payloadWithoutPending(payload string) string {
	var p map[string]any
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return "{}"
	}
	delete(p, PendingKey)
	delete(p, RememberKey)
	b, err := json.Marshal(p)
	if err != nil {
		return "{}"
	}
	return string(b)
}
