package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/passwordreset"
	"github.com/shuvo-paul/baaku/internal/views"
)

// Lang lines from laravel/framework lang/en/passwords.php (the reference
// app ships no override — reference/lang/en has no passwords.php).
const (
	// PasswordsSent is passwords.sent.
	PasswordsSent = "We have emailed your password reset link."
	// PasswordsReset is passwords.reset.
	PasswordsReset = "Your password has been reset."
	// PasswordsToken is passwords.token.
	PasswordsToken = "This password reset token is invalid."
	// PasswordsUser is passwords.user.
	PasswordsUser = "We can't find a user with that email address."
)

// passwordsThrottled renders passwords.throttled with the remaining wait.
func passwordsThrottled(retryAfter time.Duration) string {
	return fmt.Sprintf("Please wait %d seconds before trying again.", int(retryAfter.Seconds())+1)
}

// PasswordResetService is the reset-broker port; *passwordreset.Service.
type PasswordResetService interface {
	Issue(ctx context.Context, email string) (string, error)
	Complete(ctx context.Context, email, rawToken, newPassword string) error
}

// ResetLinkSender delivers one reset email (wired in main from the mailer +
// APP_URL). Takes the raw token, not the URL, so the handler stays
// config-free.
type ResetLinkSender func(email, rawToken string) error

// PasswordReset renders and processes the forgot / reset forms (Fortify
// PasswordResetLinkController + NewPasswordController).
type PasswordReset struct {
	svc      PasswordResetService
	sendLink ResetLinkSender
	appName  string
}

func NewPasswordReset(svc PasswordResetService, sendLink ResetLinkSender, appName string) *PasswordReset {
	return &PasswordReset{svc: svc, sendLink: sendLink, appName: appName}
}

// ShowForgot renders GET /forgot-password; status is the session('status')
// flash after a successful request (Laravel redirects back with it).
func (h *PasswordReset) ShowForgot(w http.ResponseWriter, r *http.Request) {
	status := middleware.FlashFromContext(r.Context())["status"]
	views.ForgotPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), status, "").Render(r.Context(), w)
}

// Forgot handles POST /forgot-password: issue a token, email the link, then
// redirect back with the passwords.sent status. Unknown email renders
// passwords.user — Fortify's PasswordResetLinkController returns
// FailedPasswordResetLinkRequestResponse for broker statuses other than
// RESET_LINK_SENT.
func (h *PasswordReset) Forgot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		h.renderForgot(w, r, "", "The email field is required.")
		return
	}

	token, err := h.svc.Issue(r.Context(), email)
	var throttled passwordreset.ThrottledError
	switch {
	case errors.Is(err, passwordreset.ErrUserNotFound):
		h.renderForgot(w, r, "", PasswordsUser)
		return
	case errors.As(err, &throttled):
		// Broker keeps the existing token; show the remaining wait.
		h.renderForgot(w, r, "", passwordsThrottled(throttled.RetryAfter))
		return
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if err := h.sendLink(email, token); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/forgot-password", "status", PasswordsSent)
}

func (h *PasswordReset) renderForgot(w http.ResponseWriter, r *http.Request, status, errMsg string) {
	views.ForgotPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), status, errMsg).Render(r.Context(), w)
}

// ShowReset renders GET /reset-password/{token} (Fortify password.reset).
func (h *PasswordReset) ShowReset(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	email := strings.TrimSpace(r.URL.Query().Get("email"))
	views.ResetPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), token, email, "").Render(r.Context(), w)
}

// Reset handles POST /reset-password (Fortify password.update).
func (h *PasswordReset) Reset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	token := r.FormValue("token")
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirmation")

	switch {
	case token == "":
		h.renderReset(w, r, token, email, PasswordsToken)
		return
	case email == "":
		h.renderReset(w, r, token, email, "The email field is required.")
		return
	case password == "":
		h.renderReset(w, r, token, email, "The password field is required.")
		return
	case password != confirm:
		h.renderReset(w, r, token, email, "The password field confirmation does not match.")
		return
	}

	err := h.svc.Complete(r.Context(), email, token, password)
	switch {
	case errors.Is(err, passwordreset.ErrInvalidToken):
		h.renderReset(w, r, token, email, PasswordsToken)
		return
	case errors.Is(err, passwordreset.ErrUserNotFound):
		h.renderReset(w, r, token, email, PasswordsUser)
		return
	case err != nil:
		// Password rule failures (password.Validate) render inline.
		h.renderReset(w, r, token, email, err.Error())
		return
	}
	middleware.RedirectWithFlash(w, r, middleware.LoginPath, "status", PasswordsReset)
}

func (h *PasswordReset) renderReset(w http.ResponseWriter, r *http.Request, token, email, errMsg string) {
	views.ResetPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), token, email, errMsg).Render(r.Context(), w)
}
