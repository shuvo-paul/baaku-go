package user

import "testing"

// want pins the exact transition matrix from
// reference/app/Enums/UserState.php.
var want = map[UserState][]UserState{
	StateUnverified: {},
	StatePending:    {StateActive, StateRejected},
	StateActive:     {StateSuspended},
	StateSuspended:  {StateActive},
	StateRejected:   {StatePending},
}

func TestUserStateCanTransitionTo(t *testing.T) {
	states := []UserState{StateUnverified, StatePending, StateActive, StateSuspended, StateRejected}

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
	got := map[UserState]string{
		StateUnverified: "unverified",
		StatePending:    "pending",
		StateActive:     "active",
		StateSuspended:  "suspended",
		StateRejected:   "rejected",
	}
	for state, value := range got {
		if string(state) != value {
			t.Errorf("state %q: got value %q, want %q", state, state, value)
		}
	}
}
