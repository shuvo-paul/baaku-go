package membershipgate_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/membershipgate"
)

// fakeChecker stubs the membership feature checks.
type fakeChecker struct {
	hasActive bool
	granted   bool
	err       error
}

func (f *fakeChecker) HasActive(_ context.Context, _ int64) (bool, error) {
	return f.hasActive, f.err
}

func (f *fakeChecker) GrantedFeature(_ context.Context, _ int64, _ string) (bool, error) {
	return f.granted, f.err
}

func TestGateDisabledPassesEveryone(t *testing.T) {
	// While the memberships feature is off, every user passes
	// (reference canAccessMembershipFeature early return).
	g := membershipgate.New(false, &fakeChecker{hasActive: false, granted: false})
	ok, err := g.CanAccess(context.Background(), 1, "members", nil)
	if err != nil || !ok {
		t.Fatalf("CanAccess = %v, %v; want true, nil", ok, err)
	}
}

func TestGateAdminBypasses(t *testing.T) {
	g := membershipgate.New(true, &fakeChecker{hasActive: false, granted: false})
	for _, perm := range []string{"manage members", "manage memberships", "manage membership plans"} {
		ok, err := g.CanAccess(context.Background(), 1, "members", []string{perm})
		if err != nil || !ok {
			t.Errorf("admin with %q: CanAccess = %v, %v; want true, nil", perm, ok, err)
		}
	}
}

func TestGateNeedsActiveMembershipAndGrant(t *testing.T) {
	tests := []struct {
		name    string
		checker *fakeChecker
		want    bool
	}{
		{"no active membership", &fakeChecker{hasActive: false}, false},
		{"active but plan lacks feature", &fakeChecker{hasActive: true, granted: false}, false},
		{"active and granted", &fakeChecker{hasActive: true, granted: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := membershipgate.New(true, tt.checker)
			ok, err := g.CanAccess(context.Background(), 1, "members", []string{"some-other"})
			if err != nil {
				t.Fatalf("CanAccess error: %v", err)
			}
			if ok != tt.want {
				t.Errorf("CanAccess = %v, want %v", ok, tt.want)
			}
		})
	}
}

func TestGatePropagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	g := membershipgate.New(true, &fakeChecker{err: sentinel})
	_, err := g.CanAccess(context.Background(), 1, "members", nil)
	if err == nil {
		t.Fatal("CanAccess: want error, got nil")
	}
}
