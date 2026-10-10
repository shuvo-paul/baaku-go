// Package role is a thin wrapper over the sqlc-generated roles queries
// (reference RoleController store/update/destroy over spatie's Role model).
package role

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/role"
)

// Repo is a thin wrapper over the sqlc-generated roles queries. It returns the
// service-layer domain type, matching repository/career → service/career.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// guardName is spatie's guard for the web guard (all seeded roles/permissions
// use it).
const guardName = "web"

// userType is the spatie morph class stored in model_has_roles.model_type.
const userType = "App\\Models\\User"

// List returns every role with its granted permission names.
func (r *Repo) List(ctx context.Context) ([]role.Role, error) {
	rows, err := r.q.ListRolesWithPermissions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]role.Role, 0, len(rows))
	for _, row := range rows {
		out = append(out, role.Role{
			ID:          row.ID,
			Name:        row.Name,
			Permissions: permNames(row.PermissionNames),
		})
	}
	return out, nil
}

// Get fetches one role with its permission names (findOrFail → pgx.ErrNoRows).
func (r *Repo) Get(ctx context.Context, id int64) (role.Role, error) {
	row, err := r.q.GetRoleWithPermissions(ctx, id)
	if err != nil {
		return role.Role{}, err
	}
	return role.Role{
		ID:          row.ID,
		Name:        row.Name,
		Permissions: permNames(row.PermissionNames),
	}, nil
}

// AllPermissionNames returns every permission name for the create/edit forms.
func (r *Repo) AllPermissionNames(ctx context.Context) ([]string, error) {
	return r.q.AllPermissionNames(ctx)
}

// AllRoleNames returns every role name, in seeded order (the edit form's
// checkbox list; *repository/role also backs userrole's validation).
func (r *Repo) AllRoleNames(ctx context.Context) ([]string, error) {
	return r.q.AllRoleNames(ctx)
}

// NameExists reports whether a role with this name already exists; when
// ignoreID is non-zero it is excluded (Rule::unique->ignore on update).
func (r *Repo) NameExists(ctx context.Context, name string, ignoreID int64) (bool, error) {
	if ignoreID != 0 {
		existing, err := r.q.GetRoleWithPermissions(ctx, ignoreID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		if existing.Name == name {
			return false, nil // name unchanged → still unique for this row
		}
	}
	return r.q.RoleExistsWithName(ctx, name)
}

// Create inserts a role and attaches its permissions.
func (r *Repo) Create(ctx context.Context, name string, permissions []string) (int64, error) {
	id, err := r.q.CreateRole(ctx, generated.CreateRoleParams{
		Name:      name,
		GuardName: guardName,
	})
	if err != nil {
		return 0, err
	}
	if err := r.syncPermissions(ctx, id, permissions); err != nil {
		return 0, err
	}
	return id, nil
}

// Update renames the role and re-syncs its permission set.
func (r *Repo) Update(ctx context.Context, id int64, name string, permissions []string) error {
	if err := r.q.UpdateRole(ctx, generated.UpdateRoleParams{ID: id, Name: name}); err != nil {
		return err
	}
	return r.syncPermissions(ctx, id, permissions)
}

// Delete removes the role by id.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteRole(ctx, id)
}

// HasUsers reports whether the role is still assigned to at least one user.
func (r *Repo) HasUsers(ctx context.Context, id int64) (bool, error) {
	return r.q.RoleHasUsers(ctx, id)
}

// syncPermissions drops the role's current grants, then attaches each given
// permission name (reference Role::syncPermissions).
func (r *Repo) syncPermissions(ctx context.Context, roleID int64, permissions []string) error {
	if err := r.q.SyncRolePermissions(ctx, roleID); err != nil {
		return err
	}
	for _, p := range permissions {
		if err := r.q.GiveRolePermissionByName(ctx, generated.GiveRolePermissionByNameParams{
			RoleID:     roleID,
			Permission: p,
		}); err != nil {
			return err
		}
	}
	return nil
}

// UserRolesFor returns the role names currently assigned to a user (reference
// UserRoleController@edit: $user->roles->pluck('name')). Used to pre-check the
// edit form and to diff the assigned set on update.
func (r *Repo) UserRolesFor(ctx context.Context, userID int64) ([]string, error) {
	return r.q.UserAssignedRoleNames(ctx, userID)
}

// SyncUserRoles replaces a user's whole role set (reference $user->syncRoles):
// detach every role from this model, then re-insert the given role names.
// Unknown names are skipped by the insert's join, matching syncRoles' tolerance.
func (r *Repo) SyncUserRoles(ctx context.Context, userID int64, roleNames []string) error {
	if err := r.q.SyncUserRoles(ctx, userID); err != nil {
		return err
	}
	for _, name := range roleNames {
		if err := r.q.GiveUserRoleByName(ctx, generated.GiveUserRoleByNameParams{
			ModelID: userID,
			Role:    name,
		}); err != nil {
			return err
		}
	}
	return nil
}

// permNames coerces the sqlc array_agg payload (pgx returns interface{} for
// the text[] column) into []string.
func permNames(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
