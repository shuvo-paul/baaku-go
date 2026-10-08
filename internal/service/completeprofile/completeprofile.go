// Package completeprofile ports the store half of the reference
// CompleteProfileController (routes/profile.php profile.complete.store) — the
// gate fields only: gender, blood_group, present_address, permanent_address.
// The careers repeater + photo/local-names wizard of
// resources/views/auth/complete-profile.blade.php lands in a later wave.
package completeprofile

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/service/register"
)

// FieldErrors mirrors the reference validator output. Same shape as the
// register service's map — reused as an alias, same semantics.
type FieldErrors = register.FieldErrors

// Values mirror app/Enums/Gender.php and app/Enums/BloodGroup.php.
var (
	genders     = map[string]bool{"male": true, "female": true, "other": true, "prefer_not_to_say": true}
	bloodGroups = map[string]bool{"A+": true, "A-": true, "B+": true, "B-": true, "AB+": true, "AB-": true, "O+": true, "O-": true}
)

// Input is the complete-profile form payload (gate fields).
type Input struct {
	Gender           string
	BloodGroup       string
	PresentAddress   string
	PermanentAddress string
}

// Store persists the gate fields; *repository/profile.Repo satisfies it.
type Store interface {
	UpsertDetails(ctx context.Context, userID int64, gender, bloodGroup, presentAddress, permanentAddress string) error
}

// Service completes a user's profile.
type Service struct {
	profiles Store
}

func New(store Store) *Service { return &Service{profiles: store} }

// Complete validates in exactly like the reference store (required + enum
// membership + max:255) and persists via the profile repo.
func (s *Service) Complete(ctx context.Context, userID int64, in Input) error {
	if errs := validate(in); len(errs) > 0 {
		return errs
	}
	return s.profiles.UpsertDetails(ctx, userID, in.Gender, in.BloodGroup, in.PresentAddress, in.PermanentAddress)
}

// validate mirrors the controller's validator (reference ProfileDetailsRequest
// + the gender/blood_group rules added in CompleteProfileController::store).
// Messages are Laravel defaults with humanized attributes ("blood group").
func validate(in Input) FieldErrors {
	errs := FieldErrors{}

	switch {
	case in.Gender == "":
		errs["gender"] = "The gender field is required."
	case !genders[in.Gender]:
		errs["gender"] = "The selected gender is invalid."
	}

	switch {
	case in.BloodGroup == "":
		errs["blood_group"] = "The blood group field is required."
	case !bloodGroups[in.BloodGroup]:
		errs["blood_group"] = "The selected blood group is invalid."
	}

	address := func(field, label, v string) {
		switch {
		case v == "":
			errs[field] = "The " + label + " field is required."
		case len(v) > 255:
			errs[field] = "The " + label + " field must not be greater than 255 characters."
		}
	}
	address("present_address", "present address", in.PresentAddress)
	address("permanent_address", "permanent address", in.PermanentAddress)

	return errs
}
