// Package career ports ProfileCareerController (reference) — the user-owned
// career CRUD behind the profile page. Validation mirrors
// StoreProfileCareerRequest / UpdateProfileCareerRequest (identical rules);
// every mutation re-submits a rejected profile for review.
package career

import (
	"context"
	"strconv"
)

// Career is the domain representation of a careers row.
type Career struct {
	ID             int64
	JobTitle       string
	Company        string
	EmploymentType string
	Industry       *string
	Location       *string
	StartYear      int32
	StartMonth     *int16
	IsCurrent      bool
	EndYear        *int32
	EndMonth       *int16
	Description    *string
}

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// employmentTypes mirrors app/Enums/EmploymentType.php.
var employmentTypes = map[string]bool{
	"full_time": true, "part_time": true, "contract": true,
	"freelance": true, "internship": true,
}

// Input is the career form payload.
type Input struct {
	EmploymentType string
	JobTitle       string
	Company        string
	Industry       string
	Location       string
	StartYear      string
	StartMonth     string
	IsCurrent      bool
	EndYear        string
	EndMonth       string
	Description    string
}

// Store is the persistence slice the service needs; *repository/career Repo
// satisfies it.
type Store interface {
	List(ctx context.Context, userID int64) ([]Career, error)
	Get(ctx context.Context, userID, id int64) (Career, error)
	ProfileID(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, profileID int64, c Career) error
	Update(ctx context.Context, id int64, c Career) error
	Delete(ctx context.Context, id int64) error
}

// Reviewer is the SubmitProfileForReview port (reference Action).
type Reviewer interface {
	Submit(ctx context.Context, userID int64) error
}

type Service struct {
	store    Store
	reviewer Reviewer
}

func New(store Store, reviewer Reviewer) *Service {
	return &Service{store: store, reviewer: reviewer}
}

func (s *Service) List(ctx context.Context, userID int64) ([]Career, error) {
	return s.store.List(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id int64) (Career, error) {
	return s.store.Get(ctx, userID, id)
}

// Create validates, inserts under the user's profile, then re-submits for
// review (reference ProfileCareerController@store).
func (s *Service) Create(ctx context.Context, userID int64, in Input) error {
	c, errs := build(in)
	if len(errs) > 0 {
		return errs
	}
	profileID, err := s.store.ProfileID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.store.Create(ctx, profileID, c); err != nil {
		return err
	}
	return s.reviewer.Submit(ctx, userID)
}

// Update validates, saves the owned row, then re-submits for review.
func (s *Service) Update(ctx context.Context, userID, id int64, in Input) error {
	c, errs := build(in)
	if len(errs) > 0 {
		return errs
	}
	cur, err := s.store.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	c.ID = cur.ID
	if err := s.store.Update(ctx, id, c); err != nil {
		return err
	}
	return s.reviewer.Submit(ctx, userID)
}

// Delete removes the owned row and re-submits for review.
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	if _, err := s.store.Get(ctx, userID, id); err != nil {
		return err
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	return s.reviewer.Submit(ctx, userID)
}

// build converts and validates the form input (reference career rules).
func build(in Input) (Career, FieldErrors) {
	errs := FieldErrors{}
	c := Career{
		JobTitle:  in.JobTitle,
		Company:   in.Company,
		IsCurrent: in.IsCurrent,
	}
	if !employmentTypes[in.EmploymentType] {
		errs["employment_type"] = "The selected employment type is invalid."
	} else {
		c.EmploymentType = in.EmploymentType
	}
	if v := in.Industry; v != "" {
		c.Industry = &v
	}
	if v := in.Location; v != "" {
		c.Location = &v
	}
	if v := in.Description; v != "" {
		c.Description = &v
	}

	startYear, ok := year("start_year", in.StartYear, true, errs)
	if ok {
		c.StartYear = startYear
	}
	c.StartMonth = month("start_month", in.StartMonth, errs)

	if !in.IsCurrent {
		if in.EndYear != "" {
			if y, ok := year("end_year", in.EndYear, false, errs); ok {
				if y < c.StartYear {
					errs["end_year"] = "The end year must be greater than or equal to start year."
				} else {
					c.EndYear = &y
				}
			}
		}
		c.EndMonth = month("end_month", in.EndMonth, errs)
	}

	required("job_title", "job title", in.JobTitle, 255, errs)
	required("company", "company", in.Company, 255, errs)
	optMax("industry", "industry", in.Industry, 255, errs)
	optMax("location", "location", in.Location, 255, errs)
	optMax("description", "description", in.Description, 5000, errs)

	if len(errs) > 0 {
		return Career{}, errs
	}
	return c, nil
}

func required(field, label, v string, max int, errs FieldErrors) {
	switch {
	case v == "":
		errs[field] = "The " + label + " field is required."
	case len(v) > max:
		errs[field] = "The " + label + " field must not be greater than " + strconv.Itoa(max) + " characters."
	}
}

func optMax(field, label, v string, max int, errs FieldErrors) {
	if len(v) > max {
		errs[field] = "The " + label + " field must not be greater than " + strconv.Itoa(max) + " characters."
	}
}

// year parses a 4-digit year in [1900, 2099]; required when want is true.
func year(field, v string, want bool, errs FieldErrors) (int32, bool) {
	if v == "" {
		if want {
			errs[field] = "The " + field + " field is required."
		}
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1900 || n > 2099 || len(v) != 4 {
		errs[field] = "The " + field + " must be a 4-digit year between 1900 and 2099."
		return 0, false
	}
	return int32(n), true
}

// month parses an optional month in [1, 12].
func month(field, v string, errs FieldErrors) *int16 {
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 12 {
		errs[field] = "The " + field + " must be between 1 and 12."
		return nil
	}
	m := int16(n)
	return &m
}
