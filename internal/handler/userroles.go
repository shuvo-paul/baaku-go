// Package handler — UserRoles serves the assign-roles form for a member
// (reference UserRoleController@edit/update), behind the "manage members"
// permission enforced at the route layer.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/service/userrole"
	"github.com/shuvo-paul/baaku/internal/views"
)

// UserRolesService is the assign-roles surface; *userrole.Service satisfies it.
type UserRolesService interface {
	Form(ctx context.Context, targetID int64) (userrole.Target, []string, error)
	Update(ctx context.Context, actorID, targetID int64, requested []string) error
}

// UserRoles renders the assign-roles form and handles the sync post.
type UserRoles struct {
	svc     UserRolesService
	appName string
}

func NewUserRoles(svc UserRolesService, appName string) *UserRoles {
	return &UserRoles{svc: svc, appName: appName}
}

// Edit renders the assign-roles form (reference UserRoleController@edit).
func (h *UserRoles) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	target, roles, err := h.svc.Form(r.Context(), id)
	if errors.Is(err, userrole.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.UserRolesPage(h.formData(r, u, target, roles, target.AssignedRoles, "")).Render(r.Context(), w)
}

// Update handles PUT /dashboard/users/{id}/roles (reference
// UserRoleController@update). Guard errors re-render the form with the
// reference message; a missing target 404s.
func (h *UserRoles) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	requested := r.Form["roles[]"]
	err := h.svc.Update(r.Context(), u.ID, id, requested)
	if err == nil {
		middleware.RedirectWithFlash(w, r, "/dashboard/users/"+strconv.FormatInt(id, 10)+"/roles", "status", "user-roles-updated")
		return
	}
	msg, ok := h.guardMessage(err)
	if !ok {
		if errors.Is(err, userrole.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Reference re-renders users/roles.blade.php with the guard message.
	target, roles, ferr := h.svc.Form(r.Context(), id)
	if ferr != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.UserRolesPage(h.formData(r, u, target, roles, requested, msg)).Render(r.Context(), w)
}

// guardMessage maps the reference's guard errors to their exact flash messages
// (users.roles.edit redirects with ->with('error', ...)).
func (h *UserRoles) guardMessage(err error) (string, bool) {
	switch {
	case errors.Is(err, userrole.ErrUnverified):
		return "Email must be verified before membership actions can be taken.", true
	case errors.Is(err, userrole.ErrCannotRemoveOwnAdmin):
		return "You cannot remove your own admin role.", true
	}
	var fieldErrs userrole.FieldErrors
	if errors.As(err, &fieldErrs) {
		return fieldErrs.Error(), true
	}
	return "", false
}

// formData builds the assign-roles form view model.
func (h *UserRoles) formData(r *http.Request, u user.User, target userrole.Target, roles, selected []string, errMsg string) views.UserRolesFormData {
	return views.UserRolesFormData{
		Sidebar:  membersSidebar(h.appName, r, u),
		User:     views.UserRoleView{ID: target.ID, Name: target.Name, Email: target.Email},
		Roles:    roles,
		Selected: selected,
		ErrMsg:   errMsg,
		CSRF:     middleware.TokenFromContext(r.Context()),
	}
}
