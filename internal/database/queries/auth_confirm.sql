-- name: UpsertPasswordConfirmation :exec
INSERT INTO public.cache (key, value, expiration)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(expiration))
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    expiration = EXCLUDED.expiration;

-- name: GetPasswordConfirmation :one
SELECT * FROM public.cache WHERE key = sqlc.arg(key);
