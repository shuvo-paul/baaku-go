// Package career reads and writes user-owned career rows for the profile
// page (reference ProfileCareerController).
package career

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/career"
)

// Repo is a thin wrapper over the sqlc-generated careers queries. It returns
// the service-layer domain type, matching repository/user → service/user.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// List returns the user's careers newest-first (orderByDesc start_year).
func (r *Repo) List(ctx context.Context, userID int64) ([]career.Career, error) {
	rows, err := r.q.GetUserCareers(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]career.Career, 0, len(rows))
	for _, c := range rows {
		out = append(out, fromGenerated(c))
	}
	return out, nil
}

// Get fetches one career scoped to the owner (findOrFail → pgx.ErrNoRows).
func (r *Repo) Get(ctx context.Context, userID, id int64) (career.Career, error) {
	c, err := r.q.GetCareerForProfile(ctx, generated.GetCareerForProfileParams{ID: id, UserID: userID})
	if err != nil {
		return career.Career{}, err
	}
	return fromGenerated(c), nil
}

// ProfileID resolves the user's profile_id (careers hang off profiles).
func (r *Repo) ProfileID(ctx context.Context, userID int64) (int64, error) {
	return r.q.GetProfileIDByUserID(ctx, userID)
}

// Create inserts a career for the given profile.
func (r *Repo) Create(ctx context.Context, profileID int64, c career.Career) error {
	_, err := r.q.CreateCareerForProfile(ctx, generated.CreateCareerForProfileParams{
		ProfileID:      profileID,
		JobTitle:       c.JobTitle,
		Company:        c.Company,
		EmploymentType: c.EmploymentType,
		Industry:       c.Industry,
		Location:       c.Location,
		StartYear:      c.StartYear,
		StartMonth:     c.StartMonth,
		IsCurrent:      c.IsCurrent,
		EndYear:        c.EndYear,
		EndMonth:       c.EndMonth,
		Description:    c.Description,
	})
	return err
}

// Update saves a career by id (the handler already ownership-checked it).
func (r *Repo) Update(ctx context.Context, id int64, c career.Career) error {
	return r.q.UpdateCareer(ctx, generated.UpdateCareerParams{
		ID:             id,
		JobTitle:       c.JobTitle,
		Company:        c.Company,
		EmploymentType: c.EmploymentType,
		Industry:       c.Industry,
		Location:       c.Location,
		StartYear:      c.StartYear,
		StartMonth:     c.StartMonth,
		IsCurrent:      c.IsCurrent,
		EndYear:        c.EndYear,
		EndMonth:       c.EndMonth,
		Description:    c.Description,
	})
}

// Delete removes a career by id.
func (r *Repo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteCareer(ctx, id)
}

func fromGenerated(c generated.Career) career.Career {
	return career.Career{
		ID:             c.ID,
		JobTitle:       c.JobTitle,
		Company:        c.Company,
		EmploymentType: c.EmploymentType,
		Industry:       c.Industry,
		Location:       c.Location,
		StartYear:      c.StartYear,
		StartMonth:     c.StartMonth,
		IsCurrent:      c.IsCurrent,
		EndYear:        c.EndYear,
		EndMonth:       c.EndMonth,
		Description:    c.Description,
	}
}
