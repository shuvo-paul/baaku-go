// Package profilereview ports SubmitProfileForReview (reference
// app/Actions/SubmitProfileForReview.php): a rejected member who edits their
// profile re-enters the pending review queue.
//
// The reference also logs a spatie activity row ('profile resubmitted for
// review'); the activity-log writer lands in the admin/activity wave, so this
// port only performs the state transition.
package profilereview

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Store is the persistence slice the service needs.
type Store interface {
	State(ctx context.Context, userID int64) (user.UserState, error)
	SetState(ctx context.Context, userID int64, state user.UserState) error
}

type Service struct {
	store Store
}

func New(store Store) *Service { return &Service{store: store} }

// Submit flips a rejected user back to pending; every other state is a no-op
// (reference: only UserState::Rejected triggers the update).
func (s *Service) Submit(ctx context.Context, userID int64) error {
	st, err := s.store.State(ctx, userID)
	if err != nil {
		return err
	}
	if st != user.StateRejected {
		return nil
	}
	return s.store.SetState(ctx, userID, user.StatePending)
}
