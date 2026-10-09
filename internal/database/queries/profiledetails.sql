-- Profile details, educations and careers for the dashboard profile page
-- (reference ProfileDetailsController@update, ProfileEducationController,
-- ProfileCareerController).

-- name: GetUserProfile :one
-- Full profile row for pre-filling the details form (reference
-- Auth::user()->profile on GET dashboard/profile).
SELECT * FROM public.profiles WHERE user_id = sqlc.arg(user_id);

-- name: UpsertProfileFull :one
-- ProfileDetailsController@update (reference UpdateProfileDetails action):
-- writes every details-form column in one upsert. A missing row is created
-- (firstOrCreate semantics). JSON columns arrive as raw []byte (nil = SQL
-- NULL); date_of_birth arrives as pgtype.Date (Valid=false → NULL).
INSERT INTO public.profiles (user_id, photo_path, date_of_birth, gender, blood_group, present_address, permanent_address, social_links, website, emergency_contact, local_names, created_at, updated_at)
VALUES (sqlc.arg(user_id), sqlc.arg(photo_path), sqlc.arg(date_of_birth), sqlc.arg(gender), sqlc.arg(blood_group), sqlc.arg(present_address), sqlc.arg(permanent_address), sqlc.arg(social_links), sqlc.arg(website), sqlc.arg(emergency_contact), sqlc.arg(local_names), now()::timestamp(0), now()::timestamp(0))
ON CONFLICT (user_id) DO UPDATE
SET photo_path = EXCLUDED.photo_path,
    date_of_birth = EXCLUDED.date_of_birth,
    gender = EXCLUDED.gender,
    blood_group = EXCLUDED.blood_group,
    present_address = EXCLUDED.present_address,
    permanent_address = EXCLUDED.permanent_address,
    social_links = EXCLUDED.social_links,
    website = EXCLUDED.website,
    emergency_contact = EXCLUDED.emergency_contact,
    local_names = EXCLUDED.local_names,
    updated_at = now()::timestamp(0)
RETURNING *;

-- name: UpdateUserContact :exec
-- ProfileDetailsController@update also updates phone on the parent user
-- (reference UpdateProfileDetails: Arr::only($data, ['name','email','phone'])).
-- name/email go through the Fortify UpdateUserProfileInformation path
-- (profileinfo service); phone lands here.
UPDATE public.users
SET phone = sqlc.arg(phone), updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: GetProfileIDByUserID :one
-- Resolves profile_id so educations/careers can be scoped to the owner.
SELECT id FROM public.profiles WHERE user_id = sqlc.arg(user_id);

-- Educations (reference ProfileEducationController — user-owned via profile).

-- name: GetUserEducations :many
-- Education list on the profile page, newest first (orderByDesc start_year).
SELECT e.* FROM public.educations e
JOIN public.profiles p ON p.id = e.profile_id
WHERE p.user_id = sqlc.arg(user_id)
ORDER BY e.start_year DESC, e.id DESC;

-- name: GetEducationForProfile :one
-- Ownership-scoped fetch (findOrFail on $user->educations()): a row belonging
-- to another profile surfaces as pgx.ErrNoRows → 404.
SELECT e.* FROM public.educations e
JOIN public.profiles p ON p.id = e.profile_id
WHERE e.id = sqlc.arg(id) AND p.user_id = sqlc.arg(user_id);

-- name: CreateEducationForProfile :one
INSERT INTO public.educations (profile_id, level, institution, student_id, subject, is_current, start_year, start_month, end_year, end_month, created_at, updated_at)
VALUES (sqlc.arg(profile_id), sqlc.arg(level), sqlc.arg(institution), sqlc.arg(student_id), sqlc.arg(subject), sqlc.arg(is_current), sqlc.arg(start_year), sqlc.arg(start_month), sqlc.arg(end_year), sqlc.arg(end_month), now()::timestamp(0), now()::timestamp(0))
RETURNING *;

-- name: UpdateEducation :exec
UPDATE public.educations
SET level = sqlc.arg(level),
    institution = sqlc.arg(institution),
    student_id = sqlc.arg(student_id),
    subject = sqlc.arg(subject),
    is_current = sqlc.arg(is_current),
    start_year = sqlc.arg(start_year),
    start_month = sqlc.arg(start_month),
    end_year = sqlc.arg(end_year),
    end_month = sqlc.arg(end_month),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteEducation :exec
DELETE FROM public.educations WHERE id = sqlc.arg(id);

-- Careers (reference ProfileCareerController — user-owned via profile).

-- name: GetUserCareers :many
SELECT c.* FROM public.careers c
JOIN public.profiles p ON p.id = c.profile_id
WHERE p.user_id = sqlc.arg(user_id)
ORDER BY c.start_year DESC, c.id DESC;

-- name: GetCareerForProfile :one
SELECT c.* FROM public.careers c
JOIN public.profiles p ON p.id = c.profile_id
WHERE c.id = sqlc.arg(id) AND p.user_id = sqlc.arg(user_id);

-- name: CreateCareerForProfile :one
INSERT INTO public.careers (profile_id, job_title, company, employment_type, industry, location, start_year, start_month, is_current, end_year, end_month, description, created_at, updated_at)
VALUES (sqlc.arg(profile_id), sqlc.arg(job_title), sqlc.arg(company), sqlc.arg(employment_type), sqlc.arg(industry), sqlc.arg(location), sqlc.arg(start_year), sqlc.arg(start_month), sqlc.arg(is_current), sqlc.arg(end_year), sqlc.arg(end_month), sqlc.arg(description), now()::timestamp(0), now()::timestamp(0))
RETURNING *;

-- name: UpdateCareer :exec
UPDATE public.careers
SET job_title = sqlc.arg(job_title),
    company = sqlc.arg(company),
    employment_type = sqlc.arg(employment_type),
    industry = sqlc.arg(industry),
    location = sqlc.arg(location),
    start_year = sqlc.arg(start_year),
    start_month = sqlc.arg(start_month),
    is_current = sqlc.arg(is_current),
    end_year = sqlc.arg(end_year),
    end_month = sqlc.arg(end_month),
    description = sqlc.arg(description),
    updated_at = now()::timestamp(0)
WHERE id = sqlc.arg(id);

-- name: DeleteCareer :exec
DELETE FROM public.careers WHERE id = sqlc.arg(id);
