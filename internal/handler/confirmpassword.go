package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/passwordconfirm"
	"github.com/shuvo-paul/baaku/internal/views"
)

// PasswordIncorrectMessage is Fortify's FailedPasswordConfirmationResponse.
const PasswordIncorrectMessage = "The provided password was incorrect."

// PasswordConfirmer is the confirm-password port; *passwordconfirm.Service.
type PasswordConfirmer interface {
	Confirm(ctx context.Context, userID int64, raw string) error
	Check(ctx context.Context, userID int64) bool
}

// ConfirmPassword renders and processes the password-confirmation prompt
// (Fortify ConfirmablePasswordController).
type ConfirmPassword struct {
	svc     PasswordConfirmer
	appName string
}

func NewConfirmPassword(svc PasswordConfirmer, appName string) *ConfirmPassword {
	return &ConfirmPassword{svc: svc, appName: appName}
}

// Show renders GET /user/confirm-password (auth).
func (h *ConfirmPassword) Show(w http.ResponseWriter, r *http.Request) {
	views.ConfirmPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), "").Render(r.Context(), w)
}

// Confirm handles POST /user/confirm-password (auth). Success redirects to
// the intended URL; failure re-renders with Fortify's message.
func (h *ConfirmPassword) Confirm(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	password := r.FormValue("password")
	if password == "" {
		h.render(w, r, "The password field is required.")
		return
	}
	switch err := h.svc.Confirm(r.Context(), u.ID, password); {
	case errors.Is(err, passwordconfirm.ErrInvalidPassword):
		h.render(w, r, PasswordIncorrectMessage)
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	default:
		middleware.Redirect(w, r, intendedDest(r, middleware.DashboardPath))
	}
}

// Status handles GET /user/confirmed-password-status (auth): 200 when the
// password was confirmed within the TTL, else 423 Locked (Fortify).
func (h *ConfirmPassword) Status(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if h.svc.Check(r.Context(), u.ID) {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusLocked)
}

func (h *ConfirmPassword) render(w http.ResponseWriter, r *http.Request, errMsg string) {
	views.ConfirmPasswordPage(h.appName, middleware.TokenFromContext(r.Context()), errMsg).Render(r.Context(), w)
}
