// Package passwordreset ports Laravel's default password broker over the
// password_reset_tokens table: issue a 64-byte random hex token, verify it
// within a 60-minute window, and consume the row on successful use. The raw
// token is stored as issued (the reference hashes at rest, but bcrypt cannot
// hash a 128-char token and the task spec calls for the raw token upserted).
package passwordreset

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	prrepo "github.com/shuvo-paul/baaku/internal/repository/passwordreset"
	"github.com/shuvo-paul/baaku/internal/service/password"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// TokenBytes is the raw token size: 64 random bytes, hex-encoded on issue.
const TokenBytes = 64

// ValidityWindow mirrors config auth.passwords.users.expire = 60 minutes.
const ValidityWindow = 60 * time.Minute

var (
	// ErrUserNotFound is returned when no user has the given email.
	ErrUserNotFound = errors.New("auth/passwordreset: no user with that email")
	// ErrInvalidToken is returned when the token is missing, mismatched, or
	// outside the validity window.
	ErrInvalidToken = errors.New("auth/passwordreset: invalid or expired token")
)

// userStore is the slice of user.Repo the broker needs.
type userStore interface {
	GetByEmail(ctx context.Context, email string) (user.User, error)
	SetPassword(ctx context.Context, id int64, passwordHash string) error
}

// tokenStore is the password_reset_tokens persistence the broker needs.
type tokenStore interface {
	Upsert(ctx context.Context, email, token string) error
	GetByEmail(ctx context.Context, email string) (prrepo.Token, error)
	DeleteByEmail(ctx context.Context, email string) error
}

// Service implements the broker flow. now is a seam for window tests.
type Service struct {
	users  userStore
	tokens tokenStore
	now    func() time.Time
}

func New(users userStore, tokens tokenStore) *Service {
	return NewWithClock(users, tokens, time.Now)
}

// NewWithClock is New with an injectable clock; the window tests drive time
// through it.
func NewWithClock(users userStore, tokens tokenStore, now func() time.Time) *Service {
	return &Service{users: users, tokens: tokens, now: now}
}

// Issue generates a fresh reset token for email, replacing any previous one.
// The raw token is returned for delivery and stored as issued.
func (s *Service) Issue(ctx context.Context, email string) (string, error) {
	if _, err := s.users.GetByEmail(ctx, email); err != nil {
		return "", mapNotFound(err)
	}
	raw := make([]byte, TokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	return token, s.tokens.Upsert(ctx, email, token)
}

// Lookup reports whether rawToken is the current valid reset token for email.
func (s *Service) Lookup(ctx context.Context, email, rawToken string) (bool, error) {
	record, err := s.tokens.GetByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if s.expired(record.CreatedAt) {
		return false, nil
	}
	return subtle.ConstantTimeCompare([]byte(record.Token), []byte(rawToken)) == 1, nil
}

// Complete verifies email + rawToken, sets the new bcrypt password hash, and
// consumes the token row so it cannot be reused.
func (s *Service) Complete(ctx context.Context, email, rawToken, newPassword string) error {
	if err := password.Validate(newPassword); err != nil {
		return err
	}
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return mapNotFound(err)
	}
	record, err := s.tokens.GetByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidToken
	}
	if err != nil {
		return err
	}
	if s.expired(record.CreatedAt) || subtle.ConstantTimeCompare([]byte(record.Token), []byte(rawToken)) != 1 {
		return ErrInvalidToken
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	if err := s.users.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	return s.tokens.DeleteByEmail(ctx, email)
}

// expired reports whether createdAt is missing or older than ValidityWindow.
func (s *Service) expired(createdAt *time.Time) bool {
	return createdAt == nil || s.now().Sub(*createdAt) > ValidityWindow
}

func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	return err
}
