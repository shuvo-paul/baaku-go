// Package membership ports reference App\Models\Membership, the HasMemberships
// trait, and the membership actions (ActivateMembership, CancelMembership,
// UpdateMembership). A membership is the user's current term on a plan; it is
// "active" while its status is active and either its plan is lifetime or its
// end date is still in the future.
package membership

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
)

// Status is the reference MembershipStatus enum value.
type Status string

const (
	StatusActive    Status = "active"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

// Transitions returns the states this status may move to (reference
// MembershipStatus@transitions).
func (s Status) Transitions() []Status {
	switch s {
	case StatusActive:
		return []Status{StatusExpired, StatusCancelled}
	default:
		return nil
	}
}

// Membership is the domain representation of a memberships row, with its plan
// resolved for term/feature calculations.
type Membership struct {
	ID               int64
	UserID           int64
	MembershipPlanID int64
	Status           Status
	StartsAt         *time.Time
	EndsAt           *time.Time
	CancelledAt      *time.Time
	Notes            *string
	CreatedBy        *int64
	Plan             membershipplan.Plan
}

// ListItem is one membership row on the admin index (reference
// memberships/index.blade.php), with the member and plan names resolved.
type ListItem struct {
	Membership
	UserName string
	PlanName string
	Email    string
}

// ErrNotFound is a missing membership (findOrFail → 404).
var ErrNotFound = errors.New("membership not found")

// ErrNotActive is the reference membership_not_active guard on cancel.
var ErrNotActive = errors.New("membership is not active")

// ErrAlreadyActivated is the reference already_activated guard (a payment
// already linked to a membership is never applied twice).
var ErrAlreadyActivated = errors.New("payment already activated")

// ErrPaymentNotActivatable is the reference payment_not_activatable guard.
var ErrPaymentNotActivatable = errors.New("payment cannot be activated")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence port; *repository/membership Repo satisfies it. The
// activation transaction spans the memberships and membership_payments tables,
// so the repo owns ActivateFromPayment and commits it atomically — the service
// stays transaction-free (repository layer owns all SQL + transactions).
type Store interface {
	GetByID(ctx context.Context, id int64) (Membership, error)
	// List returns one admin page of memberships (optionally filtered by
	// status) with the member and plan names resolved.
	List(ctx context.Context, status *string, limit, offset int64) ([]ListItem, error)
	Count(ctx context.Context, status *string) (int64, error)
	GetActiveForUser(ctx context.Context, userID int64) (Membership, error)
	GetLatestForUser(ctx context.Context, userID int64) (Membership, error)
	// LockActiveForUser returns the user's active membership with a row lock
	// (reference ActivateMembership's lockForUpdate lookup); pgx.ErrNoRows
	// when there is none.
	LockActiveForUser(ctx context.Context, userID int64) (Membership, error)
	Create(ctx context.Context, m Membership) (int64, error)
	// UpdateTerm re-plans and pushes the end date (activation extend path).
	UpdateTerm(ctx context.Context, id, planID int64, endsAt *time.Time) error
	Update(ctx context.Context, id int64, endsAt *time.Time, notes *string) error
	Cancel(ctx context.Context, id int64) error
	// ExpireLapsed flips active, non-lifetime, lapsed rows (the scheduled
	// ExpireMemberships sweep).
	ExpireLapsed(ctx context.Context) (int64, error)
	// ActivateFromPayment runs the single activation path in one transaction:
	// create or extend the user's membership, then approve+link the payment.
	// It returns the membership id and whether a new membership was created
	// (so the service can send the right email). Implements the reference
	// ActivateMembership.
	ActivateFromPayment(ctx context.Context, req ActivateRequest) (ActivateResult, error)
}

// ActivateRequest is the input to the activation transaction.
type ActivateRequest struct {
	PaymentID  int64
	UserID     int64
	PlanID     int64
	PaidAt     *time.Time // payment.paid_at (a date)
	ReviewerID *int64     // the acting staff member
	// Term is the plan's term calculator (resolved by the service above this
	// repo) used to derive the membership's end date.
	Term PlanTerm
}

// PlanTerm is the slice of a plan the activation repo needs to derive a
// membership's end date.
type PlanTerm struct {
	IsLifetime   bool
	DurationDays int32
}

// EndFrom returns the term end date for a membership starting at start; a
// lifetime term returns nil (no end). Mirrors
// MembershipPlan@termEndFrom — the single term calculator.
func (t PlanTerm) EndFrom(start time.Time) *time.Time {
	if t.IsLifetime {
		return nil
	}
	end := start.AddDate(0, 0, int(t.DurationDays))
	return &end
}

// ActivateResult reports the outcome of an activation.
type ActivateResult struct {
	MembershipID int64
	IsNew        bool
	EndsAt       *time.Time
}

// PlanReader loads a plan by id for activation; *membershipplan.Service
// satisfies it via its Get method.
type PlanReader interface {
	Get(ctx context.Context, id int64) (membershipplan.Plan, error)
}

// ActivityLogger is the spatie activitylog port (service/activitylog).
type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

// Service ports the membership actions and lookups.
type Service struct {
	store  Store
	plans  PlanReader
	logger ActivityLogger
}

func New(store Store, plans PlanReader, logger ActivityLogger) *Service {
	return &Service{store: store, plans: plans, logger: logger}
}

// Get returns one membership with its plan resolved (404 via ErrNotFound).
func (s *Service) Get(ctx context.Context, id int64) (Membership, error) {
	m, err := s.store.GetByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrNotFound
	}
	if err != nil {
		return Membership{}, err
	}
	return s.withPlan(ctx, m)
}

