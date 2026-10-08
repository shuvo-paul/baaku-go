package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
	"github.com/shuvo-paul/baaku/internal/views"
)

// Fortify FailedTwoFactorLoginResponse messages.
const (
	TwoFactorCodeInvalid     = "The provided two factor authentication code was invalid."
	TwoFactorRecoveryInvalid = "The provided two factor recovery code was invalid."
)

// Challenge verifies the pending-2FA login (twofactorchallenge.Service).
type Challenge interface {
	Challenge(ctx context.Context, sessionID, code string) (twofactorchallenge.ChallengeResult, error)
}

// TwoFactorChallenge renders and processes the login 2FA challenge
// (Fortify TwoFactorAuthenticatedSessionController). key is the decoded
// APP_KEY bytes the recaller cookie's password-hash MAC is keyed with.
type TwoFactorChallenge struct {
	svc     Challenge
	sessCfg config.Session
	appName string
	key     []byte
}

func NewTwoFactorChallenge(svc Challenge, sessCfg config.Session, appName string, key []byte) *TwoFactorChallenge {
	return &TwoFactorChallenge{svc: svc, sessCfg: sessCfg, appName: appName, key: key}
}

// Show renders GET /two-factor-challenge. No pending challenge → login
// (Fortify: !hasChallengedUser → redirect route('login')).
func (h *TwoFactorChallenge) Show(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok || sess.UserID != nil || !isPending(sess.Payload) {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.TwoFactorChallengePage(h.appName, middleware.TokenFromContext(r.Context()), "").Render(r.Context(), w)
}

// Submit handles POST /two-factor-challenge with either a TOTP code or a
// recovery code. On success the pending session row is promoted in place
// (same ID), so the existing cookie becomes authenticated — just refresh it
// and continue to the intended URL.
func (h *TwoFactorChallenge) Submit(w http.ResponseWriter, r *http.Request) {
	sess, ok := middleware.SessionFromContext(r.Context())
	if !ok || sess.UserID != nil || !isPending(sess.Payload) {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	recovery := strings.TrimSpace(r.FormValue("recovery_code"))
	switch {
	case code == "" && recovery == "":
		h.render(w, r, TwoFactorCodeInvalid)
		return
	case recovery != "":
		code = recovery
	}

	res, err := h.svc.Challenge(r.Context(), sess.ID, code)
	switch {
	case errors.Is(err, twofactorchallenge.ErrNoPendingChallenge):
		middleware.Redirect(w, r, middleware.LoginPath)
	case errors.Is(err, twofactor.ErrInvalidCode):
		if recovery != "" {
			h.render(w, r, TwoFactorRecoveryInvalid)
		} else {
			h.render(w, r, TwoFactorCodeInvalid)
		}
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	default:
		http.SetCookie(w, session.Cookie(h.sessCfg, sess.ID))
		if res.RememberToken != "" {
			http.SetCookie(w, session.RememberCookie(h.sessCfg, h.key, res.UserID, res.RememberToken, res.PasswordHash))
		}
		middleware.Redirect(w, r, intendedDest(r, middleware.DashboardPath))
	}
}

func (h *TwoFactorChallenge) render(w http.ResponseWriter, r *http.Request, errMsg string) {
	views.TwoFactorChallengePage(h.appName, middleware.TokenFromContext(r.Context()), errMsg).Render(r.Context(), w)
}

// isPending reports whether the session payload carries the pending-login
// flag (login.two_factor) — the twofactorchallenge service's own marker.
func isPending(payload string) bool {
	return strings.Contains(payload, twofactorchallenge.PendingKey)
}
