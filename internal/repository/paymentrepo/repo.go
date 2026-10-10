// Package paymentrepo is the persistence adapter for the membershippayment
// service (reference App\Models\MembershipPayment + the payment review flow).
package paymentrepo

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/memberconv"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
)

// Repo is the membership-payment store; *Repo satisfies
// membershippayment.Store.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) GetByID(ctx context.Context, id int64) (membershippayment.Payment, error) {
	p, err := r.q.GetMembershipPaymentByID(ctx, id)
	if err != nil {
		return membershippayment.Payment{}, err
	}
	return fromRow(p), nil
}

func (r *Repo) GetPendingForUser(ctx context.Context, userID int64) (membershippayment.Payment, error) {
	p, err := r.q.GetPendingPaymentForUser(ctx, userID)
	if err != nil {
		return membershippayment.Payment{}, err
	}
	return fromRow(p), nil
}

func (r *Repo) ListForUser(ctx context.Context, userID int64, limit, offset int64) ([]membershippayment.Payment, error) {
	rows, err := r.q.ListUserMembershipPayments(ctx, generated.ListUserMembershipPaymentsParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32Ptr(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]membershippayment.Payment, 0, len(rows))
	for _, row := range rows {
		p := fromRow(generated.MembershipPayment{
			ID: row.ID, UserID: row.UserID, MembershipPlanID: row.MembershipPlanID,
			MembershipID: row.MembershipID, Amount: row.Amount, Method: row.Method,
			Reference: row.Reference, PaidAt: row.PaidAt, ProofPath: row.ProofPath,
			Notes: row.Notes, ReviewNotes: row.ReviewNotes, Status: row.Status,
			ReviewedBy: row.ReviewedBy, ReviewedAt: row.ReviewedAt, CreatedBy: row.CreatedBy,
		})
		p.PlanName = row.PlanName
		out = append(out, p)
	}
	return out, nil
}

func (r *Repo) CountForUser(ctx context.Context, userID int64) (int64, error) {
	return r.q.CountUserMembershipPayments(ctx, userID)
}

func (r *Repo) List(ctx context.Context, status *string, limit, offset int64) ([]membershippayment.Payment, error) {
	rows, err := r.q.ListMembershipPayments(ctx, generated.ListMembershipPaymentsParams{
		Status: status,
		Limit:  int32(limit),
		Offset: int32Ptr(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]membershippayment.Payment, 0, len(rows))
	for _, row := range rows {
		p := fromRow(generated.MembershipPayment{
			ID: row.ID, UserID: row.UserID, MembershipPlanID: row.MembershipPlanID,
			MembershipID: row.MembershipID, Amount: row.Amount, Method: row.Method,
			Reference: row.Reference, PaidAt: row.PaidAt, ProofPath: row.ProofPath,
			Notes: row.Notes, ReviewNotes: row.ReviewNotes, Status: row.Status,
			ReviewedBy: row.ReviewedBy, ReviewedAt: row.ReviewedAt, CreatedBy: row.CreatedBy,
		})
		p.PlanName, p.UserName, p.UserEmail = row.PlanName, row.UserName, row.UserEmail
		out = append(out, p)
	}
	return out, nil
}

func (r *Repo) Count(ctx context.Context, status *string) (int64, error) {
	return r.q.CountMembershipPayments(ctx, status)
}

func (r *Repo) ListForMembership(ctx context.Context, membershipID int64) ([]membershippayment.Payment, error) {
	rows, err := r.q.ListMembershipPaymentsByMembership(ctx, &membershipID)
	if err != nil {
		return nil, err
	}
	out := make([]membershippayment.Payment, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromRow(row))
	}
	return out, nil
}

func (r *Repo) Create(ctx context.Context, p membershippayment.Payment) (int64, error) {
	return r.q.CreateMembershipPayment(ctx, generated.CreateMembershipPaymentParams{
		UserID:           p.UserID,
		MembershipPlanID: p.MembershipPlanID,
		MembershipID:     p.MembershipID,
		Amount:           memberconv.NumValue(p.Amount),
		Method:           p.Method,
		Reference:        p.Reference,
		PaidAt:           memberconv.Date(&p.PaidAt),
		ProofPath:        p.ProofPath,
		Notes:            p.Notes,
		ReviewNotes:      p.ReviewNotes,
		Status:           string(p.Status),
		ReviewedBy:       p.ReviewedBy,
		ReviewedAt:       memberconv.TS(p.ReviewedAt),
		CreatedBy:        p.CreatedBy,
	})
}

func (r *Repo) Reject(ctx context.Context, id int64, reviewNotes string, reviewerID *int64) error {
	return r.q.RejectMembershipPayment(ctx, generated.RejectMembershipPaymentParams{
		ReviewNotes: &reviewNotes,
		ReviewedBy:  reviewerID,
		ID:          id,
	})
}

// fromRow maps a generated payment row to the domain type.
func fromRow(p generated.MembershipPayment) membershippayment.Payment {
	return membershippayment.Payment{
		ID:               p.ID,
		UserID:           p.UserID,
		MembershipPlanID: p.MembershipPlanID,
		MembershipID:     p.MembershipID,
		Amount:           memberconv.NumString(p.Amount),
		Method:           p.Method,
		Reference:        p.Reference,
		PaidAt:           memberconv.DateValue(p.PaidAt),
		ProofPath:        p.ProofPath,
		Notes:            p.Notes,
		ReviewNotes:      p.ReviewNotes,
		Status:           membershippayment.Status(p.Status),
		ReviewedBy:       p.ReviewedBy,
		ReviewedAt:       memberconv.TSPtr(p.ReviewedAt),
		CreatedBy:        p.CreatedBy,
	}
}

// int32Ptr maps the optional OFFSET to its generated *int32 form (nil when 0).
func int32Ptr(v int64) *int32 {
	if v == 0 {
		return nil
	}
	i := int32(v)
	return &i
}
