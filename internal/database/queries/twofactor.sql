-- name: Claim2FAReplayCode :execrows
-- Claims a TOTP code for single use, mirroring Fortify's cache-backed
-- verifyKeyNewer replay guard (TwoFactorAuthenticationProvider::verify).
-- TTL convention: (window ?: 1) * 60 seconds. Rows affected is 1 when the
-- code is newly claimed, 0 when a live claim already exists.
INSERT INTO public.cache (key, value, expiration)
VALUES (sqlc.arg(key), sqlc.arg(value), sqlc.arg(expiration))
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, expiration = EXCLUDED.expiration
WHERE public.cache.expiration <= sqlc.arg(used_at);
