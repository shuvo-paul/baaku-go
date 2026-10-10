// Package userrole ports UserRoleController@edit/update (reference) — assigning
// dashboard roles to a member. The routes sit behind the "manage members"
// permission (reference Route::middleware('permission:manage members')). A role
// is referenced by name throughout, matching spatie's syncRoles, and every sync
// logs to the spatie activity log under the "member_management" domain.
package userrole

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Target is the user roles are being assigned to; *repository/user Repo.GetByID
// (via the TargetStore port) satisfies it.
type Target struct {
	ID            int64
	Name          string
	Email         string
	EmailVerified bool
	AssignedRoles []string
}

// TargetStore reads the target user (findOrFail → pgx.ErrNoRows); the user
// repository (*repository/user) satisfies it.
type TargetStore interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
}

// AssignedRoleReader reads a user's currently assigned role names (reference
// UserRoleController@edit: $user->roles->pluck('name')); the role repository
// (*repository/role) satisfies it.
type AssignedRoleReader interface {
	UserRolesFor(ctx context.Context, userID int64) ([]string, error)
}

// Roles reads every role name and replaces a user's role set (reference
// $user->syncRoles); *repository/role Repo satisfies it.
type Roles interface {
	// AllRoleNames returns every role name for the edit form and validation.
	AllRoleNames(ctx context.Context) ([]string, error)
	// SyncUserRoles replaces a user's whole role set (reference syncRoles).
	SyncUserRoles(ctx context.Context, userID int64, roleNames []string) error
}

// ActivityLogger is the spatie activitylog port (service/activitylog).
type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

type Service struct {
	targets      TargetStore
	assigned     AssignedRoleReader
	roles        Roles
	logger       ActivityLogger
	defaultRoles []string
}

// New wires the service. defaultRoles mirrors config('auth.default_roles');
// index 0 is the admin role the self-demotion guard protects. Tests pass fakes.
func New(targets TargetStore, assigned AssignedRoleReader, roles Roles, logger ActivityLogger, defaultRoles []string) *Service {
	return &Service{targets: targets, assigned: assigned, roles: roles, logger: logger, defaultRoles: defaultRoles}
}

// ErrNotFound is a missing target user; handlers 404 (reference findOrFail).
var ErrNotFound = errors.New("user not found")

// ErrUnverified is the reference unverified_user_no_transition guard.
var ErrUnverified = errors.New("email must be verified before membership actions")

// ErrCannotRemoveOwnAdmin is the reference cannot_remove_own_admin guard.
var ErrCannotRemoveOwnAdmin = errors.New("you cannot remove your own admin role")

// FieldErrors mirrors Laravel validator messages keyed by field (the
// roles/roles.* exists:roles,name rules).
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Form loads the target user with their assigned roles and the full role list
// for the edit checkboxes (reference UserRoleController@edit).
func (s *Service) Form(ctx context.Context, targetID int64) (Target, []string, error) {
	target, _, err := s.loadTarget(ctx, targetID)
	if err != nil {
		return Target{}, nil, err
	}
	roles, err := s.roles.AllRoleNames(ctx)
	if err != nil {
		return Target{}, nil, err
	}
	return target, roles, nil
}

// Update replaces a target's role set (reference UserRoleController@update):
// guards an unverified target, validates the role names exist, blocks removing
// one's own admin role, then syncs and logs the member_management/roles_synced
// event with the added and removed names.
func (s *Service) Update(ctx context.Context, actorID, targetID int64, requested []string) error {
	target, current, err := s.loadTarget(ctx, targetID)
	if err != nil {
		return err
	}
	// Reference guard order: unverified target first.
	if !target.EmailVerified {
		return ErrUnverified
	}

	requested = sanitizeNames(requested)
	if errs := s.validateRoles(ctx, requested); len(errs) > 0 {
		return errs
	}

	// Self-demotion guard: the actor may not drop their own admin role.
	if actorID == targetID {
		admin := s.adminRole()
		if admin != "" && contains(current, admin) && !contains(requested, admin) {
			return ErrCannotRemoveOwnAdmin
		}
	}

	if err := s.roles.SyncUserRoles(ctx, targetID, requested); err != nil {
		return err
	}

	return s.logger.Log(ctx, activitylog.Entry{
		LogName:     "member_management",
		Event:       "roles_synced",
		Description: "roles updated",
		Subject:     &activitylog.Subject{Kind: activitylog.KindUser, ID: targetID},
		Properties: map[string]any{
			"roles_added":   diff(requested, current),
			"roles_removed": diff(current, requested),
		},
		Causer: &activitylog.Causer{ID: actorID},
	})
}

// loadTarget fetches the target user plus their assigned role names.
func (s *Service) loadTarget(ctx context.Context, targetID int64) (Target, []string, error) {
	u, err := s.targets.GetByID(ctx, targetID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Target{}, nil, ErrNotFound
		}
		return Target{}, nil, err
	}
	assigned, err := s.assigned.UserRolesFor(ctx, targetID)
	if err != nil {
		return Target{}, nil, err
	}
	return Target{
		ID:            u.ID,
		Name:          u.Name,
		Email:         u.Email,
		EmailVerified: u.EmailVerifiedAt != nil,
		AssignedRoles: assigned,
	}, assigned, nil
}

// validateRoles enforces the roles.* => exists:roles,name rule; unknown names
// produce one field error. It depends on the writer listing known role names.
func (s *Service) validateRoles(ctx context.Context, names []string) FieldErrors {
	errs := FieldErrors{}
	known, err := s.roles.AllRoleNames(ctx)
	if err != nil {
		return errs
	}
	knownSet := make(map[string]bool, len(known))
	for _, k := range known {
		knownSet[k] = true
	}
	for _, n := range names {
		if !knownSet[n] {
			errs["roles"] = "The selected role is invalid."
			break
		}
	}
	return errs
}

// adminRole is the protected role at config('auth.default_roles')[0].
func (s *Service) adminRole() string {
	if len(s.defaultRoles) == 0 {
		return ""
	}
	return s.defaultRoles[0]
}

// sanitizeNames trims and drops empty entries from the checkbox input.
func sanitizeNames(in []string) []string {
	out := make([]string, 0, len(in))
	for _, n := range in {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

// diff returns the entries in a that are not in b (array_diff), preserving
// order for the activity-log properties.
func diff(a, b []string) []string {
	inB := make(map[string]bool, len(b))
	for _, v := range b {
		inB[v] = true
	}
	out := []string{}
	for _, v := range a {
		if !inB[v] {
			out = append(out, v)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
