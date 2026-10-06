package user_test

import (
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/user"
)

// want pins the exact transition matrix from
// reference/app/Enums/UserState.php.
var want = map[user.UserState][]user.UserState{
	user.StateUnverified: {},
	user.StatePending:    {user.StateActive, user.StateRejected},
	user.StateActive:     {user.StateSuspended},
	user.StateSuspended:  {user.StateActive},
	user.StateRejected:   {user.StatePending},
}

func TestUserStateCanTransitionTo(t *testing.T) {
	states := []user.UserState{user.StateUnverified, user.StatePending, user.StateActive, user.StateSuspended, user.StateRejected}

	for _, from := range states {
		for _, to := range states {
			wantOK := false
			for _, w := range want[from] {
				if w == to {
					wantOK = true
				}
			}
			if got := from.CanTransitionTo(to); got != wantOK {
				t.Errorf("%s -> %s: got %v, want %v", from, to, got, wantOK)
			}
		}
	}
}

func TestUserStateValues(t *testing.T) {
	got := map[user.UserState]string{
		user.StateUnverified: "unverified",
		user.StatePending:    "pending",
		user.StateActive:     "active",
		user.StateSuspended:  "suspended",
		user.StateRejected:   "rejected",
	}
	for state, value := range got {
		if string(state) != value {
			t.Errorf("state %q: got value %q, want %q", state, state, value)
		}
	}
}
