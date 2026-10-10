package committee_test

import (
	"context"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/committee"
)

// fakeStore is an in-memory committee.Store.
type fakeStore struct {
	members    []committee.Member
	positions  []committee.Position
	users      []committee.SearchUser
	maxSort    int32
	nameDup    bool
	posExists  bool
	userExists bool
}

func (f *fakeStore) ListMembers(context.Context) ([]committee.Member, error) {
	return f.members, nil
}
func (f *fakeStore) GetMember(_ context.Context, id int64) (committee.Member, error) {
	for _, m := range f.members {
		if m.ID == id {
			return m, nil
		}
	}
	return committee.Member{}, errNoRows{}
}
func (f *fakeStore) MaxMemberSortOrder(context.Context) (int32, error) { return f.maxSort, nil }
func (f *fakeStore) CreateMember(_ context.Context, m committee.Member) (int64, error) {
	f.members = append(f.members, m)
	return 99, nil
}
func (f *fakeStore) UpdateMember(context.Context, committee.Member) error { return nil }
func (f *fakeStore) DeleteMember(context.Context, int64) error            { return nil }
func (f *fakeStore) ReorderMember(context.Context, int64, int32) error    { return nil }
func (f *fakeStore) ListPositions(context.Context) ([]committee.Position, error) {
	return f.positions, nil
}
func (f *fakeStore) GetPosition(_ context.Context, id int64) (committee.Position, error) {
	for _, p := range f.positions {
		if p.ID == id {
			return p, nil
		}
	}
	return committee.Position{}, errNoRows{}
}
func (f *fakeStore) CreatePosition(context.Context, string) (int64, error) { return 1, nil }
func (f *fakeStore) UpdatePosition(context.Context, int64, string) error   { return nil }
func (f *fakeStore) DeletePosition(context.Context, int64) error           { return nil }
func (f *fakeStore) PositionNameExists(context.Context, string, int64) (bool, error) {
	return f.nameDup, nil
}
func (f *fakeStore) PositionIDExists(context.Context, int64) (bool, error) {
	return f.posExists, nil
}
func (f *fakeStore) UserIDExists(context.Context, int64) (bool, error) { return f.userExists, nil }
func (f *fakeStore) SearchUsers(_ context.Context, q string) ([]committee.SearchUser, error) {
	return f.users, nil
}

// errNoRows mirrors pgx.ErrNoRows for the fake.
type errNoRows struct{}

func (errNoRows) Error() string { return "no rows" }

func strp(s string) *string { return &s }
func i64p(n int64) *int64   { return &n }

func TestMemberDisplayAndVacant(t *testing.T) {
	member := committee.Member{UserName: strp("Labony"), Name: strp("Nick"), UserID: i64p(1)}
	if got := member.DisplayName(); got != "Labony" {
		t.Errorf("DisplayName prefers user name, got %q", got)
	}
	if member.Vacant() {
		t.Error("member with user must not be vacant")
	}

	named := committee.Member{Name: strp("Nick")}
	if got := named.DisplayName(); got != "Nick" {
		t.Errorf("DisplayName falls back to member name, got %q", got)
	}
	if named.Vacant() {
		t.Error("member with name must not be vacant")
	}

	vacant := committee.Member{}
	if got := vacant.DisplayName(); got != "—" {
		t.Errorf("DisplayName falls back to em dash, got %q", got)
	}
	if !vacant.Vacant() {
		t.Error("empty member must be vacant")
	}
}

func TestMemberPhotoURL(t *testing.T) {
	withPhoto := committee.Member{PhotoPath: strp("committee-photos/a.jpg"), UserPhotoPath: strp("profile-photos/b.jpg")}
	if got := withPhoto.PhotoURL(); got != "/media/committee-photos/a.jpg" {
		t.Errorf("committee photo wins, got %q", got)
	}
	userOnly := committee.Member{UserPhotoPath: strp("profile-photos/b.jpg")}
	if got := userOnly.PhotoURL(); got != "/media/profile-photos/b.jpg" {
		t.Errorf("falls back to profile photo, got %q", got)
	}
	if got := (committee.Member{}).PhotoURL(); got != "" {
		t.Errorf("no photo yields empty URL, got %q", got)
	}
}

func TestStoreValidation(t *testing.T) {
	ctx := context.Background()

	// Missing position.
	svc := committee.New(&fakeStore{})
	if _, err := svc.Store(ctx, committee.StoreInput{Name: strp("Nick")}); err == nil {
		t.Fatal("expected position required error")
	} else if !hasFieldErr(err, "position_id") {
		t.Errorf("expected position_id error, got %v", err)
	}

	// Neither user nor name (reference withValidator after()).
	svc = committee.New(&fakeStore{posExists: true})
	_, err := svc.Store(ctx, committee.StoreInput{PositionID: i64p(1)})
	if err == nil {
		t.Fatal("expected name-or-user required error")
	} else if !hasFieldErr(err, "name") {
		t.Errorf("expected name error, got %v", err)
	}

	// A registered user passes without a name; new members append (max+1).
	svc = committee.New(&fakeStore{posExists: true, userExists: true, maxSort: 4})
	id, err := svc.Store(ctx, committee.StoreInput{PositionID: i64p(1), UserID: i64p(7)})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if id != 99 {
		t.Errorf("Store id = %d, want 99", id)
	}

	// Duplicate position name (unique:positions,name).
	svc = committee.New(&fakeStore{nameDup: true})
	if _, err := svc.StorePosition(ctx, "President"); !hasFieldErr(err, "name") {
		t.Errorf("expected unique name error, got %v", err)
	}
}

// hasFieldErr reports whether the error carries a FieldErrors entry for field.
func hasFieldErr(err error, field string) bool {
	errs, ok := err.(committee.FieldErrors)
	if !ok {
		return false
	}
	_, ok = errs[field]
	return ok
}
