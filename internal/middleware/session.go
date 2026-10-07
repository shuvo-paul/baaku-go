// Session middleware: loads the cookie-named session row into the request
// context so downstream guards (RequireAuth, flash helpers) share one DB read.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// SessionLoader resolves a session row by ID; *session.Store satisfies it.
type SessionLoader interface {
	Load(ctx context.Context, id string) (generated.Session, error)
}

// SessionSaver persists a session row; *session.Store satisfies it.
type SessionSaver interface {
	Save(ctx context.Context, p generated.UpsertSessionParams) error
}

// SessionStore is what the session middleware needs: load by cookie ID,
// persist payload writes (flash aging, SetFlash).
type SessionStore interface {
	SessionLoader
	SessionSaver
}

type sessionCtxKey struct{}

type sessionState struct {
	sess  generated.Session
	saver SessionSaver
}

// WithSession stores the loaded session (and its saver) in the request
// context; Session middleware calls it on success, tests seed with it.
func WithSession(ctx context.Context, s generated.Session, saver SessionSaver) context.Context {
	return context.WithValue(ctx, sessionCtxKey{}, sessionState{sess: s, saver: saver})
}

// SessionFromContext returns the session loaded by the Session middleware.
func SessionFromContext(ctx context.Context) (generated.Session, bool) {
	st, ok := ctx.Value(sessionCtxKey{}).(sessionState)
	return st.sess, ok
}

// Session loads the session named by the cookie into the request context.
// Missing/invalid cookies and load failures yield a guest context — no error,
// no redirect; RequireAuth owns that decision.
//
// Flash aging: keys written at redirect time (flash.go) are read once by the
// next request, then stripped from the payload and persisted (Laravel flash
// semantics).
//
// ponytail: load errors (incl. DB down) degrade to guest; RequireAuth
// redirects to login either way. Log here if that ever masks debugging.
func Session(cookieName string, store SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
				if sess, err := store.Load(ctx, c.Value); err == nil {
					if flash := payloadFlash(sess.Payload); len(flash) > 0 {
						ctx = withFlash(ctx, flash)
						sess.Payload = setPayloadFlash(sess.Payload, nil)
						// ponytail: save failure drops the aging; flash would
						// show for one extra request.
						_ = store.Save(ctx, upsertSessionParams(sess))
					}
					ctx = WithSession(ctx, sess, store)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// upsertSessionParams copies a loaded session row for a Save; last_activity
// refreshes on every write (Laravel session behaviour, see Store.Save).
func upsertSessionParams(s generated.Session) generated.UpsertSessionParams {
	return generated.UpsertSessionParams{
		ID:           s.ID,
		UserID:       s.UserID,
		IpAddress:    s.IpAddress,
		UserAgent:    s.UserAgent,
		Payload:      s.Payload,
		LastActivity: int32(time.Now().Unix()),
	}
}
