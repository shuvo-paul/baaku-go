-- Memberships (reference App\Models\Membership + HasMemberships trait). A
-- membership is the user's current term on a plan; activeMembership is the
-- one that gates dashboard features.

-- name: GetMembershipByID :one
SELECT id, user_id, membership_plan_id, status, starts_at, ends_at,
       cancelled_at, notes, created_by, created_at, updated_at
FROM public.memberships
WHERE id = sqlc.arg(id);

-- name: GetActiveMembershipForUser :one
-- reference $user->activeMembership(): status active and not yet ended.
SELECT id, user_id, membership_plan_id, status, starts_at, ends_at,
       cancelled_at, notes, created_by, created_at, updated_at
FROM public.memberships
WHERE user_id = sqlc.arg(user_id)
  AND status = 'active'
  AND (ends_at IS NULL OR ends_at > now())
ORDER BY id DESC
LIMIT 1;

-- name: GetLatestMembershipForUser :one
-- reference $user->latestMembership(): the most recent row.
SELECT id, user_id, membership_plan_id, status, starts_at, ends_at,
       cancelled_at, notes, created_by, created_at, updated_at
FROM public.memberships
WHERE user_id = sqlc.arg(user_id)
ORDER BY id DESC
LIMIT 1;

-- name: ListMemberships :many
-- reference MembershipController@index, filtered by status (all = no filter).
SELECT m.id, m.user_id, m.membership_plan_id, m.status, m.starts_at, m.ends_at,
       m.cancelled_at, m.notes, m.created_by, m.created_at, m.updated_at,
       u.name AS user_name, u.email AS user_email,
       p.name AS plan_name
FROM public.memberships m
JOIN public.users u ON u.id = m.user_id
JOIN public.membership_plans p ON p.id = m.membership_plan_id
WHERE (sqlc.narg('status')::text IS NULL OR m.status = sqlc.narg('status'))
ORDER BY m.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.narg('offset');

-- name: CountMemberships :one
SELECT count(*)
FROM public.memberships m
WHERE (sqlc.narg('status')::text IS NULL OR m.status = sqlc.narg('status'));

-- name: LockActiveMembershipForUser :one
-- reference ActivateMembership's lockForUpdate() active-membership lookup.
SELECT id, user_id, membership_plan_id, status, starts_at, ends_at,
       cancelled_at, notes, created_by, created_at, updated_at
FROM public.memberships
WHERE user_id = sqlc.arg(user_id)
  AND status = 'active'
FOR UPDATE
LIMIT 1;

-- name: CreateMembership :one
INSERT INTO public.memberships
    (user_id, membership_plan_id, status, starts_at, ends_at, cancelled_at, notes, created_by, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(membership_plan_id), sqlc.arg(status),
        sqlc.narg(starts_at), sqlc.narg(ends_at), sqlc.narg(cancelled_at),
        sqlc.narg(notes), sqlc.narg(created_by), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdateMembershipTerm :exec
-- reference ActivateMembership extend path: re-plan and push the end date.
UPDATE public.memberships
SET membership_plan_id = sqlc.arg(membership_plan_id), ends_at = sqlc.narg(ends_at), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: UpdateMembership :exec
-- reference UpdateMembership action: staff adjusts end date + notes only.
UPDATE public.memberships
SET ends_at = sqlc.narg(ends_at), notes = sqlc.narg(notes), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: CancelMembership :exec
-- reference CancelMembership action.
UPDATE public.memberships
SET status = 'cancelled', cancelled_at = now()::timestamp(0), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: ExpireMemberships :execrows
-- reference ExpireMembershipsCommand: flip active, non-lifetime, lapsed rows.
UPDATE public.memberships m
SET status = 'expired', updated_at = now()::timestamp(0)
FROM public.membership_plans p
WHERE p.id = m.membership_plan_id
  AND m.status = 'active'
  AND p.is_lifetime = false
  AND m.ends_at IS NOT NULL
  AND m.ends_at <= now();

-- name: MembershipPlanHasMemberships :one
SELECT EXISTS (SELECT 1 FROM public.memberships m WHERE m.membership_plan_id = sqlc.arg(plan_id));