// ActiveForUser returns the user's active membership (reference
// $user->activeMembership); ErrNotFound when none.
func (s *Service) ActiveForUser(ctx context.Context, userID int64) (Membership, error) {
	m, err := s.store.GetActiveForUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrNotFound
	}
	if err != nil {
		return Membership{}, err
	}
	return s.withPlan(ctx, m)
}

// LatestForUser returns the user's most recent membership (reference
// $user->latestMembership); ErrNotFound when none.
func (s *Service) LatestForUser(ctx context.Context, userID int64) (Membership, error) {
	m, err := s.store.GetLatestForUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, ErrNotFound
	}
	if err != nil {
		return Membership{}, err
	}
	return s.withPlan(ctx, m)
}

// HasActive reports whether the user holds an active, unexpired membership
// (reference HasMemberships@hasActiveMembership — the feature gate's check).
func (s *Service) HasActive(ctx context.Context, userID int64) (bool, error) {
	_, err := s.store.GetActiveForUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// Page is one admin page of memberships plus the total.
type Page struct {
	Items []ListItem
	Total int64
}

// AdminList returns one page of memberships filtered by status ("" = all),
// newest-first (reference MembershipController@index).
func (s *Service) AdminList(ctx context.Context, status string, page int64) (Page, error) {
	if page < 1 {
		page = 1
	}
	var st *string
	if status != "" && status != "all" {
		st = &status
	}
	const perPage = 20
	items, err := s.store.List(ctx, st, perPage, (page-1)*perPage)
	if err != nil {
		return Page{}, err
	}
	total, err := s.store.Count(ctx, st)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: items, Total: total}, nil
}

// GrantedFeature reports whether the user's active membership plan grants a
// gated feature key (reference HasMemberships@membershipFeature +
// hasMembershipFeature). No active membership → false.
func (s *Service) GrantedFeature(ctx context.Context, userID int64, key string) (bool, error) {
	m, err := s.ActiveForUser(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return m.Plan.FeatureGranted(key), nil
}

// withPlan resolves the membership's plan for term/feature calculations.
func (s *Service) withPlan(ctx context.Context, m Membership) (Membership, error) {
	p, err := s.plan(ctx, m.MembershipPlanID)
	if err != nil {
		return Membership{}, err
	}
	m.Plan = p
	return m, nil
}

// plan loads a plan by id; bound to a PlanReader in NewPlanReader-backed
// wiring. Held as a field so tests can stub it.
func (s *Service) plan(ctx context.Context, id int64) (membershipplan.Plan, error) {
	return s.plans.Get(ctx, id)
}

// IsActive reports whether the membership currently counts as active: status
// active and either lifetime or an end date in the future (reference
// Membership@isActive).
func (m Membership) IsActive() bool {
	if m.Status != StatusActive {
		return false
	}
	if m.Plan.IsLifetime {
		return true
	}
	return m.EndsAt != nil && m.EndsAt.After(time.Now())
}

// EffectiveStatus derives the status on read so a lapsed row never reads as
// active before the scheduled sweep runs (reference
// Membership@effectiveStatus).
func (m Membership) EffectiveStatus() Status {
	if m.Status == StatusActive && !m.Plan.IsLifetime &&
		m.EndsAt != nil && !m.EndsAt.After(time.Now()) {
		return StatusExpired
	}
	return m.Status
}

// Cancel is the reference CancelMembership action: it refuses a non-active
// membership, otherwise flips it to cancelled and logs the
// membership_cancelled event.
func (s *Service) Cancel(ctx context.Context, id int64, actorID *int64) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if m.Status != StatusActive {
		return ErrNotActive
	}
	if err := s.store.Cancel(ctx, id); err != nil {
		return err
	}
	return s.logger.Log(ctx, activitylog.Entry{
		LogName:     "memberships",
		Event:       "membership_cancelled",
		Description: "membership cancelled",
		Subject:     &activitylog.Subject{Kind: "membership", ID: id},
		Properties:  map[string]any{"cancelled_by": actorID},
		Causer:      causer(actorID),
	})
}

