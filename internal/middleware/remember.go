// Remember-me restore: when the session cookie is missing or expired but a
// valid recaller cookie is present, re-open an authenticated session from it
// (Laravel SessionGuard::user pulling the user via the recaller).
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/subtle"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/session"
)

// RememberSessions creates the restored session; *session.Store satisfies it
// (CreateSession plus the Load/Save port WithSession stores for flash aging).
type RememberSessions interface {
	SessionStore
	CreateSession(ctx context.Context, userID int64) (string, error)
}

// Remember restores an authenticated session from the recaller cookie when
// the session middleware left the request unauthenticated. Mount it directly
// after Session. A restored login gets a fresh session ID and cookie — the
// recaller token itself is not rotated (Laravel doesn't either).
//
// ponytail: restored logins don't refresh the recaller cookie, so it expires
// RememberDurationMinutes(400 days) after the original login no matter how
// active the user is; re-queue the cookie per request if that ever bites.
func Remember(cfg config.Session, key []byte, users UserLoader, sessions RememberSessions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sess, ok := SessionFromContext(r.Context()); ok && sess.UserID != nil {
				next.ServeHTTP(w, r) // authenticated by session already
				return
			}
			c, err := r.Cookie(session.RecallerName())
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			uid, token, recallerHash, ok := session.ParseRecaller(c.Value)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			u, err := users.GetByID(r.Context(), uid)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			// Laravel userFromRecaller: the token must match the stored
			// remember token, and the cookie's hash must match the current
			// password hash — so logout (token cleared) and password change
			// (token cleared + hash changed) both kill old recaller cookies.
			if u.RememberToken == nil ||
				subtle.ConstantTimeCompare([]byte(*u.RememberToken), []byte(token)) != 1 ||
				!hmac.Equal([]byte(session.HashPasswordForCookie(key, u.PasswordHash)), []byte(recallerHash)) {
				next.ServeHTTP(w, r)
				return
			}
			sid, err := sessions.CreateSession(r.Context(), u.ID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, session.Cookie(cfg, sid))
			ctx := WithSession(r.Context(), generated.Session{
				ID:           sid,
				UserID:       &u.ID,
				Payload:      "{}",
				LastActivity: int32(time.Now().Unix()),
			}, sessions)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
