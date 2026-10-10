-- Roles CRUD (reference RoleController over spatie/laravel-permission's roles,
-- permissions and role_has_permissions tables). A role's permissions are the
-- names joined from permissions via the role_has_permissions pivot.

-- name: ListRolesWithPermissions :many
SELECT r.id, r.name, r.guard_name, r.created_at, r.updated_at,
       COALESCE(array_agg(p.name ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL), '{}') AS permission_names
FROM public.roles r
LEFT JOIN public.role_has_permissions rhp ON rhp.role_id = r.id
LEFT JOIN public.permissions p ON p.id = rhp.permission_id
GROUP BY r.id
ORDER BY r.id;

-- name: GetRoleWithPermissions :one
SELECT r.id, r.name, r.guard_name, r.created_at, r.updated_at,
       COALESCE(array_agg(p.name ORDER BY p.id) FILTER (WHERE p.id IS NOT NULL), '{}') AS permission_names
FROM public.roles r
LEFT JOIN public.role_has_permissions rhp ON rhp.role_id = r.id
LEFT JOIN public.permissions p ON p.id = rhp.permission_id
WHERE r.id = sqlc.arg(id)
GROUP BY r.id;

-- name: AllPermissionNames :many
SELECT p.name
FROM public.permissions p
ORDER BY p.id;

-- name: AllRoleNames :many
SELECT r.name
FROM public.roles r
ORDER BY r.id;
-- name: CreateRole :one
INSERT INTO public.roles (name, guard_name, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(guard_name), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdateRole :exec
UPDATE public.roles
SET name = sqlc.arg(name), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteRole :exec
DELETE FROM public.roles
WHERE id = sqlc.arg(id);

-- name: RoleHasUsers :one
SELECT EXISTS (
    SELECT 1 FROM public.model_has_roles mhr
    WHERE mhr.role_id = sqlc.arg(role_id)
      AND mhr.model_type = 'App\Models\User'
);

-- name: SyncRolePermissions :exec
-- replace the role's permission set (reference Role::syncPermissions): drop
-- what's no longer checked, then attach what's newly checked.
DELETE FROM public.role_has_permissions WHERE role_id = sqlc.arg(role_id);

-- name: GiveRolePermissionByName :exec
INSERT INTO public.role_has_permissions (permission_id, role_id)
SELECT p.id, sqlc.arg(role_id)
FROM public.permissions p
WHERE p.name = sqlc.arg(permission)
ON CONFLICT DO NOTHING;

-- User role assignment (reference UserRoleController@edit/update over spatie's
-- model_has_roles pivot). Role names come back ordered so the edit form's
-- checked set and the activity-log diff are deterministic.

-- name: UserAssignedRoleNames :many
SELECT r.name
FROM public.model_has_roles mhr
JOIN public.roles r ON r.id = mhr.role_id
WHERE mhr.model_id = sqlc.arg(model_id)
  AND mhr.model_type = 'App\Models\User'
ORDER BY r.name;

-- name: SyncUserRoles :exec
-- Replace a user's role set (reference $user->syncRoles): detach everything
-- attached to this model, then re-insert the given roles by name. Any name
-- that matches no roles row is skipped by the join, mirroring syncRoles'
-- tolerant behaviour for stale input.
DELETE FROM public.model_has_roles
WHERE model_id = sqlc.arg(model_id)
  AND model_type = 'App\Models\User';

-- name: GiveUserRoleByName :exec
INSERT INTO public.model_has_roles (role_id, model_type, model_id)
SELECT r.id, 'App\Models\User', sqlc.arg(model_id)
FROM public.roles r
WHERE r.name = sqlc.arg(role)
ON CONFLICT DO NOTHING;

-- name: RoleExistsWithName :one
SELECT EXISTS (SELECT 1 FROM public.roles WHERE name = sqlc.arg(name));
