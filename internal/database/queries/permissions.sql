-- Permission checks mirror spatie/laravel-permission's HasRoles::hasPermissionTo:
-- a user has a permission granted directly (model_has_permissions) or via any
-- assigned role (model_has_roles -> role_has_permissions). model_type is the
-- spatie morph class for the User model.

-- name: UserPermissionNames :many
SELECT DISTINCT p.name
FROM public.permissions p
WHERE EXISTS (
    SELECT 1 FROM public.model_has_permissions mph
    WHERE mph.permission_id = p.id
      AND mph.model_id = sqlc.arg(user_id)
      AND mph.model_type = 'App\Models\User'
)
OR EXISTS (
    SELECT 1
    FROM public.model_has_roles mhr
    JOIN public.role_has_permissions rhp ON rhp.role_id = mhr.role_id
    WHERE rhp.permission_id = p.id
      AND mhr.model_id = sqlc.arg(user_id)
      AND mhr.model_type = 'App\Models\User'
);

-- Seeding (mirrors reference database/seeders/PermissionsSeeder.php and
-- RolesSeeder.php): findOrCreate semantics on the (name, guard_name) unique.

-- name: UpsertPermission :one
INSERT INTO public.permissions (name, guard_name, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(guard_name), now()::timestamp(0), now()::timestamp(0))
ON CONFLICT (name, guard_name) DO UPDATE SET updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: UpsertRole :one
INSERT INTO public.roles (name, guard_name, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(guard_name), now()::timestamp(0), now()::timestamp(0))
ON CONFLICT (name, guard_name) DO UPDATE SET updated_at = EXCLUDED.updated_at
RETURNING id;

-- name: GiveRolePermission :exec
INSERT INTO public.role_has_permissions (permission_id, role_id)
VALUES (sqlc.arg(permission_id), sqlc.arg(role_id))
ON CONFLICT DO NOTHING;

-- name: AssignRole :exec
INSERT INTO public.model_has_roles (role_id, model_type, model_id)
VALUES (sqlc.arg(role_id), sqlc.arg(model_type), sqlc.arg(model_id))
ON CONFLICT DO NOTHING;
