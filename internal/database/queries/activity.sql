-- Activity log (spatie/laravel-activitylog's activity_log table). The
-- morph columns store the spatie class names: causer/subject of type User
-- are 'App\Models\User', roles are 'App\Models\Role'.

-- name: InsertActivity :exec
INSERT INTO public.activity_log (log_name, description, subject_type, subject_id, causer_type, causer_id, event, properties, created_at, updated_at)
VALUES (sqlc.arg(log_name), sqlc.arg(description), sqlc.arg(subject_type), sqlc.arg(subject_id), sqlc.arg(causer_type), sqlc.arg(causer_id), sqlc.arg(event), sqlc.arg(properties), now()::timestamp(0), now()::timestamp(0));

-- name: ListDashboardActivity :many
-- Admin activity page (reference ActivityLogController@index): the three
-- dashboard domains, newest first, simplePaginate(20). Causer and subject
-- are resolved eagerly like the reference's ->with(['causer','subject']).
SELECT a.id, a.log_name, a.description, a.event, a.properties, a.created_at, a.subject_id,
       cu.name AS causer_name, cu.email AS causer_email,
       su.name AS subject_user_name, su.email AS subject_user_email,
       sr.name AS subject_role_name
FROM public.activity_log a
LEFT JOIN public.users cu ON cu.id = a.causer_id AND a.causer_type = 'App\Models\User'
LEFT JOIN public.users su ON su.id = a.subject_id AND a.subject_type = 'App\Models\User'
LEFT JOIN public.roles sr ON sr.id = a.subject_id AND a.subject_type = 'App\Models\Role'
WHERE a.log_name IN ('member_management', 'role_management', 'profile')
ORDER BY a.created_at DESC, a.id DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);
