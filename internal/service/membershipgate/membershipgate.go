// Package membershipgate ports the HasMemberships@canAccessMembershipFeature
// decision. Gating is only active while the memberships feature is enabled;
// staff who administer memberships always bypass the gate; everyone else needs
// an active membership whose plan grants the feature.
package membershipgate

import (
	"context"
)

// adminPermissions bypass the membership gate (reference
// HasMemberships@isMembershipAdmin).
var adminPermissions = []string{"manage members", "manage memberships", "manage membership plans"}

// membershipAdmin reports whether the viewer holds one of the bypassing
// permissions.
func membershipAdmin(perms []string) bool {
	for _, p := range perms {
		for _, a := range adminPermissions {
			if p == a {
				return true
			}
		}
	}
	return false
}

// FeatureChecker reads whether a user holds an active membership and what their
// plan grants; *membership.Service + plan satisfy it.
type FeatureChecker interface {
	// HasActive reports an active, unexpired membership.
	HasActive(ctx context.Context, userID int64) (bool, error)
	// GrantedFeature reports whether the user's active plan grants a feature
	// key (nil plan → false).
	GrantedFeature(ctx context.Context, userID int64, key string) (bool, error)
}

// Gate decides whether a user may reach a membership-gated dashboard feature.
type Gate struct {
	featuresEnabled bool
	checker         FeatureChecker
}

// New builds the gate. featuresEnabled mirrors config('features.memberships');
// when false every user passes (reference canAccessMembershipFeature's early
// return).
func New(featuresEnabled bool, checker FeatureChecker) *Gate {
	return &Gate{featuresEnabled: featuresEnabled, checker: checker}
}

// CanAccess reports whether the user may reach the named gated feature
// (reference HasMemberships@canAccessMembershipFeature).
func (g *Gate) CanAccess(ctx context.Context, userID int64, feature string, perms []string) (bool, error) {
	if !g.featuresEnabled {
		return true, nil
	}
	if membershipAdmin(perms) {
		return true, nil
	}
	active, err := g.checker.HasActive(ctx, userID)
	if err != nil || !active {
		return false, err
	}
	return g.checker.GrantedFeature(ctx, userID, feature)
}
