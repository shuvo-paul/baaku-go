-- name: GetSessionByID :one
SELECT * FROM public.sessions WHERE id = sqlc.arg(id);

-- name: UpsertSession :exec
INSERT INTO public.sessions (id, user_id, ip_address, user_agent, payload, last_activity)
VALUES (sqlc.arg(id), sqlc.arg(user_id), sqlc.arg(ip_address), sqlc.arg(user_agent), sqlc.arg(payload), sqlc.arg(last_activity))
ON CONFLICT (id) DO UPDATE
SET user_id = EXCLUDED.user_id,
    ip_address = EXCLUDED.ip_address,
    user_agent = EXCLUDED.user_agent,
    payload = EXCLUDED.payload,
    last_activity = EXCLUDED.last_activity;

-- name: DeleteSessionByID :exec
DELETE FROM public.sessions WHERE id = sqlc.arg(id);

-- name: DeleteSessionsByUserID :exec
DELETE FROM public.sessions WHERE user_id = sqlc.arg(user_id);

-- name: DeleteExpiredSessions :exec
DELETE FROM public.sessions WHERE last_activity < sqlc.arg(cutoff);

-- name: PromoteSession :exec
-- Attaches an authenticated user to a pending (2FA-challenge) session row
-- and replaces its payload, clearing the pending-login flag.
UPDATE public.sessions
SET user_id = sqlc.arg(user_id),
    payload = sqlc.arg(payload),
    last_activity = now()::timestamp(0)
WHERE id = sqlc.arg(id);
