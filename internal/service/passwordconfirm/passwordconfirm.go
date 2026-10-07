// Package passwordconfirm ports Fortify's confirmPassword flow at the service
// layer: bcrypt-check the password, then record the confirmation in the
// cache-backed repo (internal/repository/confirm) for config
// auth.password_timeout seconds (default 10800, reference/config/auth.php).
package passwordconfirm

import (
	"context"
	"errors"

	"github.com/shuvo-paul/baaku/internal/service/password"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// ErrInvalidPassword is returned when the password doesn't match the user's
// bcrypt hash (Fortify's "auth.failed" for confirmPassword).
var ErrInvalidPassword = errors.New("passwordconfirm: invalid password")

// userStore is the slice of the user repository Confirm needs.
type userStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

// confirmStore is the cache-backed confirmation persistence
// (internal/repository/confirm). TTL lives in the repo, not here.
type confirmStore interface {
	MarkConfirmed(ctx context.Context, userID int64) error
	Confirmed(ctx context.Context, userID int64) bool
}

// Service orchestrates the password-confirmation flow.
type Service struct {
	users    userStore
	confirms confirmStore
}

func New(users userStore, confirms confirmStore) *Service {
	return &Service{users: users, confirms: confirms}
}

// Confirm checks raw against the user's bcrypt hash and records the
// confirmation (Fortify ConfirmablePasswordController::store).
func (s *Service) Confirm(ctx context.Context, userID int64, raw string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if !password.Compare(u.PasswordHash, raw) {
		return ErrInvalidPassword
	}
	return s.confirms.MarkConfirmed(ctx, userID)
}

// Check reports whether the user confirmed their password within the TTL
// (what RequirePassword-style gates ask).
func (s *Service) Check(ctx context.Context, userID int64) bool {
	return s.confirms.Confirmed(ctx, userID)
}
