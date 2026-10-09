// Package education ports ProfileEducationController (reference) — the
// user-owned education CRUD behind the profile page. Validation mirrors
// StoreProfileEducationRequest / UpdateProfileEducationRequest (identical
// rules); every mutation re-submits a rejected profile for review.
package education

import (
	"context"
	"strconv"
)

// Education is the domain representation of an educations row.
type Education struct {
	ID          int64
	Level       string
	Institution string
	StudentID   *string
	Subject     string
	IsCurrent   bool
	StartYear   int32
	StartMonth  *int16
	EndYear     *int32
	EndMonth    *int16
}

type FieldErrors map[string]string

func (e FieldErrors) Error() string { return firstMessage(e) }

func firstMessage(e FieldErrors) string {
	for _, v := range e {
		return v
	}
	return ""
}

// Input is the education form payload.
type Input struct {
	Level       string
	Institution string
	StudentID   string
	Subject     string
	StartYear   string
	StartMonth  string
	IsCurrent   bool
	EndYear     string
	EndMonth    string
}

// Store is the persistence slice the service needs; *repository/education
// Repo satisfies it.
type Store interface {
	List(ctx context.Context, userID int64) ([]Education, error)
	Get(ctx context.Context, userID, id int64) (Education, error)
	ProfileID(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, profileID int64, e Education) error
	Update(ctx context.Context, id int64, e Education) error
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

func (s *Service) List(ctx context.Context, userID int64) ([]Education, error) {
	return s.store.List(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id int64) (Education, error) {
	return s.store.Get(ctx, userID, id)
}

// Create validates, inserts under the user's profile, then re-submits for
// review (reference ProfileEducationController@store).
func (s *Service) Create(ctx context.Context, userID int64, in Input) error {
	e, errs := build(in)
	if len(errs) > 0 {
		return errs
	}
	profileID, err := s.store.ProfileID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.store.Create(ctx, profileID, e); err != nil {
		return err
	}
	return s.reviewer.Submit(ctx, userID)
}

// Update validates, saves the owned row, then re-submits for review.
func (s *Service) Update(ctx context.Context, userID, id int64, in Input) error {
	e, errs := build(in)
	if len(errs) > 0 {
		return errs
	}
	cur, err := s.store.Get(ctx, userID, id)
	if err != nil {
		return err
	}
	e.ID = cur.ID
	if err := s.store.Update(ctx, id, e); err != nil {
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

// Not-found rows surface the store's pgx.ErrNoRows directly.
// build converts and validates the form input (reference education rules).
func build(in Input) (Education, FieldErrors) {
	errs := FieldErrors{}
	e := Education{
		Level:       in.Level,
		Institution: in.Institution,
		Subject:     in.Subject,
		IsCurrent:   in.IsCurrent,
	}
	if v := in.StudentID; v != "" {
		e.StudentID = &v
	}

	startYear, ok := year("start_year", in.StartYear, true, errs)
	if ok {
		e.StartYear = startYear
	}
	e.StartMonth = month("start_month", in.StartMonth, 1, 12, errs)

	if !in.IsCurrent {
		if in.EndYear != "" {
			if y, ok := year("end_year", in.EndYear, false, errs); ok {
				if y < e.StartYear {
					errs["end_year"] = "The end year must be greater than or equal to start year."
				} else {
					e.EndYear = &y
				}
			}
		}
		e.EndMonth = month("end_month", in.EndMonth, 1, 12, errs)
	}

	required("level", "level", in.Level, 255, errs)
	required("institution", "institution", in.Institution, 255, errs)
	required("subject", "subject", in.Subject, 255, errs)
	if in.StudentID != "" && len(in.StudentID) > 255 {
		errs["student_id"] = "The student id must not be greater than 255 characters."
	}

	if len(errs) > 0 {
		return Education{}, errs
	}
	return e, nil
}

func required(field, label, v string, max int, errs FieldErrors) {
	switch {
	case v == "":
		errs[field] = "The " + label + " field is required."
	case len(v) > max:
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

// month parses an optional month in [min, max].
func month(field, v string, min, max int, errs FieldErrors) *int16 {
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		errs[field] = "The " + field + " must be between " + strconv.Itoa(min) + " and " + strconv.Itoa(max) + "."
		return nil
	}
	m := int16(n)
	return &m
}

// firstMessage is unused-safe helper kept for FieldErrors.Error stability.
