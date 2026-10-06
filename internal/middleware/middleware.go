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

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Redirect stubs; wire real routes here when they exist.
const (
	LoginPath       = "/login"
	VerifyEmailPath = "/email/verify"
	DashboardPath   = "/dashboard"
)

// SuspendedErrorKey is the flash message key the reference redirects with
// (__('dashboard.account_suspended')); passed as the error query stub until
// session flash exists.
const SuspendedErrorKey = "dashboard.account_suspended"

// UserLoader loads a user by ID; *user.Repo satisfies it.
type UserLoader interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

// SessionLoader resolves a session row by ID; *session.Store satisfies it.
type SessionLoader interface {
	Load(ctx context.Context, id string) (generated.Session, error)
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

// RequireAuth mirrors Fortify's auth middleware: session cookie → session →
// user. Missing/invalid session or user redirects to LoginPath (stub);
// otherwise the user is stored in the request context.
func RequireAuth(cookieName string, sessions SessionLoader, users UserLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookieName)
			if err != nil || c.Value == "" {
				redirect(w, r, LoginPath)
				return
			}
			sess, err := sessions.Load(r.Context(), c.Value)
			if err != nil {
				if errors.Is(err, session.ErrNotFound) {
					redirect(w, r, LoginPath)
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if sess.UserID == nil {
				redirect(w, r, LoginPath)
				return
			}
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			u, err := users.GetByID(r.Context(), *sess.UserID)
			if errors.Is(err, pgx.ErrNoRows) {
				redirect(w, r, LoginPath)
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
			redirect(w, r, LoginPath)
			return
		}
		if u.EmailVerifiedAt == nil {
			redirect(w, r, VerifyEmailPath)
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
// is redirected to DashboardPath carrying the suspended flash key; everyone
// else passes.
//
// ponytail: flash rides a query param until session flash exists; move it to
// a session flash write when the flash layer lands.
func CheckUserSuspended(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, ok := UserFromContext(r.Context()); ok && u.State == user.StateSuspended {
			redirect(w, r, DashboardPath+"?error="+SuspendedErrorKey)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func redirect(w http.ResponseWriter, r *http.Request, to string) {
	http.Redirect(w, r, to, http.StatusFound)
}