// Update is the reference UpdateMembership action: staff adjusts the end date
// and notes only, validating ends_at > starts_at.
func (s *Service) Update(ctx context.Context, id int64, endsAt *time.Time, notes *string, actorID *int64) error {
	m, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if endsAt != nil && m.StartsAt != nil && !endsAt.After(*m.StartsAt) {
		return FieldErrors{"ends_at": "The ends date must be after the start date."}
	}
	if err := s.store.Update(ctx, id, endsAt, notes); err != nil {
		return err
	}
	var endsISO any
	if endsAt != nil {
		endsISO = endsAt.Format(time.RFC3339)
	}
	return s.logger.Log(ctx, activitylog.Entry{
		LogName:     "memberships",
		Event:       "membership_updated",
		Description: "membership updated",
		Subject:     &activitylog.Subject{Kind: "membership", ID: id},
		Properties:  map[string]any{"ends_at": endsISO, "updated_by": actorID},
		Causer:      causer(actorID),
	})
}

// Activate runs the single activation path (reference ActivateMembership): the
// repo creates or extends the membership and approves+links the payment in one
// transaction. Guards already-activated and non-activatable payments first.
func (s *Service) Activate(ctx context.Context, req ActivateRequest) (ActivateResult, error) {
	// Idempotency + activatable guards (reference already_activated /
	// payment_not_activatable) are enforced inside the transaction's locked
	// payment read; the repo maps them to ErrAlreadyActivated /
	// ErrPaymentNotActivatable.
	// Resolve the plan's term calculator before opening the transaction; the
	// repo stays a dumb writer and derives the end date from this term.
	plan, err := s.plans.Get(ctx, req.PlanID)
	if err != nil {
		return ActivateResult{}, err
	}
	req.Term = planTerm(plan)
	res, err := s.store.ActivateFromPayment(ctx, req)
	if err != nil {
		return ActivateResult{}, err
	}
	if err := s.logger.Log(ctx, activitylog.Entry{
		LogName:     "memberships",
		Event:       "payment_approved",
		Description: "membership payment approved",
		Subject:     &activitylog.Subject{Kind: "membership_payment", ID: req.PaymentID},
		Properties:  map[string]any{"membership_id": res.MembershipID},
		Causer:      causer(req.ReviewerID),
	}); err != nil {
		return ActivateResult{}, err
	}
	return res, nil
}

// ExpireLapsed runs the scheduled sweep (reference ExpireMembershipsCommand),
// flipping active, non-lifetime, lapsed rows to expired.
func (s *Service) ExpireLapsed(ctx context.Context) (int64, error) {
	return s.store.ExpireLapsed(ctx)
}

// causer maps an acting staff id to an activity-log causer (nil when absent).
// planTerm maps a plan to the activation repo's term calculator.
func planTerm(p membershipplan.Plan) PlanTerm {
	t := PlanTerm{IsLifetime: p.IsLifetime}
	if p.DurationDays != nil {
		t.DurationDays = *p.DurationDays
	}
	return t
}

// causer maps an acting staff id to an activity-log causer (nil when absent).
func causer(id *int64) *activitylog.Causer {
	if id == nil || *id == 0 {
		return nil
	}
	return &activitylog.Causer{ID: *id}
}
