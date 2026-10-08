-- name: GetUserByPhone :one
SELECT * FROM public.users WHERE phone = sqlc.arg(phone);

-- name: CreateProfile :one
INSERT INTO public.profiles (user_id, created_at, updated_at)
VALUES (sqlc.arg(user_id), now()::timestamp(0), now()::timestamp(0))
RETURNING *;


-- name: GetProfileCompleteness :one
-- Profile::isComplete() = gender AND blood_group non-null. A missing profile
-- row surfaces as pgx.ErrNoRows; callers treat that as incomplete.
SELECT gender IS NOT NULL AND blood_group IS NOT NULL AS complete
FROM public.profiles
WHERE user_id = sqlc.arg(user_id);


-- name: UpsertProfileDetails :one
-- CompleteProfileController store (reference UpdateProfileDetails +
-- firstOrCreate): fills the gate fields; a missing row is created so the
-- reference firstOrCreate semantics hold.
INSERT INTO public.profiles (user_id, gender, blood_group, present_address, permanent_address, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(gender), sqlc.arg(blood_group), sqlc.arg(present_address), sqlc.arg(permanent_address), now()::timestamp(0), now()::timestamp(0))
ON CONFLICT (user_id) DO UPDATE
SET gender = EXCLUDED.gender,
    blood_group = EXCLUDED.blood_group,
    present_address = EXCLUDED.present_address,
    permanent_address = EXCLUDED.permanent_address,
    updated_at = now()::timestamp(0)
RETURNING *;

-- name: CreateEducation :one
INSERT INTO public.educations (profile_id, level, institution, student_id, subject, is_current, start_year, start_month, end_year, end_month, created_at, updated_at)
VALUES (sqlc.arg(profile_id), sqlc.arg(level), sqlc.arg(institution), sqlc.arg(student_id), sqlc.arg(subject), sqlc.arg(is_current), sqlc.arg(start_year), sqlc.arg(start_month), sqlc.arg(end_year), sqlc.arg(end_month), now()::timestamp(0), now()::timestamp(0))
RETURNING *;
