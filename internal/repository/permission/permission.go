// Package permission is a thin wrapper over the sqlc-generated role/permission
// queries, mirroring spatie/laravel-permission's tables (roles, permissions,
// model_has_roles, model_has_permissions, role_has_permissions).
package permission

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Repo is a thin wrapper over the sqlc-generated permission queries.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// UserPermissionNames returns every permission name the user holds, directly
// or via an assigned role (spatie HasRoles::getAllPermissions).
func (r *Repo) UserPermissionNames(ctx context.Context, userID int64) ([]string, error) {
	return r.q.UserPermissionNames(ctx, userID)
}

// CreateGrant seeds one permission row (findOrCreate semantics).
func (r *Repo) CreateGrant(ctx context.Context, name, guard string) (int64, error) {
	return r.q.UpsertPermission(ctx, generated.UpsertPermissionParams{
		Name:      name,
		GuardName: guard,
	})
}

// CreateRole seeds one role row (findOrCreate semantics).
func (r *Repo) CreateRole(ctx context.Context, name, guard string) (int64, error) {
	return r.q.UpsertRole(ctx, generated.UpsertRoleParams{
		Name:      name,
		GuardName: guard,
	})
}

// GiveRolePermission attaches a permission to a role; no-op when already
// attached (seeder idempotence).
func (r *Repo) GiveRolePermission(ctx context.Context, permissionID, roleID int64) error {
	return r.q.GiveRolePermission(ctx, generated.GiveRolePermissionParams{
		PermissionID: permissionID,
		RoleID:       roleID,
	})
}

// userType is the spatie morph class stored in model_has_roles.model_type.
const userType = "App\\Models\\User"

// AssignRole attaches a role to a user (spatie assignRole); no-op when
// already assigned.
func (r *Repo) AssignRole(ctx context.Context, roleID, userID int64) error {
	return r.q.AssignRole(ctx, generated.AssignRoleParams{
		RoleID:    roleID,
		ModelType: userType,
		ModelID:   userID,
	})
}
