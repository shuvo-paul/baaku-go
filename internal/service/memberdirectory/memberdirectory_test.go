package memberdirectory_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/career"
	"github.com/shuvo-paul/baaku/internal/service/education"
	"github.com/shuvo-paul/baaku/internal/service/memberdirectory"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fake store

type fakeStore struct {
	members  map[int64]memberdirectory.Member
	listOpts memberdirectory.ListOptions
	listPage memberdirectory.Page
	counts   map[string]int64
	states   map[int64]user.UserState
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		members: map[int64]memberdirectory.Member{},
		counts:  map[string]int64{},
		states:  map[int64]user.UserState{},
	}
}

func (f *fakeStore) List(_ context.Context, opts memberdirectory.ListOptions) (memberdirectory.Page, error) {
	f.listOpts = opts
	return f.listPage, nil
}

func (f *fakeStore) CountsByState(_ context.Context) (map[string]int64, error) {
	return f.counts, nil
}

func (f *fakeStore) Get(_ context.Context, id int64, activeOnly bool) (memberdirectory.Member, error) {
	m, ok := f.members[id]
	if !ok {
		return memberdirectory.Member{}, pgx.ErrNoRows
	}
	if activeOnly && m.State != user.StateActive {
		return memberdirectory.Member{}, pgx.ErrNoRows
	}
	return m, nil
}

func (f *fakeStore) SetState(_ context.Context, id int64, state user.UserState) error {
	f.states[id] = state
	if m, ok := f.members[id]; ok {
		m.State = state
		f.members[id] = m
	}
	return nil
}

// fakes for the composed deps

type fakeLogger struct{ entries []activitylog.Entry }

