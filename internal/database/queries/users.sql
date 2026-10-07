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

-- name: UpdateUserState :exec
UPDATE public.users
SET state = sqlc.arg(state), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: SetUserTwoFactor :exec
UPDATE public.users
SET two_factor_secret = sqlc.arg(two_factor_secret),
    two_factor_recovery_codes = sqlc.arg(two_factor_recovery_codes),
    two_factor_confirmed_at = sqlc.arg(two_factor_confirmed_at),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: ClearUserTwoFactor :exec
-- Wipes the 2FA secret, recovery codes, and confirmation time in one write
-- (Fortify DisableTwoFactorAuthentication clears all three columns).
UPDATE public.users
SET two_factor_secret = NULL,
    two_factor_recovery_codes = NULL,
    two_factor_confirmed_at = NULL,
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: UpdateUserProfile :exec
-- Saves name + email (UpdateUserProfileInformation); the caller decides
-- email_verified_at — nil when the email changed, current value otherwise.
UPDATE public.users
SET name = sqlc.arg(name),
    email = sqlc.arg(email),
    email_verified_at = sqlc.arg(email_verified_at),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);
