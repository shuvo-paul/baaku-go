-- Membership payment methods (reference App\Models\MembershipPaymentMethod):
-- dashboard-managed ways to pay, keyed by a unique type that doubles as the
-- label. Payments reference a method by `method` = type.

-- name: GetMembershipPaymentMethodByID :one
SELECT id, type, instructions, is_active, sort_order, created_at, updated_at
FROM public.membership_payment_methods
WHERE id = sqlc.arg(id);


-- name: MembershipPaymentMethodTypeExists :one
-- reference unique rule on type, ignoring the row being edited.
SELECT EXISTS (
    SELECT 1 FROM public.membership_payment_methods
    WHERE type = sqlc.arg(type) AND id != sqlc.arg(ignore_id)
);
-- name: ListActiveMembershipPaymentMethods :many
-- reference MembershipPaymentMethod::active(): is_active, ordered.
SELECT id, type, instructions, is_active, sort_order, created_at, updated_at
FROM public.membership_payment_methods
WHERE is_active = true
ORDER BY sort_order, type;

-- name: ListMembershipPaymentMethods :many
-- reference MembershipPaymentMethodController@index.
SELECT id, type, instructions, is_active, sort_order, created_at, updated_at
FROM public.membership_payment_methods
ORDER BY sort_order, type;

-- name: CountMembershipPaymentMethods :one
SELECT count(*) FROM public.membership_payment_methods;

-- name: MaxMembershipPaymentMethodSortOrder :one
SELECT COALESCE(MAX(sort_order), -1) FROM public.membership_payment_methods;

-- name: CreateMembershipPaymentMethod :one
INSERT INTO public.membership_payment_methods
    (type, instructions, is_active, sort_order, created_at, updated_at)
VALUES (sqlc.arg(type), sqlc.narg(instructions), sqlc.arg(is_active),
        sqlc.arg(sort_order), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: UpdateMembershipPaymentMethod :exec
UPDATE public.membership_payment_methods
SET type = sqlc.arg(type), instructions = sqlc.narg(instructions),
    is_active = sqlc.arg(is_active), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteMembershipPaymentMethod :exec
DELETE FROM public.membership_payment_methods WHERE id = sqlc.arg(id);

-- name: ReorderMembershipPaymentMethod :exec
UPDATE public.membership_payment_methods SET sort_order = sqlc.arg(sort_order), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);
