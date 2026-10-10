// Package profilereview ports SubmitProfileForReview (reference
// app/Actions/SubmitProfileForReview.php): a rejected member who edits their
// profile re-enters the pending review queue.
//
// The reference also logs a spatie activity row ('profile resubmitted for
// review'); this port writes it through the activitylog service after the
// state flip.
package profilereview

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Store is the persistence slice the service needs.
type Store interface {
	State(ctx context.Context, userID int64) (user.UserState, error)
	SetState(ctx context.Context, userID int64, state user.UserState) error
}

// ActivityLogger records one activity row; *activitylog.Service satisfies it.
type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

type Service struct {
	store      Store
	activities ActivityLogger
}

func New(store Store, activities ActivityLogger) *Service {
	return &Service{store: store, activities: activities}
}

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
	if err := s.store.SetState(ctx, userID, user.StatePending); err != nil {
		return err
	}
	// Reference logs after the flip: activity('profile')->performedOn($user)
	// ->event('resubmitted')->log('profile resubmitted for review').
	return s.activities.Log(ctx, activitylog.Entry{
		LogName:     "profile",
		Event:       "resubmitted",
		Description: "profile resubmitted for review",
		Subject:     &activitylog.Subject{Kind: activitylog.KindUser, ID: userID},
		Causer:      &activitylog.Causer{ID: userID},
	})
}
