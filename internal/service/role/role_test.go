package role_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/role"
)

// fake store

type fakeStore struct {
	roles       map[int64]role.Role
	nextID      int64
	knownPerms  []string
	takenNames  map[string]bool
	usersOnRole map[int64]bool

	createdName  string
	createdPerms []string
	updatedID    int64
	updatedName  string
	updatedPerms []string
	deletedID    int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		roles:       map[int64]role.Role{},
		knownPerms:  []string{"manage roles", "view activity log"},
		takenNames:  map[string]bool{},
		usersOnRole: map[int64]bool{},
	}
}

func (f *fakeStore) List(_ context.Context) ([]role.Role, error) {
	out := make([]role.Role, 0, len(f.roles))
	for id := int64(1); id < f.nextID; id++ {
		if r, ok := f.roles[id]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, id int64) (role.Role, error) {
	r, ok := f.roles[id]
	if !ok {
		return role.Role{}, errors.New("not found")
	}
	return r, nil
}

func (f *fakeStore) AllPermissionNames(_ context.Context) ([]string, error) {
	return f.knownPerms, nil
}

func (f *fakeStore) NameExists(_ context.Context, name string, ignoreID int64) (bool, error) {
	for id, r := range f.roles {
		if r.Name == name && id != ignoreID {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStore) Create(_ context.Context, name string, permissions []string) (int64, error) {
	f.nextID++
	f.roles[f.nextID] = role.Role{ID: f.nextID, Name: name, Permissions: permissions}
	f.createdName, f.createdPerms = name, permissions
	return f.nextID, nil
}

func (f *fakeStore) Update(_ context.Context, id int64, name string, permissions []string) error {
	f.roles[id] = role.Role{ID: id, Name: name, Permissions: permissions}
	f.updatedID, f.updatedName, f.updatedPerms = id, name, permissions
	return nil
}

func (f *fakeStore) HasUsers(_ context.Context, id int64) (bool, error) {
	return f.usersOnRole[id], nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	delete(f.roles, id)
	f.deletedID = id
	return nil
}

// fake logger

type fakeLogger struct {
	entries []activitylog.Entry
}

func (f *fakeLogger) Log(_ context.Context, e activitylog.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

// tests

func TestCreatePersistsRoleAndLogsCreated(t *testing.T) {
	store := newFakeStore()
	log := &fakeLogger{}
	id, err := role.New(store, log).Create(context.Background(), "editor", []string{"manage roles"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if id == 0 || store.createdName != "editor" || len(store.createdPerms) != 1 {
		t.Errorf("stored id=%d name=%q perms=%v", id, store.createdName, store.createdPerms)
	}
	// Reference logs role_management/created performed on the role with the
	// permissions property.
	if len(log.entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(log.entries))
	}
	e := log.entries[0]
	if e.LogName != "role_management" || e.Event != "created" || e.Description != "role created" {
		t.Errorf("entry = %+v", e)
	}
	if e.Subject == nil || e.Subject.Kind != activitylog.KindRole || e.Subject.ID != id {
		t.Errorf("subject = %+v, want role %d", e.Subject, id)
	}
	if perms, _ := e.Properties["permissions"].([]string); len(perms) != 1 || perms[0] != "manage roles" {
		t.Errorf("properties = %+v", e.Properties)
	}
}

func TestCreateValidation(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		perms    []string
		wantSub  string
	}{
		{"name required", "", nil, "The name field is required."},
		{"name too long", string(make([]byte, 256)), nil, "The name must not be greater than 255 characters."},
		{"unknown permission", "editor", []string{"nope"}, "The selected permission is invalid."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			_, err := role.New(store, &fakeLogger{}).Create(context.Background(), tt.roleName, tt.perms)
			var errs role.FieldErrors
			if !errors.As(err, &errs) {
				t.Fatalf("err = %v, want FieldErrors", err)
			}
			if errs["name"] != tt.wantSub && errs["permissions"] != tt.wantSub {
				t.Errorf("errors = %v, want substring %q", errs, tt.wantSub)
			}
		})
	}
}

func TestCreateDuplicateNameRejected(t *testing.T) {
	store := newFakeStore()
	svc := role.New(store, &fakeLogger{})
	if _, err := svc.Create(context.Background(), "editor", nil); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := svc.Create(context.Background(), "editor", nil)
	var errs role.FieldErrors
	if !errors.As(err, &errs) || errs["name"] != "The name has already been taken." {
		t.Errorf("err = %v, want unique violation", err)
	}
}

func TestUpdateSyncsPermsAndLogsDelta(t *testing.T) {
	store := newFakeStore()
	log := &fakeLogger{}
	svc := role.New(store, log)
	id, err := svc.Create(context.Background(), "editor", []string{"manage roles"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	err = svc.Update(context.Background(), id, "senior editor", []string{"view activity log"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if store.updatedName != "senior editor" || len(store.updatedPerms) != 1 || store.updatedPerms[0] != "view activity log" {
		t.Errorf("updated = %q %v", store.updatedName, store.updatedPerms)
	}
	if len(log.entries) != 2 {
		t.Fatalf("log entries = %d, want 2", len(log.entries))
	}
	e := log.entries[1]
	if e.Event != "updated" {
		t.Errorf("event = %q, want updated", e.Event)
	}
	added, _ := e.Properties["permissions_added"].([]string)
	removed, _ := e.Properties["permissions_removed"].([]string)
	if len(added) != 1 || added[0] != "view activity log" || len(removed) != 1 || removed[0] != "manage roles" {
		t.Errorf("delta added=%v removed=%v", added, removed)
	}
}

func TestUpdateAllowsUnchangedName(t *testing.T) {
	store := newFakeStore()
	svc := role.New(store, &fakeLogger{})
	id, _ := svc.Create(context.Background(), "editor", nil)
	// Renaming to itself must not trip the unique rule (ignore self).
	if err := svc.Update(context.Background(), id, "editor", nil); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestDeleteRefusesRoleInUse(t *testing.T) {
	store := newFakeStore()
	svc := role.New(store, &fakeLogger{})
	id, _ := svc.Create(context.Background(), "editor", nil)
	store.usersOnRole[id] = true
	err := svc.Delete(context.Background(), id)
	if !errors.Is(err, role.ErrInUse) {
		t.Fatalf("err = %v, want ErrInUse", err)
	}
	if store.deletedID != 0 {
		t.Errorf("role was deleted despite users")
	}
}

func TestDeleteRemovesFreeRoleAndLogsDeleted(t *testing.T) {
	store := newFakeStore()
	log := &fakeLogger{}
	svc := role.New(store, log)
	id, _ := svc.Create(context.Background(), "editor", nil)
	if err := svc.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.deletedID != id {
		t.Errorf("deletedID = %d, want %d", store.deletedID, id)
	}
	e := log.entries[len(log.entries)-1]
	if e.Event != "deleted" || e.Description != "role deleted" {
		t.Errorf("entry = %+v", e)
	}
	if name, _ := e.Properties["role_name"].(string); name != "editor" {
		t.Errorf("role_name property = %v", e.Properties["role_name"])
	}
}
