-- name: GetUserByID :one
SELECT * FROM public.users WHERE id = sqlc.arg(id);

-- name: GetUserByEmail :one
SELECT * FROM public.users WHERE email = sqlc.arg(email);

-- name: CreateUser :one
INSERT INTO public.users (name, email, password, phone, created_at, updated_at)
VALUES (sqlc.arg(name), sqlc.arg(email), sqlc.arg(password), sqlc.arg(phone), now()::timestamp(0), now()::timestamp(0))
RETURNING *;

-- name: UpdateUserPassword :exec
UPDATE public.users
SET password = sqlc.arg(password), remember_token = NULL, updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: VerifyUserEmail :exec
UPDATE public.users
SET email_verified_at = now()::timestamp(0), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);
