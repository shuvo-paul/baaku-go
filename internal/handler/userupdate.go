package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/passwordchange"
	"github.com/shuvo-paul/baaku/internal/service/profileinfo"
)

// Fortify status flash constants (Fortify::PASSWORD_UPDATED etc.).
const (
	PasswordUpdatedFlash           = "password-updated"
	ProfileInformationUpdatedFlash = "profile-information-updated"
)

// UpdatePassword handles PUT /user/password (Fortify PasswordController).
// The form lives in the profile page (dashboard wave); until then this is
// the endpoint only.
type UpdatePassword struct {
	change func(ctx context.Context, userID int64, current, new, confirm string) error
}

func NewUpdatePassword(change func(ctx context.Context, userID int64, current, new, confirm string) error) *UpdatePassword {
	return &UpdatePassword{change: change}
}

// ProfileUpdater handles PUT /user/profile-information (Fortify
// ProfileInformationController).
type ProfileUpdater struct {
	update func(ctx context.Context, userID int64, name, email string) error
}

func NewProfileUpdater(update func(ctx context.Context, userID int64, name, email string) error) *ProfileUpdater {
	return &ProfileUpdater{update: update}
}

// Update handles PUT /user/password (auth). Validation failures redirect
// back with an error flash; success flashes password-updated (Fortify
// PasswordUpdatedResponse).
func (h *UpdatePassword) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	err := h.change(r.Context(), u.ID,
		r.FormValue("current_password"),
		r.FormValue("password"),
		r.FormValue("password_confirmation"),
	)
	switch {
	case errors.Is(err, passwordchange.ErrCurrentPasswordInvalid):
		middleware.RedirectWithFlash(w, r, backURL(r), "error", "The current password field does not match your password.")
	case err != nil:
		middleware.RedirectWithFlash(w, r, backURL(r), "error", err.Error())
	default:
		middleware.RedirectWithFlash(w, r, backURL(r), "status", PasswordUpdatedFlash)
	}
}

// Update handles PUT /user/profile-information (auth).
func (h *ProfileUpdater) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	err := h.update(r.Context(), u.ID,
		strings.TrimSpace(r.FormValue("name")),
		strings.TrimSpace(r.FormValue("email")),
	)
	var fieldErrs profileinfo.FieldErrors
	switch {
	case errors.As(err, &fieldErrs):
		middleware.RedirectWithFlash(w, r, backURL(r), "error", fieldErrs.Error())
	case err != nil:
		middleware.RedirectWithFlash(w, r, backURL(r), "error", "internal server error")
	default:
		middleware.RedirectWithFlash(w, r, backURL(r), "status", ProfileInformationUpdatedFlash)
	}
}

// backURL is the Referer when it is a same-site path, else the dashboard —
// never bounce off-host on a forged Referer.
func backURL(r *http.Request) string {
	if ref := r.Referer(); ref != "" {
		if u, err := http.NewRequest(http.MethodGet, ref, nil); err == nil && u.URL.Path != "" {
			return u.URL.RequestURI()
		}
	}
	return middleware.DashboardPath
}
