package userrole_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/service/userrole"
)

// fakes

type fakeTargetStore struct {
	users map[int64]user.User
}

func (f *fakeTargetStore) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := f.users[id]
	if !ok {
		return user.User{}, pgx.ErrNoRows
	}
	return u, nil
}

type fakeAssigned struct {
	roles map[int64][]string
}

func (f *fakeAssigned) UserRolesFor(_ context.Context, userID int64) ([]string, error) {
	return f.roles[userID], nil
}

type fakeRoles struct {
	known  []string
	synced map[int64][]string
}

func (f *fakeRoles) AllRoleNames(_ context.Context) ([]string, error) {
	return f.known, nil
}

func (f *fakeRoles) SyncUserRoles(_ context.Context, userID int64, roleNames []string) error {
	if f.synced == nil {
		f.synced = map[int64][]string{}
	}
	f.synced[userID] = roleNames
	return nil
}

type fakeLogger struct {
	entries []activitylog.Entry
}

func (f *fakeLogger) Log(_ context.Context, e activitylog.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

// harness

type harness struct {
	targets  *fakeTargetStore
	assigned *fakeAssigned
	roles    *fakeRoles
	log      *fakeLogger
	svc      *userrole.Service
}

func newHarness(defaultRoles []string) *harness {
	now := time.Now()
	targets := &fakeTargetStore{users: map[int64]user.User{
		1: {ID: 1, Name: "Admin", Email: "admin@example.com", EmailVerifiedAt: &now},
		2: {ID: 2, Name: "Member", Email: "member@example.com", EmailVerifiedAt: &now},
		7: {ID: 7, Name: "Rook", Email: "rook@example.com"}, // unverified
	}}
	assigned := &fakeAssigned{roles: map[int64][]string{1: {"admin"}, 2: {"member"}}}
	roles := &fakeRoles{known: []string{"admin", "moderator", "member"}}
	log := &fakeLogger{}
	if defaultRoles == nil {
		defaultRoles = []string{"admin", "moderator", "member"}
	}
	return &harness{
		targets:  targets,
		assigned: assigned,
		roles:    roles,
		log:      log,
		svc:      userrole.New(targets, assigned, roles, log, defaultRoles),
	}
}

// tests

func TestUpdateSyncsRolesAndLogsDelta(t *testing.T) {
	h := newHarness(nil)
	err := h.svc.Update(context.Background(), 1, 2, []string{"moderator", "member"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := h.roles.synced[2]; len(got) != 2 || got[0] != "moderator" || got[1] != "member" {
		t.Errorf("synced = %v", got)
	}
	if len(h.log.entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(h.log.entries))
	}
	e := h.log.entries[0]
	if e.LogName != "member_management" || e.Event != "roles_synced" || e.Description != "roles updated" {
		t.Errorf("entry = %+v", e)
	}
	if e.Subject == nil || e.Subject.Kind != activitylog.KindUser || e.Subject.ID != 2 {
		t.Errorf("subject = %+v", e.Subject)
	}
	if e.Causer == nil || e.Causer.ID != 1 {
		t.Errorf("causer = %+v", e.Causer)
	}
	added, _ := e.Properties["roles_added"].([]string)
	removed, _ := e.Properties["roles_removed"].([]string)
	// "member" is kept; only "moderator" is added, nothing removed.
	if len(added) != 1 || added[0] != "moderator" || len(removed) != 0 {
		t.Errorf("delta added=%v removed=%v", added, removed)
	}
}

func TestUpdateGuards(t *testing.T) {
	tests := []struct {
		name      string
		actorID   int64
		targetID  int64
		requested []string
		wantErr   error
	}{
		{"unverified target", 1, 7, []string{"member"}, userrole.ErrUnverified},
		{"self demotion of admin", 1, 1, []string{"member"}, userrole.ErrCannotRemoveOwnAdmin},
		{"missing target", 1, 99, []string{"member"}, userrole.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(nil)
			err := h.svc.Update(context.Background(), tt.actorID, tt.targetID, tt.requested)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if h.roles.synced != nil {
				t.Errorf("roles were synced despite guard")
			}
			if len(h.log.entries) != 0 {
				t.Errorf("logged despite guard")
			}
		})
	}
}

func TestUpdateRejectsUnknownRole(t *testing.T) {
	h := newHarness(nil)
	err := h.svc.Update(context.Background(), 1, 2, []string{"nope"})
	var errs userrole.FieldErrors
	if !errors.As(err, &errs) || errs["roles"] != "The selected role is invalid." {
		t.Fatalf("err = %v, want FieldErrors", err)
	}
	if h.roles.synced != nil {
		t.Errorf("roles were synced despite invalid name")
	}
}

func TestUpdateSelfKeepingAdminAllowed(t *testing.T) {
	h := newHarness(nil)
	// Keeping the admin role while adding another is not a demotion.
	if err := h.svc.Update(context.Background(), 1, 1, []string{"admin", "member"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := h.roles.synced[1]; len(got) != 2 {
		t.Errorf("synced = %v", got)
	}
}

func TestUpdateEmptySelectionClearsRoles(t *testing.T) {
	h := newHarness(nil)
	if err := h.svc.Update(context.Background(), 1, 2, nil); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := h.roles.synced[2]; len(got) != 0 {
		t.Errorf("synced = %v, want empty", got)
	}
	e := h.log.entries[len(h.log.entries)-1]
	removed, _ := e.Properties["roles_removed"].([]string)
	if len(removed) != 1 || removed[0] != "member" {
		t.Errorf("removed = %v", removed)
	}
}

func TestFormReturnsTargetAndAllRoles(t *testing.T) {
	h := newHarness(nil)
	target, roles, err := h.svc.Form(context.Background(), 2)
	if err != nil {
		t.Fatalf("Form: %v", err)
	}
	if target.ID != 2 || target.Name != "Member" || target.Email != "member@example.com" || !target.EmailVerified {
		t.Errorf("target = %+v", target)
	}
	if len(target.AssignedRoles) != 1 || target.AssignedRoles[0] != "member" {
		t.Errorf("assigned = %v", target.AssignedRoles)
	}
	if len(roles) != 3 || roles[0] != "admin" {
		t.Errorf("roles = %v", roles)
	}
}
