// Package education reads and writes user-owned education rows for the
// profile page (reference ProfileEducationController).
package education

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/education"
)

// Repo is a thin wrapper over the sqlc-generated educations queries. It
// returns the service-layer domain type, matching repository/user →
// service/user.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// List returns the user's educations newest-first (orderByDesc start_year).
func (r *Repo) List(ctx context.Context, userID int64) ([]education.Education, error) {
	rows, err := r.q.GetUserEducations(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]education.Education, 0, len(rows))
	for _, e := range rows {
		out = append(out, fromGenerated(e))
	}
	return out, nil
}

// Get fetches one education scoped to the owner; a row owned by another
// profile surfaces as pgx.ErrNoRows (reference findOrFail → 404).
func (r *Repo) Get(ctx context.Context, userID, id int64) (education.Education, error) {
	e, err := r.q.GetEducationForProfile(ctx, generated.GetEducationForProfileParams{ID: id, UserID: userID})
	if err != nil {
		return education.Education{}, err
	}
	return fromGenerated(e), nil
}

// ProfileID resolves the user's profile_id (educations hang off profiles).
func (r *Repo) ProfileID(ctx context.Context, userID int64) (int64, error) {
	return r.q.GetProfileIDByUserID(ctx, userID)
}

// Create inserts an education for the given profile.
func (r *Repo) Create(ctx context.Context, profileID int64, e education.Education) error {
	_, err := r.q.CreateEducationForProfile(ctx, generated.CreateEducationForProfileParams{
		ProfileID:   profileID,
		Level:       e.Level,
		Institution: e.Institution,
		StudentID:   e.StudentID,
		Subject:     e.Subject,
		IsCurrent:   e.IsCurrent,
		StartYear:   e.StartYear,
		StartMonth:  e.StartMonth,
		EndYear:     e.EndYear,
		EndMonth:    e.EndMonth,
	})
	return err
}

// Update saves an education by id (the handler already ownership-checked it).
func (r *Repo) Update(ctx context.Context, id int64, e education.Education) error {
	return r.q.UpdateEducation(ctx, generated.UpdateEducationParams{
		ID:          id,
		Level:       e.Level,
		Institution: e.Institution,
		StudentID:   e.StudentID,
		Subject:     e.Subject,
		IsCurrent:   e.IsCurrent,
		StartYear:   e.StartYear,
		StartMonth:  e.StartMonth,
		EndYear:     e.EndYear,
		EndMonth:    e.EndMonth,
	})
}

// Delete removes an education by id.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteEducation(ctx, id)
}

func fromGenerated(e generated.Education) education.Education {
	return education.Education{
		ID:          e.ID,
		Level:       e.Level,
		Institution: e.Institution,
		StudentID:   e.StudentID,
		Subject:     e.Subject,
		IsCurrent:   e.IsCurrent,
		StartYear:   e.StartYear,
		StartMonth:  e.StartMonth,
		EndYear:     e.EndYear,
		EndMonth:    e.EndMonth,
	}
}
