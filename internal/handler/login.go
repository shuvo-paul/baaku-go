// Package handler holds the HTTP handlers: parse, validate, render, delegate.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/login"
	"github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/views"
)

// AuthFailedMessage is lang/en/auth.php 'failed' — identical for unknown
// emails and wrong passwords so login cannot enumerate accounts.
const AuthFailedMessage = "These credentials do not match our records."

// ChallengePath is Fortify's two-factor challenge route.
const ChallengePath = "/user/two-factor-challenge"

// LoginService is the login/logout port; *login.Service satisfies it.
type LoginService interface {
	Login(ctx context.Context, email, password string) (login.LoginResult, error)
	Logout(ctx context.Context, sessionID string) error
}

// PendingSessions opens the pending-2FA session; *twofactorchallenge.Service
// satisfies it (Begin keeps the "login.two_factor" payload shape in-package).
type PendingSessions interface {
	Begin(ctx context.Context, userID int64) (string, error)
}

// Login renders and processes the login form and logout (Fortify
// AuthenticatedSessionController).
type Login struct {
	auth    LoginService
	pending PendingSessions
	sessCfg config.Session
	appName string
}

func NewLogin(auth LoginService, pending PendingSessions, sessCfg config.Session, appName string) *Login {
	return &Login{auth: auth, pending: pending, sessCfg: sessCfg, appName: appName}
}

// ShowLogin renders GET /login; authenticated visitors bounce to the
// dashboard (Fortify RedirectIfAuthenticated).
func (h *Login) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if sess, ok := middleware.SessionFromContext(r.Context()); ok && sess.UserID != nil {
		middleware.Redirect(w, r, middleware.DashboardPath)
		return
	}
	views.LoginPage(h.appName, middleware.TokenFromContext(r.Context()), "", "").Render(r.Context(), w)
}

// Login handles POST /login.
func (h *Login) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	switch {
	case email == "":
		h.renderForm(w, r, email, "The email field is required.")
		return
	case password == "":
		h.renderForm(w, r, email, "The password field is required.")
		return
	}

	res, err := h.auth.Login(r.Context(), email, password)
	switch {
	case errors.Is(err, login.ErrInvalidCredentials):
		// Re-render keeps the entered email (Laravel old('email')).
		h.renderForm(w, r, email, AuthFailedMessage)
		return
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if res.TwoFactorPending {
		// Pending session: user_id NULL + "login.two_factor" payload. The
		// cookie carries the pending session ID so H6's challenge can find
		// the row; RequireAuth still treats the visitor as a guest.
		sid, err := h.pending.Begin(r.Context(), res.UserID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, session.Cookie(h.sessCfg, sid))
		middleware.Redirect(w, r, ChallengePath)
		return
	}

	http.SetCookie(w, session.Cookie(h.sessCfg, res.SessionID))
	middleware.Redirect(w, r, h.postLoginDest(r))
}

// postLoginDest is the intended URL RequireAuth stashed, else the dashboard.
// Only same-site paths count — never redirect off-host.
func (h *Login) postLoginDest(r *http.Request) string {
	dest, ok := middleware.IntendedFromContext(r.Context())
	if ok && strings.HasPrefix(dest, "/") && !strings.HasPrefix(dest, "//") {
		return dest
	}
	return middleware.DashboardPath
}

func (h *Login) renderForm(w http.ResponseWriter, r *http.Request, email, errMsg string) {
	views.LoginPage(h.appName, middleware.TokenFromContext(r.Context()), email, errMsg).Render(r.Context(), w)
}

// Logout handles POST /logout: destroy the session if any, always clear the
// cookie, back to login (Fortify logout has no auth gate).
func (h *Login) Logout(w http.ResponseWriter, r *http.Request) {
	if sess, ok := middleware.SessionFromContext(r.Context()); ok && sess.ID != "" {
		if err := h.auth.Logout(r.Context(), sess.ID); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	c := session.Cookie(h.sessCfg, "")
	c.Value = ""
	c.MaxAge = -1
	c.Expires = time.Unix(0, 0)
	http.SetCookie(w, c)
	middleware.Redirect(w, r, middleware.LoginPath)
}
