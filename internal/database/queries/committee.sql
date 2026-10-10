-- Committee + positions (reference CommitteeController + PositionController +
-- App\Services\Committee). Members carry an optional registered user and an
-- optional free-text name; photos live on the public disk.

-- name: ListCommitteeMembers :many
-- Dashboard + public listings, both ordered by the dashboard drag order.
SELECT cm.id, cm.position_id, cm.user_id, cm.name, cm.photo_path, cm.sort_order,
       cm.created_at, cm.updated_at,
       pos.name AS position_name, u.name AS user_name, u.email AS user_email,
       p.photo_path AS user_photo_path
FROM public.committee_members cm
LEFT JOIN public.positions pos ON pos.id = cm.position_id
LEFT JOIN public.users u ON u.id = cm.user_id
LEFT JOIN public.profiles p ON p.user_id = u.id
ORDER BY cm.sort_order, cm.id;

-- name: GetCommitteeMemberByID :one
SELECT cm.id, cm.position_id, cm.user_id, cm.name, cm.photo_path, cm.sort_order,
       cm.created_at, cm.updated_at,
       pos.name AS position_name, u.name AS user_name, u.email AS user_email,
       p.photo_path AS user_photo_path
FROM public.committee_members cm
LEFT JOIN public.positions pos ON pos.id = cm.position_id
LEFT JOIN public.users u ON u.id = cm.user_id
LEFT JOIN public.profiles p ON p.user_id = u.id
WHERE cm.id = sqlc.arg(id);

-- name: MaxCommitteeMemberSortOrder :one
-- New members append to the end (reference CommitteeController@store).
SELECT COALESCE(MAX(sort_order), -1) FROM public.committee_members;

-- name: CreateCommitteeMember :one
INSERT INTO public.committee_members (position_id, user_id, name, photo_path, sort_order, created_at, updated_at)
VALUES (sqlc.arg(position_id), sqlc.narg(user_id), sqlc.narg(name), sqlc.narg(photo_path),
        sqlc.arg(sort_order), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdateCommitteeMember :exec
UPDATE public.committee_members
SET position_id = sqlc.arg(position_id), user_id = sqlc.narg(user_id),
    name = sqlc.narg(name), photo_path = sqlc.narg(photo_path),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteCommitteeMember :exec
DELETE FROM public.committee_members WHERE id = sqlc.arg(id);

-- name: ReorderCommitteeMember :exec
-- Assigns each id its list index as the new sort order (reference
-- CommitteeController@reorder).
UPDATE public.committee_members SET sort_order = sqlc.arg(sort_order), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: ListPositions :many
-- Reference PositionController@index: positions with the member count.
SELECT p.id, p.name, p.created_at, p.updated_at,
       (SELECT count(*) FROM public.committee_members cm WHERE cm.position_id = p.id)::bigint AS committee_members_count
FROM public.positions p
ORDER BY p.id;

-- name: GetPositionByID :one
SELECT id, name, created_at, updated_at FROM public.positions WHERE id = sqlc.arg(id);

-- name: CreatePosition :one
INSERT INTO public.positions (name, created_at, updated_at)
VALUES (sqlc.arg(name), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdatePosition :exec
UPDATE public.positions SET name = sqlc.arg(name), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeletePosition :exec
DELETE FROM public.positions WHERE id = sqlc.arg(id);

-- name: PositionNameExists :one
-- Unique-name validation, ignoring the row being edited (unique:positions,name,ID).
SELECT EXISTS (
    SELECT 1 FROM public.positions
    WHERE name = sqlc.arg(name) AND id <> sqlc.arg(id)
);

-- name: PositionIDExists :one
-- position_id exists:positions,id validation on the member form.
SELECT EXISTS (SELECT 1 FROM public.positions WHERE id = sqlc.arg(id));

-- name: UserIDExists :one
-- user_id exists:users,id validation on the member form.
SELECT EXISTS (SELECT 1 FROM public.users WHERE id = sqlc.arg(id));

-- name: SearchCommitteeUsers :many
-- The member-picker search (reference Livewire UserSearch): active users by
-- name or email, limit 8.
SELECT id, name, email FROM public.users
WHERE state = 'active'
  AND (name LIKE '%' || sqlc.arg(q) || '%'
       OR email LIKE '%' || sqlc.arg(q) || '%')
ORDER BY name
LIMIT 8;
