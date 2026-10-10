// Package planrepo is the persistence adapter for the membershipplan service
// (reference App\Models\MembershipPlan + MembershipPlanController).
package planrepo

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/memberconv"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
)

// Repo is the membership-plan store; *Repo satisfies membershipplan.Store.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) GetByID(ctx context.Context, id int64) (membershipplan.Plan, error) {
	p, err := r.q.GetMembershipPlanByID(ctx, id)
	if err != nil {
		return membershipplan.Plan{}, err
	}
	return fromRow(p), nil
}

func (r *Repo) List(ctx context.Context) ([]membershipplan.Plan, error) {
	rows, err := r.q.ListMembershipPlans(ctx)
	if err != nil {
		return nil, err
	}
	return fromRows(rows), nil
}

func (r *Repo) ListActive(ctx context.Context) ([]membershipplan.Plan, error) {
	rows, err := r.q.ListActiveMembershipPlans(ctx)
	if err != nil {
		return nil, err
	}
	return fromRows(rows), nil
}

func (r *Repo) MaxSortOrder(ctx context.Context) (int32, error) {
	v, err := r.q.MaxMembershipPlanSortOrder(ctx)
	if err != nil {
		return 0, err
	}
	return toInt32(v), nil
}

func (r *Repo) Create(ctx context.Context, p membershipplan.Plan) (int64, error) {
	return r.q.CreateMembershipPlan(ctx, generated.CreateMembershipPlanParams{
		Name:         p.Name,
		Description:  p.Description,
		Price:        memberconv.NumValue(p.Price),
		DurationDays: durationDays(p.DurationDays),
		IsLifetime:   p.IsLifetime,
		Features:     memberconv.FeaturesJSON(p.Features),
		SortOrder:    p.SortOrder,
		IsActive:     p.IsActive,
	})
}

func (r *Repo) Update(ctx context.Context, p membershipplan.Plan) error {
	return r.q.UpdateMembershipPlan(ctx, generated.UpdateMembershipPlanParams{
		Name:         p.Name,
		Description:  p.Description,
		Price:        memberconv.NumValue(p.Price),
		DurationDays: durationDays(p.DurationDays),
		IsLifetime:   p.IsLifetime,
		Features:     memberconv.FeaturesJSON(p.Features),
		SortOrder:    p.SortOrder,
		IsActive:     p.IsActive,
		ID:           p.ID,
	})
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	return r.q.DeleteMembershipPlan(ctx, id)
}

func (r *Repo) HasMemberships(ctx context.Context, id int64) (bool, error) {
	return r.q.MembershipPlanHasMemberships(ctx, id)
}

func (r *Repo) HasPayments(ctx context.Context, id int64) (bool, error) {
	return r.q.MembershipPlanHasPayments(ctx, id)
}

func (r *Repo) Reorder(ctx context.Context, id int64, sort int32) error {
	return r.q.ReorderMembershipPlan(ctx, generated.ReorderMembershipPlanParams{
		SortOrder: sort,
		ID:        id,
	})
}

// fromRow maps a generated plan row to the domain type.
func fromRow(p generated.MembershipPlan) membershipplan.Plan {
	return membershipplan.Plan{
		ID:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		Price:        memberconv.NumString(p.Price),
		DurationDays: days(p.DurationDays),
		IsLifetime:   p.IsLifetime,
		Features:     memberconv.FeaturesMap(p.Features),
		SortOrder:    p.SortOrder,
		IsActive:     p.IsActive,
	}
}

func fromRows(rows []generated.MembershipPlan) []membershipplan.Plan {
	out := make([]membershipplan.Plan, 0, len(rows))
	for _, r := range rows {
		out = append(out, fromRow(r))
	}
	return out
}

// durationDays maps the domain *int32 to the generated *int16 (NULL when
// lifetime).
func durationDays(d *int32) *int16 {
	if d == nil {
		return nil
	}
	v := int16(*d)
	return &v
}

// days maps the generated *int16 to the domain *int32.
func days(d *int16) *int32 {
	if d == nil {
		return nil
	}
	v := int32(*d)
	return &v
}

// toInt32 coerces the COALESCE(MAX(sort_order), -1) interface{} payload (pgx
// returns int64 for count/max) to int32.
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
