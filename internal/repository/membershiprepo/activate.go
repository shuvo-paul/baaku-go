package membershiprepo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/membership"
)

// Payment statuses activation accepts (reference PaymentStatus).
const (
	payStatusPending  = "pending"
	payStatusApproved = "approved"
)

// ActivateFromPayment runs the reference ActivateMembership path in one
// transaction: create or extend the user's membership, then approve+link the
// payment. Idempotent per payment (a payment already linked to a membership is
// never applied twice). The caller passes the plan's term (lifetime flag +
// duration) because the term calculator lives in the plan service, above this
// repo.
func (r *Repo) ActivateFromPayment(ctx context.Context, req membership.ActivateRequest) (membership.ActivateResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return membership.ActivateResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit
	q := r.q.WithTx(tx)

	// Lock + read the payment; enforce the already-activated / activatable
	// guards before doing any work (reference guards).
	p, err := q.LockMembershipPaymentForUpdate(ctx, req.PaymentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return membership.ActivateResult{}, membership.ErrNotFound
	}
	if err != nil {
		return membership.ActivateResult{}, err
	}
	if p.MembershipID != nil {
		return membership.ActivateResult{}, membership.ErrAlreadyActivated
	}
	if p.Status != payStatusPending && p.Status != payStatusApproved {
		return membership.ActivateResult{}, membership.ErrPaymentNotActivatable
	}

	// Lock the user's active membership.
	existing, err := q.LockActiveMembershipForUser(ctx, req.UserID)
	isNew := false
	var membershipID int64
	var endsAt *time.Time

	if errors.Is(err, pgx.ErrNoRows) {
		start := time.Now()
		if p.PaidAt.Valid {
			start = p.PaidAt.Time
		}
		endsAt = req.Term.EndFrom(start)
		startCopy := start
		membershipID, err = q.CreateMembership(ctx, generated.CreateMembershipParams{
			UserID:           req.UserID,
			MembershipPlanID: req.PlanID,
			Status:           string(membership.StatusActive),
			StartsAt:         ts(&startCopy),
			EndsAt:           ts(endsAt),
			CreatedBy:        req.ReviewerID,
		})
		if err != nil {
			return membership.ActivateResult{}, err
		}
		isNew = true
	} else if err != nil {
		return membership.ActivateResult{}, err
	} else if req.Term.IsLifetime {
		// Lifetime memberships are never shortened or downgraded.
		membershipID = existing.ID
	} else {
		base := startOfDay(time.Now())
		if existing.EndsAt.Valid && existing.EndsAt.Time.After(time.Now()) {
			base = startOfDay(existing.EndsAt.Time)
		}
		endsAt = req.Term.EndFrom(base)
		if err := q.UpdateMembershipTerm(ctx, generated.UpdateMembershipTermParams{
			MembershipPlanID: req.PlanID,
			EndsAt:           ts(endsAt),
			ID:               existing.ID,
		}); err != nil {
			return membership.ActivateResult{}, err
		}
		membershipID = existing.ID
	}

	// Approve + link the payment.
	if err := q.ApproveMembershipPayment(ctx, generated.ApproveMembershipPaymentParams{
		ReviewedBy:   req.ReviewerID,
		MembershipID: &membershipID,
		ID:           req.PaymentID,
	}); err != nil {
		return membership.ActivateResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return membership.ActivateResult{}, err
	}
	return membership.ActivateResult{
		MembershipID: membershipID,
		IsNew:        isNew,
		EndsAt:       endsAt,
	}, nil
}

// startOfDay truncates to the calendar day (reference startOfDay()).
func startOfDay(t time.Time) time.Time {
	y, mo, d := t.Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, t.Location())
}
