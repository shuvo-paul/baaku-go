// Package activitylog ports spatie/laravel-activitylog for the three
// dashboard domains (member_management, role_management, profile) and the
// admin activity page (reference ActivityLogController@index).
//
// The reference builds entries with the fluent logger
// (activity('profile')->performedOn($user)->event('submitted')->log(...));
// this port records the same fields in one Entry value.
package activitylog

import (
	"context"
	"encoding/json"
	"time"

	activityrepo "github.com/shuvo-paul/baaku/internal/repository/activitylog"
)

// Morph kinds for activity subjects (spatie stores the morph class).
const (
	KindUser = "user"
	KindRole = "role"
)

// Entry is one activity row to write, mirroring the fields spatie's
// activity() builder collects before ->log().
type Entry struct {
	LogName     string
	Event       string
	Description string
	// Subject is the affected model (user or role); nil = no subject.
	Subject *Subject
	// Causer is the acting member; nil = System.
	Causer *Causer
	// Properties is the spatie ->withProperties() bag persisted as JSON on the
	// row (permissions, permissions_added/removed, role_name, …); nil = none.
	Properties map[string]any
}

// Subject is the affected model of an entry.
type Subject struct {
	Kind string // KindUser or KindRole
	ID   int64
}

// Causer is the acting member (always a user).
type Causer struct {
	ID int64
}

// Store persists entries and answers the admin feed; *repository/activitylog
// .Repo satisfies it.
type Store interface {
	Insert(ctx context.Context, logName, description, subjectType string, subjectID, causerID *int64, event string, properties []byte) error
	ListPage(ctx context.Context, page, pageSize int) ([]activityrepo.Row, bool, error)
}

// Item is one activity row resolved for display.
type Item struct {
	ID          int64
	LogName     string
	Event       string
	Description string
	// Actor is the causer's display name; "System" when the row has none
	// (reference activity_log.unknown_actor).
	Actor string
	// SubjectName is the subject's display name (user name/email or role
	// name); empty when the row has no subject.
	SubjectName string
	// SubjectID identifies the subject row for links; 0 = no subject.
	SubjectID int64
	// label and (once those routes exist) the edit link.
	SubjectKind string
	// Properties is the decoded spatie properties bag; nil when empty.
	Properties map[string]any
	CreatedAt  time.Time
}

// Service writes activity rows and answers the admin activity feed.
type Service struct {
	store    Store
	pageSize int
}

// pageSizeAdminFeed mirrors the reference simplePaginate(20).
const pageSizeAdminFeed = 20

func New(store Store) *Service {
	return &Service{store: store, pageSize: pageSizeAdminFeed}
}

// Log writes one activity row. A failure is returned to the caller — the
// reference's ->log() throws on write failure too.
func (s *Service) Log(ctx context.Context, e Entry) error {
	var subjectID, causerID *int64
	subjectTypeStr := ""
	if e.Subject != nil {
		subjectTypeStr = e.Subject.Kind
		subjectID = &e.Subject.ID
	}
	if e.Causer != nil {
		causerID = &e.Causer.ID
	}
	var props []byte
	if e.Properties != nil {
		if b, err := json.Marshal(e.Properties); err == nil {
			props = b
		}
	}
	return s.store.Insert(ctx, e.LogName, e.Description, subjectTypeStr, subjectID, causerID, e.Event, props)
}

// Recent returns one page of the admin activity feed (newest first) and
// whether a next page exists.
func (s *Service) Recent(ctx context.Context, page int) ([]Item, bool, error) {
	rows, more, err := s.store.ListPage(ctx, page, s.pageSize)
	if err != nil {
		return nil, false, err
	}
	items := make([]Item, 0, len(rows))
	for _, r := range rows {
		items = append(items, itemFromRow(r))
	}
	return items, more, nil
}

// systemActor is the display name for rows without a causer.
const systemActor = "System"

func itemFromRow(r activityrepo.Row) Item {
	it := Item{
		ID:          r.ID,
		LogName:     r.LogName,
		Event:       r.Event,
		Description: r.Description,
		CreatedAt:   r.CreatedAt,
	}
	if r.CauserName != "" {
		it.Actor = r.CauserName
	} else {
		it.Actor = systemActor
	}
	it.SubjectID = r.SubjectID
	switch {
	case r.SubjectUserName != "":
		it.SubjectName, it.SubjectKind = r.SubjectUserName, KindUser
	case r.SubjectUserEmail != "":
		it.SubjectName, it.SubjectKind = r.SubjectUserEmail, KindUser
	case r.SubjectRoleName != "":
		it.SubjectName, it.SubjectKind = r.SubjectRoleName, KindRole
	}
	if len(r.Properties) > 0 {
		// spatie writes a JSON object (or []); only an object has pairs to show.
		var props map[string]any
		if err := json.Unmarshal(r.Properties, &props); err == nil && len(props) > 0 {
			it.Properties = props
		}
	}
	return it
}
