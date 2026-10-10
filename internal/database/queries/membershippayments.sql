-- Membership payments (reference App\Models\MembershipPayment). A member
-- submits a pending payment (with optional proof); staff approve/reject it,
-- and approval runs it through ActivateMembership.

-- name: GetMembershipPaymentByID :one
SELECT id, user_id, membership_plan_id, membership_id, amount, method,
       reference, paid_at, proof_path, notes, review_notes, status,
       reviewed_by, reviewed_at, created_by, created_at, updated_at
FROM public.membership_payments
WHERE id = sqlc.arg(id);


-- name: ListMembershipPaymentsByMembership :many
-- reference MembershipController@show's $membership->payments.
SELECT mp.id, mp.user_id, mp.membership_plan_id, mp.membership_id, mp.amount,
       mp.method, mp.reference, mp.paid_at, mp.proof_path, mp.notes,
       mp.review_notes, mp.status, mp.reviewed_by, mp.reviewed_at,
       mp.created_by, mp.created_at, mp.updated_at
FROM public.membership_payments mp
WHERE mp.membership_id = sqlc.arg(membership_id)
ORDER BY mp.id DESC;
-- name: GetPendingPaymentForUser :one
-- reference MyMembershipController@show's latest pending payment.
SELECT id, user_id, membership_plan_id, membership_id, amount, method,
       reference, paid_at, proof_path, notes, review_notes, status,
       reviewed_by, reviewed_at, created_by, created_at, updated_at
FROM public.membership_payments
WHERE user_id = sqlc.arg(user_id) AND status = 'pending'
ORDER BY id DESC
LIMIT 1;

-- name: ListUserMembershipPayments :many
-- reference MyMembershipController@show's newest-first payments page.
SELECT mp.id, mp.user_id, mp.membership_plan_id, mp.membership_id, mp.amount,
       mp.method, mp.reference, mp.paid_at, mp.proof_path, mp.notes,
       mp.review_notes, mp.status, mp.reviewed_by, mp.reviewed_at,
       mp.created_by, mp.created_at, mp.updated_at,
       p.name AS plan_name
FROM public.membership_payments mp
JOIN public.membership_plans p ON p.id = mp.membership_plan_id
WHERE mp.user_id = sqlc.arg(user_id)
ORDER BY mp.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.narg('offset');

-- name: CountUserMembershipPayments :one
SELECT count(*) FROM public.membership_payments WHERE user_id = sqlc.arg(user_id);

-- name: ListMembershipPayments :many
-- reference PaymentController@index, filtered by status (all = no filter).
SELECT mp.id, mp.user_id, mp.membership_plan_id, mp.membership_id, mp.amount,
       mp.method, mp.reference, mp.paid_at, mp.proof_path, mp.notes,
       mp.review_notes, mp.status, mp.reviewed_by, mp.reviewed_at,
       mp.created_by, mp.created_at, mp.updated_at,
       u.name AS user_name, u.email AS user_email,
       p.name AS plan_name
FROM public.membership_payments mp
JOIN public.users u ON u.id = mp.user_id
JOIN public.membership_plans p ON p.id = mp.membership_plan_id
WHERE (sqlc.narg('status')::text IS NULL OR mp.status = sqlc.narg('status'))
ORDER BY mp.id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.narg('offset');

-- name: CountMembershipPayments :one
SELECT count(*)
FROM public.membership_payments mp
WHERE (sqlc.narg('status')::text IS NULL OR mp.status = sqlc.narg('status'));

-- name: CreateMembershipPayment :one
INSERT INTO public.membership_payments
    (user_id, membership_plan_id, membership_id, amount, method, reference,
     paid_at, proof_path, notes, review_notes, status, reviewed_by, reviewed_at,
     created_by, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(membership_plan_id), sqlc.narg(membership_id),
        sqlc.arg(amount), sqlc.arg(method), sqlc.narg(reference),
        sqlc.arg(paid_at), sqlc.narg(proof_path), sqlc.narg(notes),
        sqlc.narg(review_notes), sqlc.arg(status), sqlc.narg(reviewed_by),
        sqlc.narg(reviewed_at), sqlc.narg(created_by), now()::timestamp(0), now()::timestamp(0))
RETURNING id;

-- name: ApproveMembershipPayment :exec
-- reference ActivateMembership payment update: mark approved + link membership.
UPDATE public.membership_payments
SET status = 'approved', reviewed_by = sqlc.narg(reviewed_by),
    reviewed_at = now()::timestamp(0), membership_id = sqlc.arg(membership_id),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: RejectMembershipPayment :exec
-- reference RejectPayment action.
UPDATE public.membership_payments
SET status = 'rejected', review_notes = sqlc.arg(review_notes),
    reviewed_by = sqlc.narg(reviewed_by), reviewed_at = now()::timestamp(0),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: LockMembershipPaymentForUpdate :one
-- reference ActivateMembership's lockForUpdate on the plan row (payment row).
SELECT id, user_id, membership_plan_id, membership_id, amount, method,
       reference, paid_at, proof_path, notes, review_notes, status,
       reviewed_by, reviewed_at, created_by, created_at, updated_at
FROM public.membership_payments
WHERE id = sqlc.arg(id)
FOR UPDATE;
