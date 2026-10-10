// Package membershippayment ports reference App\Models\MembershipPayment, the
// SubmitMembershipPayment / RejectMembershipPayment actions, and the payment
// review flow. A member submits a pending payment (with an optional proof);
// staff approve (→ activation) or reject it.
package membershippayment

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/membership"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
)

// Status is the reference PaymentStatus enum value.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// Payment is the domain representation of a membership_payments row, with its
// plan name and (optionally) the user's name resolved for display.
type Payment struct {
	ID               int64
	UserID           int64
	MembershipPlanID int64
	MembershipID     *int64
	Amount           string // decimal(10,2) as the canonical "1500.00" string
	Method           string
	Reference        *string
	PaidAt           time.Time // a date
	ProofPath        *string
	Notes            *string
	ReviewNotes      *string
	Status           Status
	ReviewedBy       *int64
	ReviewedAt       *time.Time
	CreatedBy        *int64
	PlanName         string
	UserName         string
	UserEmail        string
}

// IsPending reports whether the payment awaits review.
func (p Payment) IsPending() bool { return p.Status == StatusPending }

// ErrNotFound is a missing payment (findOrFail → 404).
var ErrNotFound = errors.New("membership payment not found")

// ErrNotOwner is the reference abort_unless(payment->user_id === user) 403 on
// the member-facing payment pages.
var ErrNotOwner = errors.New("not your payment")

// ErrNotPending is the reference payment_not_pending guard on approve/reject.
var ErrNotPending = errors.New("payment is not pending")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence port; *repository/membershippayment Repo satisfies
// it. Activation is delegated to the membership Store's transaction.
type Store interface {
	GetByID(ctx context.Context, id int64) (Payment, error)
	GetPendingForUser(ctx context.Context, userID int64) (Payment, error)
	ListForUser(ctx context.Context, userID int64, limit, offset int64) ([]Payment, error)
	CountForUser(ctx context.Context, userID int64) (int64, error)
	List(ctx context.Context, status *string, limit, offset int64) ([]Payment, error)
	Count(ctx context.Context, status *string) (int64, error)
	// ListForMembership returns a membership's payments newest-first
	// (reference MembershipController@show's $membership->payments).
	ListForMembership(ctx context.Context, membershipID int64) ([]Payment, error)
	Create(ctx context.Context, p Payment) (int64, error)
	// Reject marks a pending payment rejected with review notes (reference
	// RejectMembershipPayment).
	Reject(ctx context.Context, id int64, reviewNotes string, reviewerID *int64) error
}

// Activator runs the single activation path; *membership.Service satisfies it
// via Activate.
type Activator interface {
	Activate(ctx context.Context, req membership.ActivateRequest) (membership.ActivateResult, error)
}

// ActivityLogger is the spatie activitylog port (service/activitylog).
type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

// Service ports the payment submission, review and listing flows.
type Service struct {
	store   Store
	members *membership.Service
	plans   PlanReader
	methods ActiveMethodTypes
	logger  ActivityLogger
}

// ActiveMethodTypes lists the payment-method type strings a payment may
// reference; *membershippaymentmethod.Service satisfies it via ActiveTypes.
type ActiveMethodTypes interface {
	ActiveTypes(ctx context.Context) ([]string, error)
}

// PlanReader loads a plan (for the "plan must be active" + "amount must match
// price" rules); *membershipplan.Service satisfies it via Get.
type PlanReader interface {
	Get(ctx context.Context, id int64) (membershipplan.Plan, error)
}

func New(store Store, members *membership.Service, plans PlanReader, methods ActiveMethodTypes, logger ActivityLogger) *Service {
	return &Service{store: store, members: members, plans: plans, methods: methods, logger: logger}
}

// SubmitInput is the validated submission payload (reference
// SubmitMembershipPayment / RecordMembershipPaymentRequest).
type SubmitInput struct {
	UserID    int64 // the target member (staff may record for another user)
	PlanID    int64
	Amount    string
	Method    string
	Reference *string
	PaidAt    time.Time
	Notes     *string
	ProofPath *string
	Activate  bool // staff-recorded payments may activate immediately
	CreatedBy int64
}

// Submit records a pending payment (reference SubmitMembershipPayment). When
// activate is set and the actor is staff, it immediately runs activation.
// Returns the payment id and, when activated, the membership id.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (int64, *int64, error) {
	if errs := s.validate(ctx, in); len(errs) > 0 {
		return 0, nil, errs
	}
	id, err := s.store.Create(ctx, Payment{
		UserID:           in.UserID,
		MembershipPlanID: in.PlanID,
		Amount:           in.Amount,
		Method:           in.Method,
		Reference:        in.Reference,
		PaidAt:           in.PaidAt,
		ProofPath:        in.ProofPath,
		Notes:            in.Notes,
		Status:           StatusPending,
		CreatedBy:        &in.CreatedBy,
	})
	if err != nil {
		return 0, nil, err
	}
	if !in.Activate {
		return id, nil, nil
	}
	reviewer := in.CreatedBy
	res, err := s.members.Activate(ctx, membership.ActivateRequest{
		PaymentID:  id,
		UserID:     in.UserID,
		PlanID:     in.PlanID,
		PaidAt:     &in.PaidAt,
		ReviewerID: &reviewer,
	})
	if err != nil {
		return id, nil, err
	}
	return id, &res.MembershipID, nil
}

