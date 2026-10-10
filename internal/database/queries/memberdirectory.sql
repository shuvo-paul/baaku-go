-- Member directory (reference UserRoleController@index/show): paginated user
-- cards with roles, photo and latest education/current career, plus the
-- by-state counts and the state-transition target fetch.

-- name: ListDirectoryUsers :many
-- Admin sees every state, staff/directory view sees only active (the
-- non-admin branch hard-filters state='active' in the reference; here the
-- caller passes active_only). Search matches name or email with Laravel's
-- LIKE-escape convention. Pending is ordered oldest-first, everyone else by
-- name (reference index()).
SELECT u.id, u.name, u.email, u.state, u.phone, u.email_verified_at, u.created_at,
       p.photo_path,
       COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), '{}') AS role_names,
       COALESCE(ed.level, '') AS education_level,
       COALESCE(ed.institution, '') AS education_institution,
       COALESCE(cr.job_title, '') AS career_job_title,
       COALESCE(cr.company, '') AS career_company
FROM public.users u
LEFT JOIN public.profiles p ON p.user_id = u.id
LEFT JOIN model_has_roles mhr ON mhr.model_type = 'App\Models\User' AND mhr.model_id = u.id
LEFT JOIN roles r ON r.id = mhr.role_id
LEFT JOIN LATERAL (
    SELECT e.level, e.institution FROM public.educations e
    WHERE e.profile_id = p.id
    ORDER BY e.start_year DESC, e.id DESC LIMIT 1
) ed ON true
LEFT JOIN LATERAL (
    SELECT c.job_title, c.company FROM public.careers c
    WHERE c.profile_id = p.id
    ORDER BY c.is_current DESC, c.id ASC LIMIT 1
) cr ON true
WHERE (NOT COALESCE(sqlc.narg('active_only'), false) OR u.state = 'active')
  AND (sqlc.narg('state')::text IS NULL OR u.state = sqlc.narg('state'))
  AND (sqlc.narg('search')::text IS NULL
       OR u.name LIKE '%' || sqlc.narg('search') || '%' ESCAPE '\'
       OR u.email LIKE '%' || sqlc.narg('search') || '%' ESCAPE '\')
GROUP BY u.id, p.photo_path, ed.level, ed.institution, cr.job_title, cr.company
ORDER BY CASE WHEN u.state = 'pending' THEN u.created_at END ASC NULLS LAST, u.name ASC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountDirectoryUsers :many
-- Paginator total for the same filter/search as ListDirectoryUsers.
SELECT u.state, count(*)::bigint AS total
FROM public.users u
WHERE (NOT COALESCE(sqlc.narg('active_only'), false) OR u.state = 'active')
  AND (sqlc.narg('state')::text IS NULL OR u.state = sqlc.narg('state'))
  AND (sqlc.narg('search')::text IS NULL
       OR u.name LIKE '%' || sqlc.narg('search') || '%' ESCAPE '\'
       OR u.email LIKE '%' || sqlc.narg('search') || '%' ESCAPE '\')
GROUP BY u.state;

-- name: CountUsersByState :many
-- The per-state badge counts the admin index header shows (array_sum = total).
SELECT state, count(*)::bigint AS total FROM public.users GROUP BY state;

-- name: GetDirectoryUserByID :one
-- Show page fetch: admins see any state, everyone else only active users
-- (reference show(): findOrFail scoped to state=active for non-admins).
SELECT u.id, u.name, u.email, u.state, u.phone, u.email_verified_at, u.created_at,
       p.photo_path, p.date_of_birth, p.gender, p.blood_group,
       p.present_address, p.permanent_address, p.social_links, p.website,
       p.emergency_contact, (p.id IS NOT NULL)::boolean AS has_profile,
       COALESCE(array_agg(r.name ORDER BY r.name) FILTER (WHERE r.name IS NOT NULL), '{}') AS role_names
FROM public.users u
LEFT JOIN public.profiles p ON p.user_id = u.id
LEFT JOIN model_has_roles mhr ON mhr.model_type = 'App\Models\User' AND mhr.model_id = u.id
LEFT JOIN roles r ON r.id = mhr.role_id
WHERE u.id = sqlc.arg(id)
  AND (NOT COALESCE(sqlc.narg('active_only'), false) OR u.state = 'active')
GROUP BY u.id, p.id;
