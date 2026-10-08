// Package profile reads profile rows for the complete-profile gate.
package profile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

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
