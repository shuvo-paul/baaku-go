// Package methodrepo is the persistence adapter for the
// membershippaymentmethod service (reference
// App\Models\MembershipPaymentMethod + MembershipPaymentMethodController).
package methodrepo

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
)

// Repo is the membership-payment-method store; *Repo satisfies
// membershippaymentmethod.Store.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) GetByID(ctx context.Context, id int64) (membershippaymentmethod.Method, error) {
	m, err := r.q.GetMembershipPaymentMethodByID(ctx, id)
	if err != nil {
		return membershippaymentmethod.Method{}, err
	}
	return fromRow(m), nil
}

func (r *Repo) List(ctx context.Context) ([]membershippaymentmethod.Method, error) {
	rows, err := r.q.ListMembershipPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	return fromRows(rows), nil
}

func (r *Repo) ListActive(ctx context.Context) ([]membershippaymentmethod.Method, error) {
	rows, err := r.q.ListActiveMembershipPaymentMethods(ctx)
	if err != nil {
		return nil, err
	}
	return fromRows(rows), nil
}

func (r *Repo) MaxSortOrder(ctx context.Context) (int32, error) {
	v, err := r.q.MaxMembershipPaymentMethodSortOrder(ctx)
	if err != nil {
		return 0, err
	}
	return toInt32(v), nil
}

func (r *Repo) Create(ctx context.Context, m membershippaymentmethod.Method) (int64, error) {
	return r.q.CreateMembershipPaymentMethod(ctx, generated.CreateMembershipPaymentMethodParams{
		Type:         string(m.Type),
		Instructions: m.Instructions,
		IsActive:     m.IsActive,
		SortOrder:    m.SortOrder,
	})
}

func (r *Repo) Update(ctx context.Context, m membershippaymentmethod.Method) error {
	return r.q.UpdateMembershipPaymentMethod(ctx, generated.UpdateMembershipPaymentMethodParams{
		Type:         string(m.Type),
		Instructions: m.Instructions,
		IsActive:     m.IsActive,
		ID:           m.ID,
	})
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteMembershipPaymentMethod(ctx, id)
}

func (r *Repo) Reorder(ctx context.Context, id int64, sort int32) error {
	return r.q.ReorderMembershipPaymentMethod(ctx, generated.ReorderMembershipPaymentMethodParams{
		SortOrder: sort,
		ID:        id,
	})
}

func (r *Repo) TypeExists(ctx context.Context, typ string, ignoreID int64) (bool, error) {
	return r.q.MembershipPaymentMethodTypeExists(ctx, generated.MembershipPaymentMethodTypeExistsParams{
		Type:     typ,
		IgnoreID: ignoreID,
	})
}

// fromRow maps a generated payment-method row to the domain type.
func fromRow(m generated.MembershipPaymentMethod) membershippaymentmethod.Method {
	return membershippaymentmethod.Method{
		ID:           m.ID,
		Type:         membershippaymentmethod.MethodType(m.Type),
		Instructions: m.Instructions,
		IsActive:     m.IsActive,
		SortOrder:    m.SortOrder,
	}
}

func fromRows(rows []generated.MembershipPaymentMethod) []membershippaymentmethod.Method {
	out := make([]membershippaymentmethod.Method, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out
}

// toInt32 coerces the COALESCE(MAX(sort_order), -1) interface{} payload (pgx
// returns int64 for max) to int32.
func toInt32(v interface{}) int32 {
	switch t := v.(type) {
	case int64:
		return int32(t)
	case int32:
		return t
	case int:
		return int32(t)
	default:
		return 0
	}
}
