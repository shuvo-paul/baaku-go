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
