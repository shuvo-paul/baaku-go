// Package middleware provides chi auth guards mirroring the reference app's
// Fortify auth/verified middleware and App\Http\Middleware\CheckUserApproved /
// CheckUserSuspended (reference/bootstrap/app.php route aliases).
//
// Redirect targets are path stubs until routes and views exist.
package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Redirect stubs; wire real routes here when they exist.
const (
	LoginPath       = "/login"
	VerifyEmailPath = "/email/verify"
	DashboardPath   = "/dashboard"
)

// SuspendedErrorKey is the flash message key the reference redirects with
// (__('dashboard.account_suspended')). We have no i18n layer yet, so it rides
// the flash as-is; swap in the translated string when translations land.
const SuspendedErrorKey = "dashboard.account_suspended"

// UserLoader loads a user by ID; *user.Repo satisfies it.
type UserLoader interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

type ctxKey struct{}

// UserFromContext returns the authenticated user stored by RequireAuth.
func UserFromContext(ctx context.Context) (user.User, bool) {
	u, ok := ctx.Value(ctxKey{}).(user.User)
	return u, ok
}

// WithUser stores the authenticated user in the request context; RequireAuth
// calls it on success, tests use it to seed a request.
func WithUser(ctx context.Context, u user.User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

// RequireAuth mirrors Fortify's auth middleware: the session the Session
// middleware loaded from the context → user. Missing/invalid session or user
// redirects to LoginPath (stub); otherwise the user is stored in the request
// context.
func RequireAuth(users UserLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sess, ok := SessionFromContext(r.Context())
			if !ok || sess.UserID == nil {
				Redirect(w, r, LoginPath)
				return
			}
			u, err := users.GetByID(r.Context(), *sess.UserID)
			if errors.Is(err, pgx.ErrNoRows) {
				Redirect(w, r, LoginPath)
				return
			}
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
		})
	}
}

// RequireVerified mirrors EnsureEmailIsVerified (Fortify emailVerification
// feature): guests go to LoginPath, users with nil email_verified_at to
// VerifyEmailPath (stub), everyone else passes.
func RequireVerified(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := UserFromContext(r.Context())
		if !ok {
			Redirect(w, r, LoginPath)
			return
		}
		if u.EmailVerifiedAt == nil {
			Redirect(w, r, VerifyEmailPath)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CheckUserApproved mirrors reference CheckUserApproved: an authenticated
// user whose state is not active gets 403; guests pass through.
func CheckUserApproved(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFromContext(r.Context()); ok && u.State != user.StateActive {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CheckUserSuspended mirrors reference CheckUserSuspended: a suspended user
// is redirected to DashboardPath with the suspended message flash-set
// (reference: redirect()->route('dashboard')->with('error', ...)); everyone
// else passes.
func CheckUserSuspended(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFromContext(r.Context()); ok && u.State == user.StateSuspended {
			RedirectWithFlash(w, r, DashboardPath, "error", SuspendedErrorKey)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// CompleteProfilePath is the profile-completion form route (reference route
// name profile.complete). Stub until views land.
const CompleteProfilePath = "/profile/complete"

// ProfileCompleteness reports whether a user's profile is complete;
// *profile.Repo satisfies it.
type ProfileCompleteness interface {
	Complete(ctx context.Context, userID int64) (bool, error)
}

// CompleteProfileCheck mirrors App\Http\Middleware\CompleteProfileCheck:
// users whose profile lacks gender or blood_group are redirected to the
// profile-completion form; complete profiles pass. Guests pass through —
// auth middleware owns that redirect.
func CompleteProfileCheck(check ProfileCompleteness) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			complete, err := check.Complete(r.Context(), u.ID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !complete {
				Redirect(w, r, CompleteProfilePath)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
