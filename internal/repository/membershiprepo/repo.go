// Package membershiprepo is the persistence adapter for the membership
// service. The activation transaction spans the memberships and
// membership_payments tables plus the plan row, so this repo (which holds the
// pool) owns ActivateFromPayment and commits it atomically — the service stays
// transaction-free.
package membershiprepo

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/membership"
)

// Repo is the membership store. q backs single-query reads; pool opens the
// activation transaction.
type Repo struct {
	q    *generated.Queries
	pool *pgxpool.Pool
}

func NewRepo(q *generated.Queries, pool *pgxpool.Pool) *Repo {
	return &Repo{q: q, pool: pool}
}

func (r *Repo) List(ctx context.Context, status *string, limit, offset int64) ([]membership.ListItem, error) {
	rows, err := r.q.ListMemberships(ctx, generated.ListMembershipsParams{
		Status: status,
		Limit:  int32(limit),
		Offset: int32Ptr(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]membership.ListItem, 0, len(rows))
	for _, row := range rows {
		m := fromRow(generated.Membership{
			ID: row.ID, UserID: row.UserID, MembershipPlanID: row.MembershipPlanID,
			Status: row.Status, StartsAt: row.StartsAt, EndsAt: row.EndsAt,
			CancelledAt: row.CancelledAt, Notes: row.Notes, CreatedBy: row.CreatedBy,
		})
		out = append(out, membership.ListItem{
			Membership: m,
			UserName:   row.UserName,
			PlanName:   row.PlanName,
			Email:      row.UserEmail,
		})
	}
	return out, nil
}

func (r *Repo) Count(ctx context.Context, status *string) (int64, error) {
	return r.q.CountMemberships(ctx, status)
}

func (r *Repo) GetByID(ctx context.Context, id int64) (membership.Membership, error) {
	m, err := r.q.GetMembershipByID(ctx, id)
	if err != nil {
		return membership.Membership{}, err
	}
	return fromRow(m), nil
}

func (r *Repo) GetActiveForUser(ctx context.Context, userID int64) (membership.Membership, error) {
	m, err := r.q.GetActiveMembershipForUser(ctx, userID)
	if err != nil {
		return membership.Membership{}, err
	}
	return fromRow(m), nil
}

func (r *Repo) GetLatestForUser(ctx context.Context, userID int64) (membership.Membership, error) {
	m, err := r.q.GetLatestMembershipForUser(ctx, userID)
	if err != nil {
		return membership.Membership{}, err
	}
	return fromRow(m), nil
}

func (r *Repo) LockActiveForUser(ctx context.Context, userID int64) (membership.Membership, error) {
	m, err := r.q.LockActiveMembershipForUser(ctx, userID)
	if err != nil {
		return membership.Membership{}, err
	}
	return fromRow(m), nil
}

func (r *Repo) Create(ctx context.Context, m membership.Membership) (int64, error) {
	return r.q.CreateMembership(ctx, generated.CreateMembershipParams{
		UserID:           m.UserID,
		MembershipPlanID: m.MembershipPlanID,
		Status:           string(m.Status),
		StartsAt:         ts(m.StartsAt),
		EndsAt:           ts(m.EndsAt),
		CancelledAt:      ts(m.CancelledAt),
		Notes:            m.Notes,
		CreatedBy:        m.CreatedBy,
	})
}

func (r *Repo) UpdateTerm(ctx context.Context, id, planID int64, endsAt *time.Time) error {
	return r.q.UpdateMembershipTerm(ctx, generated.UpdateMembershipTermParams{
		MembershipPlanID: planID,
		EndsAt:           ts(endsAt),
		ID:               id,
	})
}

func (r *Repo) Update(ctx context.Context, id int64, endsAt *time.Time, notes *string) error {
	return r.q.UpdateMembership(ctx, generated.UpdateMembershipParams{
		EndsAt: ts(endsAt),
		Notes:  notes,
		ID:     id,
	})
}

func (r *Repo) Cancel(ctx context.Context, id int64) error {
	return r.q.CancelMembership(ctx, id)
}

func (r *Repo) ExpireLapsed(ctx context.Context) (int64, error) {
	return r.q.ExpireMemberships(ctx)
}

// fromRow maps a generated membership row to the domain type.
func fromRow(m generated.Membership) membership.Membership {
	return membership.Membership{
		ID:               m.ID,
		UserID:           m.UserID,
		MembershipPlanID: m.MembershipPlanID,
		Status:           membership.Status(m.Status),
		StartsAt:         tsPtr(m.StartsAt),
		EndsAt:           tsPtr(m.EndsAt),
		CancelledAt:      tsPtr(m.CancelledAt),
		Notes:            m.Notes,
		CreatedBy:        m.CreatedBy,
	}
}
