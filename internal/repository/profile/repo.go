// Package profile reads profile rows for the complete-profile gate.
package profile

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Repo is a thin wrapper over the sqlc-generated profiles queries.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// Complete reports whether the user's profile satisfies Profile::isComplete()
// (gender AND blood_group non-null). A missing profile row reads as
// incomplete, matching $user->profile?->isComplete() on a null relation.
func (r *Repo) Complete(ctx context.Context, userID int64) (bool, error) {
	complete, err := r.q.GetProfileCompleteness(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return complete != nil && *complete, nil
}

// UpsertDetails fills the complete-profile gate fields
// (reference UpdateProfileDetails + firstOrCreate); see the query comment.
func (r *Repo) UpsertDetails(ctx context.Context, userID int64, gender, bloodGroup, presentAddress, permanentAddress string) error {
	_, err := r.q.UpsertProfileDetails(ctx, generated.UpsertProfileDetailsParams{
		UserID:           userID,
		Gender:           &gender,
		BloodGroup:       &bloodGroup,
		PresentAddress:   &presentAddress,
		PermanentAddress: &permanentAddress,
	})
	return err
}

// Profile is the domain representation of a profiles row for the details
// form. JSON columns are decoded into typed maps; DateOfBirth is a plain
// YYYY-MM-DD string ("" when unset).
type Profile struct {
	ID               int64
	UserID           int64
	PhotoPath        *string
	DateOfBirth      string
	Gender           *string
	BloodGroup       *string
	PresentAddress   *string
	PermanentAddress *string
	SocialLinks      map[string]string
	Website          *string
	EmergencyContact map[string]string
	LocalNames       map[string]string
}

// Get fetches the full profile row for pre-filling the details form; a
// missing row surfaces as pgx.ErrNoRows.
func (r *Repo) Get(ctx context.Context, userID int64) (Profile, error) {
	p, err := r.q.GetUserProfile(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	return profileFromGenerated(p), nil
}

// UpsertDetailsFull writes every details-form column in one upsert
// (reference ProfileDetailsController@update via UpdateProfileDetails).
func (r *Repo) UpsertDetailsFull(ctx context.Context, p Profile) error {
	dob, err := parseDate(p.DateOfBirth)
	if err != nil {
		return err
	}
	_, err = r.q.UpsertProfileFull(ctx, generated.UpsertProfileFullParams{
		UserID:           p.UserID,
		PhotoPath:        p.PhotoPath,
		DateOfBirth:      dob,
		Gender:           p.Gender,
		BloodGroup:       p.BloodGroup,
		PresentAddress:   p.PresentAddress,
		PermanentAddress: p.PermanentAddress,
		SocialLinks:      marshalJSON(p.SocialLinks),
		Website:          p.Website,
		EmergencyContact: marshalJSON(p.EmergencyContact),
		LocalNames:       marshalJSON(p.LocalNames),
	})
	return err
}

// UpdateUserContact sets phone on the parent user (details form).
func (r *Repo) UpdateUserContact(ctx context.Context, userID int64, phone *string) error {
	return r.q.UpdateUserContact(ctx, generated.UpdateUserContactParams{ID: userID, Phone: phone})
}

// profileFromGenerated maps a generated profiles row to the domain Profile,
// decoding the json columns into maps (nil/empty → empty map).
func profileFromGenerated(p generated.Profile) Profile {
	return Profile{
		ID:               p.ID,
		UserID:           p.UserID,
		PhotoPath:        p.PhotoPath,
		DateOfBirth:      formatDate(p.DateOfBirth),
		Gender:           p.Gender,
		BloodGroup:       p.BloodGroup,
		PresentAddress:   p.PresentAddress,
		PermanentAddress: p.PermanentAddress,
		SocialLinks:      unmarshalMap(p.SocialLinks),
		Website:          p.Website,
		EmergencyContact: unmarshalMap(p.EmergencyContact),
		LocalNames:       unmarshalMap(p.LocalNames),
	}
}

// parseDate turns a YYYY-MM-DD string into a pgtype.Date; "" → invalid (NULL).
func parseDate(s string) (pgtype.Date, error) {
	if s == "" {
		return pgtype.Date{}, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// formatDate renders a pgtype.Date as YYYY-MM-DD ("" when unset).
func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// marshalJSON encodes a map for a json column; an empty map → nil (SQL NULL),
// mirroring the reference array_filter(...) ?: null semantics.
func marshalJSON(m map[string]string) []byte {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// unmarshalMap decodes a json column into a map[string]string.
func unmarshalMap(b []byte) map[string]string {
	m := map[string]string{}
	if len(b) == 0 {
		return m
	}
	_ = json.Unmarshal(b, &m)
	return m
}
