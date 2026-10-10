package middleware

import (
	"context"
	"net/http"
)

// PermissionChecker answers named-permission checks for a user;
// *permission.Service satisfies it.
type PermissionChecker interface {
	Can(ctx context.Context, userID int64, permission string) (bool, error)
}

// RequirePermission mirrors spatie's PermissionMiddleware for a single
// permission: an authenticated user lacking it gets 403, holders pass.
// Guests cannot reach these routes (auth middleware runs first); a missing
// user is treated as unauthorized, matching spatie's deny-by-default.
func RequirePermission(checker PermissionChecker, permission string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			ok, err := checker.Can(r.Context(), u.ID, permission)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// PermissionLister loads every effective permission name for a user;
// *permission.Repo satisfies it. LoadPermissions stores the result in the
// request context so views can gate nav items like the reference's
// auth()->user()->can(…) checks in layouts/dashboard.blade.php.
type PermissionLister interface {
	UserPermissionNames(ctx context.Context, userID int64) ([]string, error)
}

type permsCtxKey struct{}

// PermissionsFromContext returns the names LoadPermissions stashed (nil for
// guests or when the middleware did not run).
func PermissionsFromContext(ctx context.Context) []string {
	names, _ := ctx.Value(permsCtxKey{}).([]string)
	return names
}

// LoadPermissions stashes the authenticated user's effective permission
// names in the context. shortcut: spatie reads these from the cache table;
// we hit the DB once per request — same trade-off the permission service
// already documents.
func LoadPermissions(lister PermissionLister) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if u, ok := UserFromContext(ctx); ok {
				names, err := lister.UserPermissionNames(ctx, u.ID)
				if err != nil {
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				ctx = context.WithValue(ctx, permsCtxKey{}, names)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