// Approve is the reference ApprovePayment: staff approves a pending payment by
// running it through the single activation path.
func (s *Service) Approve(ctx context.Context, paymentID int64, reviewerID int64) (int64, error) {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if p.Status != StatusPending {
		return 0, ErrNotPending
	}
	res, err := s.members.Activate(ctx, membership.ActivateRequest{
		PaymentID:  paymentID,
		UserID:     p.UserID,
		PlanID:     p.MembershipPlanID,
		PaidAt:     &p.PaidAt,
		ReviewerID: &reviewerID,
	})
	if err != nil {
		return 0, err
	}
	return res.MembershipID, nil
}

// Reject is the reference RejectMembershipPayment: a pending payment is marked
// rejected with a required reason and logged.
func (s *Service) Reject(ctx context.Context, paymentID, reviewerID int64, reviewNotes string) error {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if p.Status != StatusPending {
		return ErrNotPending
	}
	if reviewNotes == "" {
		return FieldErrors{"review_notes": "The review notes field is required."}
	}
	if err := s.store.Reject(ctx, paymentID, reviewNotes, &reviewerID); err != nil {
		return err
	}
	return s.logger.Log(ctx, activitylog.Entry{
		LogName:     "memberships",
		Event:       "payment_rejected",
		Description: "membership payment rejected",
		Subject:     &activitylog.Subject{Kind: "membership_payment", ID: paymentID},
		Properties:  map[string]any{"review_notes": reviewNotes},
		Causer:      &activitylog.Causer{ID: reviewerID},
	})
}

// GetOwned returns a payment the given user owns (member-facing pages);
// PendingForUser returns the member's latest pending payment (reference
// MyMembershipController@show's $pendingPayment); ErrNotFound when none.
func (s *Service) PendingForUser(ctx context.Context, userID int64) (Payment, error) {
	p, err := s.store.GetPendingForUser(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrNotFound
		}
		return Payment{}, err
	}
	return p, nil
}

// GetOwned returns a payment the given user owns (member-facing pages);
// ErrNotOwner when it belongs to someone else.
func (s *Service) GetOwned(ctx context.Context, userID, paymentID int64) (Payment, error) {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrNotFound
		}
		return Payment{}, err
	}
	if p.UserID != userID {
		return Payment{}, ErrNotOwner
	}
	return p, nil
}

// Get returns any payment (admin pages).
func (s *Service) Get(ctx context.Context, paymentID int64) (Payment, error) {
	p, err := s.store.GetByID(ctx, paymentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Payment{}, ErrNotFound
		}
		return Payment{}, err
	}
	return p, nil
}

// Page is one page of payments plus the total (admin/member listings).
type Page struct {
	Payments []Payment
	Total    int64
}

// ListForUser returns the member's newest-first payments page.
func (s *Service) ListForUser(ctx context.Context, userID int64, page int64) (Page, error) {
	if page < 1 {
		page = 1
	}
	const perPage = 10
	items, err := s.store.ListForUser(ctx, userID, perPage, (page-1)*perPage)
	if err != nil {
		return Page{}, err
	}
	total, err := s.store.CountForUser(ctx, userID)
	if err != nil {
		return Page{}, err
	}
	return Page{Payments: items, Total: total}, nil
}

// List returns the admin payments page filtered by status ("" = all).
func (s *Service) List(ctx context.Context, status string, page int64) (Page, error) {
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
	return Page{Payments: items, Total: total}, nil
}

// ListForMembership returns a membership's payments newest-first (reference
// MembershipController@show's $membership->payments).
func (s *Service) ListForMembership(ctx context.Context, membershipID int64) ([]Payment, error) {
	return s.store.ListForMembership(ctx, membershipID)
}

// validate enforces the Submit/RecordMembershipPaymentRequest rules: the plan
// must exist and be active, the method an active type, the paid date not in
// the future, and the amount must match the plan price.
func (s *Service) validate(ctx context.Context, in SubmitInput) FieldErrors {
	errs := FieldErrors{}
	plan, err := s.plans.Get(ctx, in.PlanID)
	if err != nil {
		errs["membership_plan_id"] = "The selected plan is invalid."
		return errs
	}
	if !plan.IsActive {
		errs["membership_plan_id"] = "The selected plan is inactive."
	}
	types, err := s.methods.ActiveTypes(ctx)
	if err == nil && !contains(types, in.Method) {
		errs["method"] = "The selected method is invalid."
	}
	if in.PaidAt.After(endOfToday()) {
		errs["paid_at"] = "The paid date must be a date before or equal to today."
	}
	if plan.Price != normalizeAmount(in.Amount) {
		errs["amount"] = "The amount must match the plan price of " + plan.Price + "."
	}
	return errs
}

// normalizeAmount renders an amount to the two-decimal canonical form the plan
// price uses (reference number_format((float)$amount, 2, '.', ”)).
func normalizeAmount(s string) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return strconv.FormatFloat(f, 'f', 2, 64)
}

// endOfToday is the inclusive "today" bound for the paid_at rule.
func endOfToday() time.Time {
	now := time.Now()
	y, mo, d := now.Date()
	return time.Date(y, mo, d, 23, 59, 59, 0, now.Location())
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
