-- Membership plans (reference App\Models\MembershipPlan). A plan holds exactly
-- one term: a duration_days count, or lifetime (duration NULL).

-- name: GetMembershipPlanByID :one
SELECT id, name, description, price, duration_days, is_lifetime, features,
       sort_order, is_active, created_at, updated_at
FROM public.membership_plans
WHERE id = sqlc.arg(id);

-- name: ListActiveMembershipPlans :many
-- reference MembershipPlan::active(): is_active, ordered by sort_order.
SELECT id, name, description, price, duration_days, is_lifetime, features,
       sort_order, is_active, created_at, updated_at
FROM public.membership_plans
WHERE is_active = true
ORDER BY sort_order, id;

-- name: ListMembershipPlans :many
-- reference MembershipPlanController@index.
SELECT id, name, description, price, duration_days, is_lifetime, features,
       sort_order, is_active, created_at, updated_at
FROM public.membership_plans
ORDER BY sort_order, id;

-- name: CountMembershipPlans :one
SELECT count(*) FROM public.membership_plans;

-- name: MaxMembershipPlanSortOrder :one
SELECT COALESCE(MAX(sort_order), -1) FROM public.membership_plans;

-- name: CreateMembershipPlan :one
INSERT INTO public.membership_plans
    (name, description, price, duration_days, is_lifetime, features, sort_order, is_active, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.narg(description), sqlc.arg(price),
        sqlc.narg(duration_days), sqlc.arg(is_lifetime), sqlc.narg(features),
        sqlc.arg(sort_order), sqlc.arg(is_active), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdateMembershipPlan :exec
UPDATE public.membership_plans
SET name = sqlc.arg(name), description = sqlc.narg(description), price = sqlc.arg(price),
    duration_days = sqlc.narg(duration_days), is_lifetime = sqlc.arg(is_lifetime),
    features = sqlc.narg(features), sort_order = sqlc.arg(sort_order),
    is_active = sqlc.arg(is_active), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteMembershipPlan :exec
DELETE FROM public.membership_plans WHERE id = sqlc.arg(id);

-- name: MembershipPlanHasPayments :one
SELECT EXISTS (SELECT 1 FROM public.membership_payments mp WHERE mp.membership_plan_id = sqlc.arg(plan_id));

-- name: ReorderMembershipPlan :exec
UPDATE public.membership_plans SET sort_order = sqlc.arg(sort_order), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);
