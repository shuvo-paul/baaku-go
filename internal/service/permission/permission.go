// Package permission ports the spatie/laravel-permission checks the dashboard
// routes rely on (route middleware `permission:…`). Reference behavior
// (Spatie\Permission\Middleware\PermissionMiddleware + HasRoles): a user
// passes when the permission name is granted directly or via any assigned
// role; failure aborts 403.
package permission

import "context"

// Store loads a user's effective permission names. *permission.Repo satisfies it.
type Store interface {
	UserPermissionNames(ctx context.Context, userID int64) ([]string, error)
}

// Service answers permission checks for authenticated users.
type Service struct {
	store Store
}

func New(store Store) *Service { return &Service{store: store} }

// Can reports whether the user holds the named permission.
func (s *Service) Can(ctx context.Context, userID int64, permission string) (bool, error) {
	// shortcut: spatie caches the permission catalog in the cache table and
	// busts it on role/permission writes; we hit the DB per check. Upgrade
	// when permission writes become frequent enough to measure.
	names, err := s.store.UserPermissionNames(ctx, userID)
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == permission {
			return true, nil
		}
	}
	return false, nil
}
