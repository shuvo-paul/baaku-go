package middleware

import (
	"context"
	"net/http"
)

// MembershipPath is the member-facing membership page the gate redirects to
// (reference route name dashboard.membership.show).
const MembershipPath = "/dashboard/membership"

// FeatureLockedErrorKey is the flash value the membership page renders as the
// "feature locked" message (reference membership.feature_locked).
const FeatureLockedErrorKey = "membership.feature-locked"

// MembershipFeatureChecker answers the membership feature gate for a user;
// *membershipgate.Gate satisfies it.
type MembershipFeatureChecker interface {
	// CanAccess reports whether the user may reach the named gated feature.
	CanAccess(ctx context.Context, userID int64, feature string, perms []string) (bool, error)
}

// RequireMembershipFeature mirrors the reference MembershipFeature middleware:
// while the memberships feature is enabled, a user needs an active membership
// whose plan grants $feature (or a membership-administration permission) to
// proceed; otherwise they are redirected to their membership page. It must run
// after LoadPermissions so the permission names are in the context.
func RequireMembershipFeature(gate MembershipFeatureChecker, feature string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := UserFromContext(r.Context())
			if !ok {
				Redirect(w, r, LoginPath)
				return
			}
			allowed, err := gate.CanAccess(r.Context(), u.ID, feature, PermissionsFromContext(r.Context()))
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !allowed {
				RedirectWithFlash(w, r, MembershipPath, "error", FeatureLockedErrorKey)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
