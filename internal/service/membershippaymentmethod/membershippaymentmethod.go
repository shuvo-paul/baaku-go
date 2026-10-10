// Package membershippaymentmethod ports reference
// App\Models\MembershipPaymentMethod and the
// MembershipPaymentMethodController (dashboard CRUD + drag-reorder). A method
// is keyed by a unique `type` that doubles as its display label, so a method's
// identity never drifts from its payment history.
package membershippaymentmethod

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// MethodType is the reference MembershipMethodType enum value.
type MethodType string

const (
	Bkash        MethodType = "bkash"
	Nagad        MethodType = "nagad"
	BankTransfer MethodType = "bank_transfer"
)

// Label maps a type to its human label (reference MembershipMethodType@label).
func (t MethodType) Label() string {
	switch t {
	case Bkash:
		return "bKash"
	case Nagad:
		return "Nagad"
	case BankTransfer:
		return "Bank Transfer"
	}
	return string(t)
}

// AllMethodTypes returns every known type for authoring forms (reference
// MembershipMethodType::options()).
func AllMethodTypes() []MethodType {
	return []MethodType{Bkash, Nagad, BankTransfer}
}

// ValidMethodType reports whether s is a known type (the store/update
// required-in rule).
func ValidMethodType(s string) bool {
	for _, t := range AllMethodTypes() {
		if string(t) == s {
			return true
		}
	}
	return false
}

// Method is the domain representation of a membership_payment_methods row.
type Method struct {
	ID           int64
	Type         MethodType
	Instructions *string
	IsActive     bool
	SortOrder    int32
}

// InstructionsHTML renders a stored method's instructions to HTML. The
// instructions are Editor.js JSON (reference RendersEditorContent trait); a
// plain-text value is escaped and shown as-is.
func (m Method) InstructionsHTML() string {
	return RenderEditorHTML(m.Instructions)
}

// ErrNotFound is a missing method (findOrFail → 404).
var ErrNotFound = errors.New("payment method not found")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence port; *repository/membershippaymentmethod Repo
// satisfies it.
type Store interface {
	GetByID(ctx context.Context, id int64) (Method, error)
	List(ctx context.Context) ([]Method, error)
	ListActive(ctx context.Context) ([]Method, error)
	MaxSortOrder(ctx context.Context) (int32, error)
	Create(ctx context.Context, m Method) (int64, error)
	Update(ctx context.Context, m Method) error
	Delete(ctx context.Context, id int64) error
	Reorder(ctx context.Context, id int64, sort int32) error
	TypeExists(ctx context.Context, typ string, ignoreID int64) (bool, error)
}

// Service ports MembershipPaymentMethodController@store/update/destroy/reorder
// and the member-facing active-methods listing.
type Service struct {
	store Store
}

func New(store Store) *Service { return &Service{store: store} }

// Get returns one method (404 via ErrNotFound).
func (s *Service) Get(ctx context.Context, id int64) (Method, error) {
	m, err := s.store.GetByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Method{}, ErrNotFound
	}
	return m, err
}

// AdminList returns every method ordered for the dashboard.
func (s *Service) AdminList(ctx context.Context) ([]Method, error) {
	return s.store.List(ctx)
}

// ActiveList returns the member-facing active methods (reference
// MembershipPaymentMethod::active()).
func (s *Service) ActiveList(ctx context.Context) ([]Method, error) {
	return s.store.ListActive(ctx)
}

// ActiveTypes returns the type strings that a payment may reference (the
// store-payment "method must be an active type" rule).
func (s *Service) ActiveTypes(ctx context.Context) ([]string, error) {
	ms, err := s.store.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, string(m.Type))
	}
	return out, nil
}

// StoreInput is the validated store/update payload.
type StoreInput struct {
	Type         string
	Instructions *string
	IsActive     *bool
}

// Store creates a method, appending it to the end (max sort_order + 1;
// reference MembershipPaymentMethodController@store).
func (s *Service) Store(ctx context.Context, in StoreInput) (int64, error) {
	if errs := s.validate(ctx, in, 0); len(errs) > 0 {
		return 0, errs
	}
	m := methodFromInput(in)
	max, err := s.store.MaxSortOrder(ctx)
	if err != nil {
		return 0, err
	}
	m.SortOrder = max + 1
	return s.store.Create(ctx, m)
}

// Update edits a method (reference MembershipPaymentMethodController@update).
func (s *Service) Update(ctx context.Context, id int64, in StoreInput) error {
	if errs := s.validate(ctx, in, id); len(errs) > 0 {
		return errs
	}
	existing, err := s.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	m := methodFromInput(in)
	m.ID = id
	m.SortOrder = existing.SortOrder
	return s.store.Update(ctx, m)
}

// Destroy deletes a method (reference MembershipPaymentMethodController@
// destroy).
func (s *Service) Destroy(ctx context.Context, id int64) error {
	return s.store.Delete(ctx, id)
}

// Reorder persists a new position for one method (reference
// MembershipPaymentMethodController@reorder).
func (s *Service) Reorder(ctx context.Context, id int64, sort int32) error {
	return s.store.Reorder(ctx, id, sort)
}

// methodFromInput maps validated input to a stored method.
func methodFromInput(in StoreInput) Method {
	m := Method{Type: MethodType(in.Type), Instructions: in.Instructions, IsActive: true}
	if in.IsActive != nil {
		m.IsActive = *in.IsActive
	}
	return m
}

// validate enforces the Store/UpdateMembershipPaymentMethodRequest rules.
// validate enforces the Store/UpdateMembershipPaymentMethodRequest rules,
// including the unique type rule (ignoreID is the row being edited).
func (s *Service) validate(ctx context.Context, in StoreInput, ignoreID int64) FieldErrors {
	errs := FieldErrors{}
	switch {
	case in.Type == "":
		errs["type"] = "The type field is required."
	case !ValidMethodType(in.Type):
		errs["type"] = "The selected type is invalid."
	default:
		if exists, err := s.store.TypeExists(ctx, in.Type, ignoreID); err == nil && exists {
			errs["type"] = "The type has already been taken."
		}
	}
	if in.Instructions != nil && len(*in.Instructions) > 5000 {
		errs["instructions"] = "The instructions must not be greater than 5000 characters."
	}
	return errs
}
