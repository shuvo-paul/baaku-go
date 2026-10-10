// Package handler — RoleAdmin serves the dashboard's roles CRUD (reference
// RoleController over spatie/laravel-permission), behind the "manage roles"
// permission enforced at the route layer.
package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/role"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// RoleService is the roles CRUD surface; *role.Service satisfies it.
type RoleService interface {
	List(ctx context.Context) ([]role.Role, error)
	Get(ctx context.Context, id int64) (role.Role, error)
	AllPermissionNames(ctx context.Context) ([]string, error)
	Create(ctx context.Context, name string, permissions []string) (int64, error)
	Update(ctx context.Context, id int64, name string, permissions []string) error
	Delete(ctx context.Context, id int64) error
}

// RoleAdmin renders the roles index + create/edit forms and handles the
// store/update/destroy posts (reference RoleController).
type RoleAdmin struct {
	svc     RoleService
	appName string
}

func NewRoleAdmin(svc RoleService, appName string) *RoleAdmin {
	return &RoleAdmin{svc: svc, appName: appName}
}

// Index renders the roles table (reference RoleController@index).
func (h *RoleAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	roles, err := h.svc.List(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.RoleRow, 0, len(roles))
	for _, rl := range roles {
		rows = append(rows, views.RoleRow{ID: rl.ID, Name: rl.Name, Permissions: rl.Permissions})
	}
	flash := middleware.FlashFromContext(r.Context())
	views.RolesPage(views.RolesPageData{
		Sidebar: roleSidebar(h.appName, r, u),
		Rows:    rows,
		Flash:   flash["status"],
		Err:     flash["error"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the create-role form (reference RoleController@create).
func (h *RoleAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	perms, err := h.svc.AllPermissionNames(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.RoleFormPage(h.formData(r, u, 0, "", nil, perms, "")).Render(r.Context(), w)
}

// Store handles POST /dashboard/roles (reference RoleController@store).
func (h *RoleAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	perms := r.Form["permissions[]"]
	if _, err := h.svc.Create(r.Context(), name, perms); err != nil {
		h.renderFormError(w, r, u, 0, name, perms, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/roles", "status", "role-created")
}

// Edit renders the edit-role form (reference RoleController@edit).
func (h *RoleAdmin) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	cur, err := h.svc.Get(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	perms, err := h.svc.AllPermissionNames(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.RoleFormPage(h.formData(r, u, id, cur.Name, cur.Permissions, perms, "")).Render(r.Context(), w)
}

// Update handles PUT /dashboard/roles/{id} (reference RoleController@update).
func (h *RoleAdmin) Update(w http.ResponseWriter, r *http.Request) {
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
	name := r.FormValue("name")
	perms := r.Form["permissions[]"]
	if err := h.svc.Update(r.Context(), id, name, perms); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.renderFormError(w, r, u, id, name, perms, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/roles", "status", "role-updated")
}

// Destroy handles DELETE /dashboard/roles/{id} (reference
// RoleController@destroy): a role still assigned to users redirects back with
// the role_has_users error flash instead of deleting.
func (h *RoleAdmin) Destroy(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, role.ErrInUse) {
			middleware.RedirectWithFlash(w, r, "/dashboard/roles", "error", "role-has-users")
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/roles", "status", "role-deleted")
}

// renderFormError re-renders the role form with the validator message
// (reference: $request->errors() redisplay). Non-field errors 500.
func (h *RoleAdmin) renderFormError(w http.ResponseWriter, r *http.Request, u user.User, id int64, name string, perms []string, err error) {
	var fieldErrs role.FieldErrors
	if !errors.As(err, &fieldErrs) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	all, svcErr := h.svc.AllPermissionNames(r.Context())
	if svcErr != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.RoleFormPage(h.formData(r, u, id, name, perms, all, fieldErrs.Error())).Render(r.Context(), w)
}

// formData builds the role create/edit form view model.
func (h *RoleAdmin) formData(r *http.Request, u user.User, id int64, name string, selected, all []string, errMsg string) views.RoleFormData {
	title := "Create Role"
	if id != 0 {
		title = "Edit Role"
	}
	return views.RoleFormData{
		Sidebar:      roleSidebar(h.appName, r, u),
		Title:        title,
		ID:           id,
		Name:         name,
		Permissions:  all,
		Selected:     selected,
		FlashMessage: errMsg,
		CSRF:         middleware.TokenFromContext(r.Context()),
	}
}

// roleSidebar builds the dashboard chrome for the roles pages; permissions
// come from middleware.LoadPermissions and gate nav items the same way the
// reference's dashboard layout calls can().
func roleSidebar(appName string, r *http.Request, u user.User) views.SidebarData {
	return views.NewSidebarData(appName, middleware.TokenFromContext(r.Context()), u.Name, u.Email, "/dashboard/roles", string(u.State), middleware.PermissionsFromContext(r.Context()))
}
