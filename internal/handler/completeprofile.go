package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/completeprofile"
	"github.com/shuvo-paul/baaku/internal/views"
)

// ProfileCompleteness reports whether a profile is complete;
// *repository/profile.Repo satisfies it (shared with the gate middleware).
type ProfileCompleteness interface {
	Complete(ctx context.Context, userID int64) (bool, error)
}

// CompleteProfileSvc completes a profile; *completeprofile.Service satisfies it.
type CompleteProfileSvc interface {
	Complete(ctx context.Context, userID int64, in completeprofile.Input) error
}

// CompleteProfile renders and processes GET/POST /profile/complete (reference
// CompleteProfileController). Guards (auth + verified + user.suspended) are
// applied on the route.
type CompleteProfile struct {
	svc      CompleteProfileSvc
	profiles ProfileCompleteness
	appName  string
}

func NewCompleteProfile(svc CompleteProfileSvc, profiles ProfileCompleteness, appName string) *CompleteProfile {
	return &CompleteProfile{svc: svc, profiles: profiles, appName: appName}
}

// Show renders the completion form (reference CompleteProfileController@create):
// complete profiles bounce to the dashboard.
func (h *CompleteProfile) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	complete, err := h.profiles.Complete(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if complete {
		middleware.Redirect(w, r, middleware.DashboardPath)
		return
	}
	views.CompleteProfilePage(h.appName, middleware.TokenFromContext(r.Context()), "").Render(r.Context(), w)
}

// Store handles POST /profile/complete (reference CompleteProfileController@store):
// validate + persist the gate fields, then redirect to the dashboard with the
// profile_completed status flash. The reference skips the flash for admins;
// roles aren't ported yet, so everyone gets it.
func (h *CompleteProfile) Store(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	complete, err := h.profiles.Complete(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if complete {
		middleware.Redirect(w, r, middleware.DashboardPath)
		return
	}

	in := completeprofile.Input{
		Gender:           strings.TrimSpace(r.FormValue("gender")),
		BloodGroup:       strings.TrimSpace(r.FormValue("blood_group")),
		PresentAddress:   strings.TrimSpace(r.FormValue("present_address")),
		PermanentAddress: strings.TrimSpace(r.FormValue("permanent_address")),
	}
	err = h.svc.Complete(r.Context(), u.ID, in)
	var fieldErrs completeprofile.FieldErrors
	switch {
	case errors.As(err, &fieldErrs):
		views.CompleteProfilePage(h.appName, middleware.TokenFromContext(r.Context()), fieldErrs.Error()).Render(r.Context(), w)
		return
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, middleware.DashboardPath, "status", "Your profile has been updated.")
}
