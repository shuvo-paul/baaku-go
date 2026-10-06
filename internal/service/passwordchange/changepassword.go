package passwordchange

import (
	"context"
	"errors"
	"fmt"

	"github.com/shuvo-paul/baaku/internal/service/password"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// ErrCurrentPasswordInvalid is returned when the current password does not
// match the stored hash (Fortify's current_password rule).
var ErrCurrentPasswordInvalid = errors.New("auth: current password is incorrect")

// passwordStore is the storage dependency for ChangePassword. In-package so
// tests can substitute a fake; user.Repo satisfies it structurally.
type passwordStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	SetPassword(ctx context.Context, id int64, passwordHash string) error
}

// ChangePassword replaces the user's password, mirroring Fortify's
// UpdateUserPassword: current password must match, the new password must
// pass the Laravel password rules and be confirmed, then it is bcrypt-hashed
// and saved via SetPassword (which also clears remember_token).
func ChangePassword(ctx context.Context, store passwordStore, userID int64, current, new, confirm string) error {
	u, err := store.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("auth: get user: %w", err)
	}
	if !password.Compare(u.PasswordHash, current) {
		return ErrCurrentPasswordInvalid
	}
	if err := password.Validate(new); err != nil {
		return err
	}
	if err := password.Confirmed(new, confirm); err != nil {
		return err
	}
	h, err := password.Hash(new)
	if err != nil {
		return err
	}
	return store.SetPassword(ctx, userID, h)
}
