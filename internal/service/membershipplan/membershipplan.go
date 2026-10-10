// Package membershipplan ports reference App\Models\MembershipPlan and the
// MembershipPlanController (dashboard CRUD + drag-reorder). A plan holds
// exactly one term: a duration_days count, or lifetime (duration NULL).
package membershipplan

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Plan is the domain representation of a membership_plans row.
type Plan struct {
	ID           int64
	Name         string
	Description  *string
	Price        string // decimal(10,2), kept as the canonical "1500.00" string
	DurationDays *int32 // NULL when lifetime
	IsLifetime   bool
	Features     map[string]string // gated-feature toggles (e.g. members: "1")
	SortOrder    int32
	IsActive     bool
}

// ErrNotFound is a missing plan (findOrFail → 404).
var ErrNotFound = errors.New("membership plan not found")

// ErrInUse blocks deleting a plan that has memberships or payments
// (reference plan_in_use guard).
var ErrInUse = errors.New("membership plan is in use")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence port; *repository/membershipplan Repo satisfies it.
type Store interface {
	GetByID(ctx context.Context, id int64) (Plan, error)
	List(ctx context.Context) ([]Plan, error)
	ListActive(ctx context.Context) ([]Plan, error)
	MaxSortOrder(ctx context.Context) (int32, error)
	Create(ctx context.Context, p Plan) (int64, error)
	Update(ctx context.Context, p Plan) error
	Delete(ctx context.Context, id int64) error
	HasMemberships(ctx context.Context, id int64) (bool, error)
	HasPayments(ctx context.Context, id int64) (bool, error)
	Reorder(ctx context.Context, id int64, sort int32) error
}

// Service ports MembershipPlanController@store/update/destroy/reorder and the
// member-facing plans listing.
type Service struct {
	store Store
}

func New(store Store) *Service { return &Service{store: store} }

// Get returns one plan (404 via ErrNotFound).
func (s *Service) Get(ctx context.Context, id int64) (Plan, error) {
	p, err := s.store.GetByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	return p, err
}

// AdminList returns every plan ordered for the dashboard.
func (s *Service) AdminList(ctx context.Context) ([]Plan, error) {
	return s.store.List(ctx)
}

// ActiveList returns the member-facing active plans (reference
// MembershipPlan::active()).
func (s *Service) ActiveList(ctx context.Context) ([]Plan, error) {
	return s.store.ListActive(ctx)
}

// StoreInput is the validated store/update payload (reference
// MembershipPlanRequest@prepareForValidation, which normalizes the authored
// term into a single duration_days).
type StoreInput struct {
	Name         string
	Description  *string
	Price        string
	DurationDays *int32
	IsLifetime   bool
	Features     map[string]string
	SortOrder    *int32
	IsActive     *bool
}

// Store creates a plan. New plans append to the end (max sort_order + 1) when
// no explicit sort_order is given (reference MembershipPlanController@store).
func (s *Service) Store(ctx context.Context, in StoreInput) (int64, error) {
	if errs := validate(in); len(errs) > 0 {
		return 0, errs
	}
	return s.create(ctx, in, planFromInput(in))
}

func (s *Service) create(ctx context.Context, in StoreInput, p Plan) (int64, error) {
	if in.SortOrder == nil {
		max, err := s.store.MaxSortOrder(ctx)
		if err != nil {
			return 0, err
		}
		p.SortOrder = max + 1
	}
	return s.store.Create(ctx, p)
}

// Update edits a plan (reference MembershipPlanController@update).
func (s *Service) Update(ctx context.Context, id int64, in StoreInput) error {
	if errs := validate(in); len(errs) > 0 {
		return errs
	}
	if _, err := s.store.GetByID(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	p := planFromInput(in)
	p.ID = id
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	return s.store.Update(ctx, p)
}

// Destroy deletes a plan, refusing one in use (reference plan_in_use guard).
func (s *Service) Destroy(ctx context.Context, id int64) error {
	inUse, err := s.hasAnyUse(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrInUse
	}
	return s.store.Delete(ctx, id)
}

func (s *Service) hasAnyUse(ctx context.Context, id int64) (bool, error) {
	m, err := s.store.HasMemberships(ctx, id)
	if err != nil || m {
		return m, err
	}
	return s.store.HasPayments(ctx, id)
}

// Reorder persists a new position for one plan (reference
// MembershipPlanController@reorder).
func (s *Service) Reorder(ctx context.Context, id int64, sort int32) error {
	return s.store.Reorder(ctx, id, sort)
}

// planFromInput maps validated input to a stored plan.
func planFromInput(in StoreInput) Plan {
	p := Plan{
		Name:        in.Name,
		Description: in.Description,
		Price:       in.Price,
		IsLifetime:  in.IsLifetime,
		Features:    in.Features,
		IsActive:    true,
	}
	if in.DurationDays != nil {
		d := *in.DurationDays
		p.DurationDays = &d
	}
	if in.IsActive != nil {
		p.IsActive = *in.IsActive
	}
	if in.SortOrder != nil {
		p.SortOrder = *in.SortOrder
	}
	return p
}

// validate enforces the MembershipPlanRequest rules + the exclusive-term
// invariant (exactly one of duration / lifetime).
func validate(in StoreInput) FieldErrors {
	errs := FieldErrors{}
	if in.Name == "" {
		errs["name"] = "The name field is required."
	} else if len(in.Name) > 255 {
		errs["name"] = "The name must not be greater than 255 characters."
	}
	if in.Price == "" {
		errs["price"] = "The price field is required."
	}
	// Exclusive term: exactly one of a positive duration or lifetime.
	set := 0
	if in.DurationDays != nil && *in.DurationDays > 0 {
		set++
	}
	if in.IsLifetime {
		set++
	}
	if set != 1 {
		errs["term_days"] = "Exactly one term must be set: a duration in days, months, or lifetime."
	}
	return errs
}

// TermEndFrom returns the term end date for a membership starting at start; a
// lifetime term returns nil (no end). Reference
// MembershipPlan@termEndFrom — the single term calculator.
func (p Plan) TermEndFrom(start time.Time) *time.Time {
	if p.IsLifetime || p.DurationDays == nil {
		return nil
	}
	end := start.AddDate(0, 0, int(*p.DurationDays))
	return &end
}

// TermLabel renders a human term: "Lifetime", "12 months", or "7 days"
// (reference MembershipPlan@termLabel).
func (p Plan) TermLabel() string {
	if p.IsLifetime || p.DurationDays == nil {
		return "Lifetime"
	}
	days := int(*p.DurationDays)
	if days >= 60 && days%30 == 0 {
		months := days / 30
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	}
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// FeatureGranted reports whether this plan's features map enables a key with a
// truthy value ("1", "true", "yes", "on" — reference
// HasMemberships@hasMembershipFeature).
func (p Plan) FeatureGranted(key string) bool {
	v, ok := p.Features[key]
	if !ok || v == "" {
		return false
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
