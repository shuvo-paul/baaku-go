// Package role ports RoleController (reference) — the dashboard's roles CRUD.
// A role grants a set of permission names; the routes sit behind the
// "manage roles" permission (reference StoreRoleRequest/UpdateRoleRequest
// authorize + the permission:manage roles route group). Every mutation logs to
// the spatie activity log under the "role_management" domain.
package role

import (
	"context"
	"errors"
	"strings"

	"github.com/shuvo-paul/baaku/internal/service/activitylog"
)

// Role is the domain representation of a roles row with its granted
// permission names.
type Role struct {
	ID          int64
	Name        string
	Permissions []string
}

// ErrInUse is returned by Delete when the role is still assigned to at least
// one user (reference: role_has_users, redirect with error, no delete).
var ErrInUse = errors.New("role still has users")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence slice the service needs; *repository/role Repo
// satisfies it.
type Store interface {
	List(ctx context.Context) ([]Role, error)
	Get(ctx context.Context, id int64) (Role, error)
	AllPermissionNames(ctx context.Context) ([]string, error)
	// NameExists reports whether a role with this name already exists; id is
	// ignored when checking uniqueness on update (Rule::unique->ignore).
	NameExists(ctx context.Context, name string, ignoreID int64) (bool, error)
	Create(ctx context.Context, name string, permissions []string) (int64, error)
	Update(ctx context.Context, id int64, name string, permissions []string) error
	HasUsers(ctx context.Context, id int64) (bool, error)
	Delete(ctx context.Context, id int64) error
}

// ActivityLogger is the spatie activitylog port (service/activitylog).

type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

type Service struct {
	store  Store
	logger ActivityLogger
}

func New(store Store, logger ActivityLogger) *Service {
	return &Service{store: store, logger: logger}
}

func (s *Service) List(ctx context.Context) ([]Role, error) {
	return s.store.List(ctx)
}

func (s *Service) Get(ctx context.Context, id int64) (Role, error) {
	return s.store.Get(ctx, id)
}

// AllPermissionNames returns every permission name for the create/edit forms.
func (s *Service) AllPermissionNames(ctx context.Context) ([]string, error) {
	return s.store.AllPermissionNames(ctx)
}

// Create validates the name (required, string, max:255, unique) and the
// permission names (exists:permissions,name), inserts the role with its
// grants, and logs the role_management/created event.
func (s *Service) Create(ctx context.Context, name string, permissions []string) (int64, error) {
	name, perms, errs := s.validate(ctx, name, permissions, 0)
	if len(errs) > 0 {
		return 0, errs
	}
	id, err := s.store.Create(ctx, name, perms)
	if err != nil {
		return 0, err
	}
	if err := s.log(ctx, "created", "role created", id, perms, nil); err != nil {
		return 0, err
	}
	return id, nil
}

// Update validates (unique ignoring self), renames the role, re-syncs its
// permissions, and logs the role_management/updated event with the added and
// removed permission sets.
func (s *Service) Update(ctx context.Context, id int64, name string, permissions []string) error {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	name, perms, errs := s.validate(ctx, name, permissions, id)
	if len(errs) > 0 {
		return errs
	}
	if err := s.store.Update(ctx, id, name, perms); err != nil {
		return err
	}
	delta := permDelta(cur.Permissions, perms)
	return s.log(ctx, "updated", "role updated", id, perms, delta)
}

// Delete refuses to remove a role still assigned to users (reference: role_has_users),
// otherwise logs the role_management/deleted event and deletes the row. Logs
// before the delete, matching the reference order.
func (s *Service) Delete(ctx context.Context, id int64) error {
	cur, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	inUse, err := s.store.HasUsers(ctx, id)
	if err != nil {
		return err
	}
	if inUse {
		return ErrInUse
	}
	if err := s.log(ctx, "deleted", "role deleted", id, nil, map[string]any{
		"role_name": cur.Name,
	}); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// log writes one role_management entry performed on the role, persisting the
// spatie properties bag (permissions / permissions_added / removed /
// role_name) that the reference attaches via ->withProperties(). The causer is
// the acting member; callers pass the current user's ID.
func (s *Service) log(ctx context.Context, event, description string, roleID int64, perms []string, props map[string]any) error {
	if props == nil {
		props = map[string]any{}
	}
	if perms != nil {
		props["permissions"] = perms
	}
	return s.logger.Log(ctx, activitylog.Entry{
		LogName:     "role_management",
		Event:       event,
		Description: description,
		Subject:     &activitylog.Subject{Kind: activitylog.KindRole, ID: roleID},
		Properties:  props,
	})
}

// validate enforces the StoreRoleRequest/UpdateRoleRequest rules for the name
// (required, string, unique ignoring ignoreID) and each permission name (must
// be a known permission). Returns the trimmed name, the sanitized permission
// list, and any field errors.
func (s *Service) validate(ctx context.Context, name string, permissions []string, ignoreID int64) (string, []string, FieldErrors) {
	errs := FieldErrors{}
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		errs["name"] = "The name field is required."
	case len(name) > 255:
		errs["name"] = "The name must not be greater than 255 characters."
	default:
		if exists, err := s.store.NameExists(ctx, name, ignoreID); err == nil && exists {
			errs["name"] = "The name has already been taken."
		}
	}

	perms := sanitizePerms(permissions)
	if known, err := s.store.AllPermissionNames(ctx); err == nil {
		knownSet := make(map[string]bool, len(known))
		for _, k := range known {
			knownSet[k] = true
		}
		for _, p := range perms {
			if !knownSet[p] {
				errs["permissions"] = "The selected permission is invalid."
				break
			}
		}
	}

	if len(errs) > 0 {
		return name, perms, errs
	}
	return name, perms, nil
}

// sanitizePerms trims and drops empty entries from the submitted permission
// names.
func sanitizePerms(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// permDelta returns the reference's permissions_added / permissions_removed
// bags (array_diff both ways) for the role_management/updated event.
func permDelta(cur, next []string) map[string]any {
	curSet := make(map[string]bool, len(cur))
	for _, c := range cur {
		curSet[c] = true
	}
	nextSet := make(map[string]bool, len(next))
	for _, n := range next {
		nextSet[n] = true
	}
	var added, removed []string
	for _, n := range next {
		if !curSet[n] {
			added = append(added, n)
		}
	}
	for _, c := range cur {
		if !nextSet[c] {
			removed = append(removed, c)
		}
	}
	return map[string]any{
		"permissions_added":   added,
		"permissions_removed": removed,
	}
}
