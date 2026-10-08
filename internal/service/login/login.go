// Package auth orchestrates credential login/logout for the session guard.
//
// Port of the Laravel reference app's Fortify login flow
// (reference/config/auth.php, config/fortify.php): retrieve the user by
// email, bcrypt-verify the password, then either open a session or — when
// the user has a confirmed two-factor enrolment
// (users.two_factor_confirmed_at IS NOT NULL) — return a pending-2FA state
// for the challenge flow (Task 08) to handle.
package login

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	sesssvc "github.com/shuvo-paul/baaku/internal/service/session"
)

// ErrInvalidCredentials is returned for both unknown emails and wrong
// passwords, mirroring Laravel's generic "auth.failed" message so login
// cannot be used to enumerate accounts.
var ErrInvalidCredentials = errors.New("auth: invalid email or password")

// ErrUserNotFound is returned by UserByEmail implementations when no user
// matches the email (database wiring maps pgx.ErrNoRows to it).
var ErrUserNotFound = errors.New("auth: user not found")

// User carries the credential fields Login needs from the users table.
type User struct {
	ID           int64
	PasswordHash string
	// RememberToken is the current remember-me token ("" when none).
	RememberToken string
}

// UserStore looks up users by email (ErrUserNotFound when absent) and
// persists remember-me tokens.
type UserStore interface {
	UserByEmail(ctx context.Context, email string) (User, error)
	SetRememberToken(ctx context.Context, id int64, token *string) error
}

// SessionStore creates and destroys session rows (public.sessions).
type SessionStore interface {
	// CreateSession stores a new session for userID and returns its ID.
	CreateSession(ctx context.Context, userID int64) (string, error)
	// DeleteSession destroys the session; deleting an unknown ID is a no-op.
	DeleteSession(ctx context.Context, sessionID string) error
}

// TwoFactorCheck reports whether a user's two-factor enrolment is confirmed
// and a challenge is required before a session may be issued.
type TwoFactorCheck interface {
	TwoFactorConfirmed(ctx context.Context, userID int64) (bool, error)
}

// LoginResult is the outcome of Login.
type LoginResult struct {
	UserID int64
	// SessionID is set only on success; empty when TwoFactorPending.
	SessionID string
	// TwoFactorPending means credentials were valid but no session was
	// created: the user must pass the 2FA challenge (Task 08) first.
	TwoFactorPending bool
	// RememberToken + PasswordHash are set only when the caller asked to be
	// remembered and the session opened outright — the handler builds the
	// recaller cookie from them.
	RememberToken string
	PasswordHash  string
}

// Service orchestrates login/logout. The ports are declared here
// (consumer-side interfaces) so this package has no dependency on the
// repository or challenge implementations from other tasks.
type Service struct {
	users    UserStore
	sessions SessionStore
	twofa    TwoFactorCheck
}

func New(users UserStore, sessions SessionStore, twofa TwoFactorCheck) *Service {
	return &Service{users: users, sessions: sessions, twofa: twofa}
}

// dummyHash is a bcrypt hash of an arbitrary string at cost 12 (Laravel's
// default BCRYPT_ROUNDS). Compared when the email is unknown so both
// credential-failure paths take the same time.
var dummyHash = []byte("$2a$12$RJhIKhiLp8/zkdfh65b0l.MniCWNeYy2pFgxDQ5JmZrMMWScNh92O")

// Login verifies email+password and opens a session, or reports that a
// two-factor challenge is pending instead. remember requests the long-lived
// recaller cookie (Laravel SessionGuard::login($user, $remember)); the token
// is ensured here and returned on the result so the handler can queue it.
func (s *Service) Login(ctx context.Context, email, password string, remember bool) (LoginResult, error) {
	user, err := s.users.UserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, fmt.Errorf("auth: lookup user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	pending, err := s.twofa.TwoFactorConfirmed(ctx, user.ID)
	if err != nil {
		// Fail closed: never issue a session when 2FA status is unknown.
		return LoginResult{}, fmt.Errorf("auth: two-factor check: %w", err)
	}
	if pending {
		// The recaller cookie is queued only after the challenge succeeds
		// (Fortify stashes login.remember in the pending session).
		return LoginResult{UserID: user.ID, TwoFactorPending: true}, nil
	}

	res := LoginResult{UserID: user.ID}
	if remember {
		token, err := sesssvc.EnsureRememberToken(ctx, s.users, user.ID, user.RememberToken)
		if err != nil {
			return LoginResult{}, fmt.Errorf("auth: remember token: %w", err)
		}
		res.RememberToken, res.PasswordHash = token, user.PasswordHash
	}

	sid, err := s.sessions.CreateSession(ctx, user.ID)
	if err != nil {
		return LoginResult{}, fmt.Errorf("auth: create session: %w", err)
	}
	res.SessionID = sid
	return res, nil
}

// Logout destroys the session identified by sessionID and — like Laravel's
// SessionGuard::logout, which cycles the remember token — clears the user's
// remember token when known, killing every outstanding recaller cookie.
func (s *Service) Logout(ctx context.Context, sessionID string, userID *int64) error {
	if err := s.sessions.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("auth: destroy session: %w", err)
	}
	if userID != nil {
		if err := s.users.SetRememberToken(ctx, *userID, nil); err != nil {
			return fmt.Errorf("auth: clear remember token: %w", err)
		}
	}
	return nil
}
