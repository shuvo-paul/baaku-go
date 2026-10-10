package activitylog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	activityrepo "github.com/shuvo-paul/baaku/internal/repository/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
)

// fake store

type fakeStore struct {
	inserted activitylog.Entry
	// args captured from the Insert call (morph kind + ids).
	subjectType string
	subjectID   *int64
	causerID    *int64
	insertErr   error

	rows    []activityrepo.Row
	more    bool
	listErr error
}

func (f *fakeStore) Insert(_ context.Context, logName, description, subjectType string, subjectID, causerID *int64, event string, _ []byte) error {
	f.inserted = activitylog.Entry{
		LogName:     logName,
		Event:       event,
		Description: description,
	}
	if subjectID != nil {
		f.inserted.Subject = &activitylog.Subject{Kind: subjectType, ID: *subjectID}
	}
	if causerID != nil {
		f.inserted.Causer = &activitylog.Causer{ID: *causerID}
	}
	f.subjectType, f.subjectID, f.causerID = subjectType, subjectID, causerID
	return f.insertErr
}

func (f *fakeStore) ListPage(_ context.Context, _, _ int) ([]activityrepo.Row, bool, error) {
	if f.listErr != nil {
		return nil, false, f.listErr
	}
	return f.rows, f.more, nil
}

// tests

func TestLogWritesEntryWithSubjectAndCauser(t *testing.T) {
	store := &fakeStore{}
	svc := activitylog.New(store)
	err := svc.Log(context.Background(), activitylog.Entry{
		LogName:     "profile",
		Event:       "submitted",
		Description: "profile submitted",
		Subject:     &activitylog.Subject{Kind: activitylog.KindUser, ID: 7},
		Causer:      &activitylog.Causer{ID: 7},
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if store.subjectType != activitylog.KindUser || store.subjectID == nil || *store.subjectID != 7 {
		t.Errorf("subject morph = %q id = %v, want user 7", store.subjectType, store.subjectID)
	}
	if store.causerID == nil || *store.causerID != 7 {
		t.Errorf("causer = %v, want 7", store.causerID)
	}
}

func TestLogWithoutSubjectLeavesMorphNil(t *testing.T) {
	store := &fakeStore{}
	svc := activitylog.New(store)
	err := svc.Log(context.Background(), activitylog.Entry{LogName: "x", Event: "e", Description: "d"})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if store.subjectID != nil || store.causerID != nil {
		t.Errorf("subject/causer = %v/%v, want nil/nil", store.subjectID, store.causerID)
	}
}

func TestLogStoreErrorPropagates(t *testing.T) {
	want := errors.New("boom")
	err := activitylog.New(&fakeStore{insertErr: want}).Log(context.Background(), activitylog.Entry{LogName: "x"})
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}

func TestRecentResolvesActorAndSubject(t *testing.T) {
	store := &fakeStore{rows: []activityrepo.Row{
		{ID: 1, LogName: "role_management", Event: "created", Description: "d",
			CauserName: "Ada", SubjectRoleName: "editor", CreatedAt: time.Now()},
		{ID: 2, LogName: "profile", Event: "submitted", Description: "d"}, // no causer → System
	}, more: true}
	items, more, err := activitylog.New(store).Recent(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if !more {
		t.Error("more = false, want true")
	}
	if len(items) != 2 {
		t.Fatalf("len = %d, want 2", len(items))
	}
	if items[0].Actor != "Ada" || items[0].SubjectName != "editor" || items[0].SubjectKind != activitylog.KindRole {
		t.Errorf("item[0] = %+v", items[0])
	}
	if items[1].Actor != "System" || items[1].SubjectName != "" {
		t.Errorf("item[1] = %+v", items[1])
	}
}

func TestRecentDecodesProperties(t *testing.T) {
	store := &fakeStore{rows: []activityrepo.Row{
		{ID: 1, LogName: "member_management", Event: "state_changed", Description: "d",
			Properties: []byte(`{"old":"pending","new":"active"}`)},
	}}
	items, _, err := activitylog.New(store).Recent(context.Background(), 1)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if items[0].Properties["old"] != "pending" || items[0].Properties["new"] != "active" {
		t.Errorf("props = %+v", items[0].Properties)
	}
}
