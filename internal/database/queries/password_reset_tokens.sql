-- name: UpsertPasswordResetToken :exec
INSERT INTO public.password_reset_tokens (email, token, created_at)
VALUES (sqlc.arg(email), sqlc.arg(token), now()::timestamp(0))
ON CONFLICT (email) DO UPDATE
SET token = EXCLUDED.token,
    created_at = EXCLUDED.created_at;

-- name: GetPasswordResetTokenByEmail :one
SELECT * FROM public.password_reset_tokens WHERE email = sqlc.arg(email);

-- name: DeletePasswordResetTokenByEmail :exec
DELETE FROM public.password_reset_tokens WHERE email = sqlc.arg(email);