func (f *fakeLogger) Log(_ context.Context, e activitylog.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

type sentMail struct{ to, subject, body string }

type fakeNotifier struct{ sent []sentMail }

func (f *fakeNotifier) send(to, subject, body string) error {
	f.sent = append(f.sent, sentMail{to, subject, body})
	return nil
}

type fakeEducations struct{ list []education.Education }

func (f *fakeEducations) List(_ context.Context, _ int64) ([]education.Education, error) {
	return f.list, nil
}

type fakeCareers struct{ list []career.Career }

func (f *fakeCareers) List(_ context.Context, _ int64) ([]career.Career, error) {
	return f.list, nil
}

func newService(store *fakeStore, log *fakeLogger, mail *fakeNotifier) *memberdirectory.Service {
	return memberdirectory.New(store, &fakeEducations{}, &fakeCareers{}, log, mail.send, "http://example.test")
}

// tests

func TestDirectoryAdminSeesFiltersAndCounts(t *testing.T) {
	store := newFakeStore()
	store.counts = map[string]int64{"pending": 2, "active": 5}
	svc := newService(store, &fakeLogger{}, &fakeNotifier{})
	// Invalid filter falls back to "all" (reference in_array guard).
	if _, _, err := svc.Directory(context.Background(), true, "bogus", "", 1); err != nil {
		t.Fatalf("Directory: %v", err)
	}
	if store.listOpts.ActiveOnly {
		t.Error("admin query should not force active_only")
	}
	if store.listOpts.State != "" {
		t.Errorf("state = %q, want empty (all)", store.listOpts.State)
	}
	// A concrete filter passes through.
	if _, _, err := svc.Directory(context.Background(), true, "pending", "x", 2); err != nil {
		t.Fatalf("Directory: %v", err)
	}
	if store.listOpts.State != "pending" || store.listOpts.Search != "x" {
		t.Errorf("opts = %+v", store.listOpts)
	}
	if store.listOpts.Offset != memberdirectory.PerPage {
		t.Errorf("offset = %d, want %d", store.listOpts.Offset, memberdirectory.PerPage)
	}
}

func TestDirectoryMemberForcesActiveOnly(t *testing.T) {
	store := newFakeStore()
	svc := newService(store, &fakeLogger{}, &fakeNotifier{})
	pg, counts, err := svc.Directory(context.Background(), false, "pending", "", 0)
	if err != nil {
		t.Fatalf("Directory: %v", err)
	}
	if !store.listOpts.ActiveOnly {
		t.Error("non-admin query must force active_only")
	}
	if len(counts) != 0 {
		t.Errorf("counts = %v, want empty for non-admins", counts)
	}
	if pg.Total != 0 {
		t.Errorf("total = %d", pg.Total)
	}
}

func TestGetComposesNarrativeLists(t *testing.T) {
	store := newFakeStore()
	store.members[7] = memberdirectory.Member{
		MemberCard: memberdirectory.MemberCard{ID: 7, Name: "Ada", State: user.StateActive},
	}
	edus := &fakeEducations{list: []education.Education{{ID: 1, Level: "bsc", Institution: "TU"}}}
	crs := &fakeCareers{list: []career.Career{{ID: 1, JobTitle: "Engineer", Company: "ACME"}}}
	svc := memberdirectory.New(store, edus, crs, &fakeLogger{}, (&fakeNotifier{}).send, "http://example.test")
	m, err := svc.Get(context.Background(), false, 7)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if m.ID != 7 {
		t.Errorf("id = %d", m.ID)
	}
	if len(m.Educations) != 1 || m.Educations[0].Institution != "TU" {
		t.Errorf("educations = %+v", m.Educations)
	}
	if len(m.Careers) != 1 || m.Careers[0].Company != "ACME" {
		t.Errorf("careers = %+v", m.Careers)
	}
}

func TestGetNonAdminCannotSeeSuspended(t *testing.T) {
	store := newFakeStore()
	store.members[9] = memberdirectory.Member{
		MemberCard: memberdirectory.MemberCard{ID: 9, State: user.StateSuspended},
	}
	svc := newService(store, &fakeLogger{}, &fakeNotifier{})
	if _, err := svc.Get(context.Background(), false, 9); !errors.Is(err, memberdirectory.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	// Admins see every state.
	if _, err := svc.Get(context.Background(), true, 9); err != nil {
		t.Fatalf("admin Get: %v", err)
	}
}

func TestChangeStatePendingToActive(t *testing.T) {
	store := newFakeStore()
	store.members[3] = memberdirectory.Member{
		MemberCard: memberdirectory.MemberCard{ID: 3, Name: "Ada", Email: "ada@example.test", State: user.StatePending, EmailVerified: true},
	}
	log := &fakeLogger{}
	mail := &fakeNotifier{}
	svc := newService(store, log, mail)
	if err := svc.ChangeState(context.Background(), 1, 3, "active", ""); err != nil {
		t.Fatalf("ChangeState: %v", err)
	}
	if store.states[3] != user.StateActive {
		t.Errorf("state = %v", store.states[3])
	}
	// Reference logs member_management/state_changed with the properties bag.
	if len(log.entries) != 1 {
		t.Fatalf("log entries = %d", len(log.entries))
	}
	e := log.entries[0]
	if e.LogName != "member_management" || e.Event != "state_changed" {
		t.Errorf("entry = %+v", e)
	}
	if e.Properties["old_state"] != "pending" || e.Properties["new_state"] != "active" {
		t.Errorf("properties = %+v", e.Properties)
	}
	if e.Causer == nil || e.Causer.ID != 1 {
		t.Errorf("causer = %+v", e.Causer)
	}
	if e.Subject == nil || e.Subject.ID != 3 || e.Subject.Kind != activitylog.KindUser {
		t.Errorf("subject = %+v", e.Subject)
	}
	// UserActivatedNotification.
	if len(mail.sent) != 1 || mail.sent[0].subject != "Your account has been activated" {
		t.Errorf("mail = %+v", mail.sent)
	}
}

func TestChangeStateSuspendRequiresReason(t *testing.T) {
	store := newFakeStore()
	store.members[4] = memberdirectory.Member{
		MemberCard: memberdirectory.MemberCard{ID: 4, State: user.StateActive, EmailVerified: true},
	}
	mail := &fakeNotifier{}
	svc := newService(store, &fakeLogger{}, mail)
	err := svc.ChangeState(context.Background(), 1, 4, "suspended", "  ")
	var errs memberdirectory.FieldErrors
	if !errors.As(err, &errs) || errs["reason"] == "" {
		t.Fatalf("err = %v, want reason FieldError", err)
	}
	if len(mail.sent) != 0 {
		t.Error("no mail on validation failure")
	}
	// With a reason it goes through and notifies (reference wording).
	if err := svc.ChangeState(context.Background(), 1, 4, "suspended", "inactive for a year"); err != nil {
		t.Fatalf("ChangeState: %v", err)
	}
	if len(mail.sent) != 1 || mail.sent[0].subject != "Your account has been suspended" {
		t.Errorf("mail = %+v", mail.sent)
	}
}

func TestChangeStateGuards(t *testing.T) {
	verified := func(id int64, st user.UserState) memberdirectory.Member {
		return memberdirectory.Member{MemberCard: memberdirectory.MemberCard{ID: id, State: st, EmailVerified: true}}
	}
	tests := []struct {
		name    string
		target  memberdirectory.Member
		actor   int64
		state   string
		reason  string
		wantErr error
	}{
		{"unverified target", memberdirectory.Member{MemberCard: memberdirectory.MemberCard{ID: 5, State: user.StatePending}}, 1, "active", "", memberdirectory.ErrUnverified},
		{"self change", verified(1, user.StateActive), 1, "suspended", "because", memberdirectory.ErrSelfChange},
		{"invalid transition", verified(6, user.StateActive), 1, "active", "", memberdirectory.ErrInvalidTransition},
		{"unknown state value", verified(6, user.StatePending), 1, "wat", "", nil}, // FieldErrors, checked below
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			store.members[tt.target.ID] = tt.target
			svc := newService(store, &fakeLogger{}, &fakeNotifier{})
			err := svc.ChangeState(context.Background(), tt.actor, tt.target.ID, tt.state, tt.reason)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			var errs memberdirectory.FieldErrors
			if !errors.As(err, &errs) || errs["state"] == "" {
				t.Errorf("err = %v, want state FieldError", err)
			}
		})
	}
}

func TestChangeStateRejectNotifiesWithReason(t *testing.T) {
	store := newFakeStore()
	store.members[8] = memberdirectory.Member{
		MemberCard: memberdirectory.MemberCard{ID: 8, State: user.StatePending, EmailVerified: true},
	}
	mail := &fakeNotifier{}
	svc := newService(store, &fakeLogger{}, mail)
	if err := svc.ChangeState(context.Background(), 1, 8, "rejected", "missing documents"); err != nil {
		t.Fatalf("ChangeState: %v", err)
	}
	if store.states[8] != user.StateRejected {
		t.Errorf("state = %v", store.states[8])
	}
	if len(mail.sent) != 1 {
		t.Fatalf("mail sent = %d", len(mail.sent))
	}
	if mail.sent[0].subject != "Your membership application was not approved" {
		t.Errorf("subject = %q", mail.sent[0].subject)
	}
	if !strings.Contains(mail.sent[0].body, "missing documents") {
		t.Errorf("body = %q, want the reason line", mail.sent[0].body)
	}
}
