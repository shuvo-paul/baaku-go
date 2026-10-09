package education_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/service/education"
)

type fakeStore struct {
	created   []education.Education
	updated   map[int64]education.Education
	deleted   []int64
	profileID int64
	getErr    error
}

func (f *fakeStore) List(_ context.Context, _ int64) ([]education.Education, error) { return nil, nil }

func (f *fakeStore) Get(_ context.Context, _, id int64) (education.Education, error) {
	if f.getErr != nil {
		return education.Education{}, f.getErr
	}
	if e, ok := f.updated[id]; ok {
		return e, nil
	}
	return education.Education{ID: id}, nil
}

func (f *fakeStore) ProfileID(_ context.Context, _ int64) (int64, error) { return f.profileID, nil }

func (f *fakeStore) Create(_ context.Context, profileID int64, e education.Education) error {
	f.profileID = profileID
	f.created = append(f.created, e)
	return nil
}
func (f *fakeStore) Update(_ context.Context, id int64, e education.Education) error {
	if f.updated == nil {
		f.updated = map[int64]education.Education{}
	}
	f.updated[id] = e
	return nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeReviewer struct{ calls int }

func (f *fakeReviewer) Submit(_ context.Context, _ int64) error { f.calls++; return nil }

func validInput() education.Input {
	return education.Input{
		Level: "Masters", Institution: "Khulna University", Subject: "Physics",
		StartYear: "2020", StartMonth: "3", EndYear: "2022",
	}
}

func TestCreatePersistsUnderProfile(t *testing.T) {
	store := &fakeStore{profileID: 99}
	rev := &fakeReviewer{}
	if err := education.New(store, rev).Create(context.Background(), 7, validInput()); err != nil {
		t.Fatalf("Create = %v, want nil", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created = %d rows, want 1", len(store.created))
	}
	e := store.created[0]
	if store.profileID != 99 || e.Level != "Masters" || e.StartYear != 2020 {
		t.Errorf("stored = %+v (profile %d)", e, store.profileID)
	}
	if rev.calls != 1 {
		t.Errorf("reviewer calls = %d, want 1", rev.calls)
	}
}

func TestCreateCurrentSkipsEnd(t *testing.T) {
	in := validInput()
	in.IsCurrent = true
	in.EndYear = "2022"
	store := &fakeStore{}
	if err := education.New(store, &fakeReviewer{}).Create(context.Background(), 1, in); err != nil {
		t.Fatalf("Create = %v", err)
	}
	if store.created[0].EndYear != nil {
		t.Errorf("end_year = %v, want nil when is_current", *store.created[0].EndYear)
	}
}

func TestValidationMessages(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*education.Input)
		wantSub string
	}{
		{"level required", func(in *education.Input) { in.Level = "" }, "The level field is required."},
		{"institution required", func(in *education.Input) { in.Institution = "" }, "The institution field is required."},
		{"subject required", func(in *education.Input) { in.Subject = "" }, "The subject field is required."},
		{"start year required", func(in *education.Input) { in.StartYear = "" }, "The start_year field is required."},
		{"start year bad", func(in *education.Input) { in.StartYear = "22" }, "4-digit year"},
		{"end before start", func(in *education.Input) { in.EndYear = "2019" }, "greater than or equal"},
		{"month out of range", func(in *education.Input) { in.StartMonth = "13" }, "must be between 1 and 12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			err := education.New(&fakeStore{}, &fakeReviewer{}).Create(context.Background(), 1, in)
			var errs education.FieldErrors
			if !errors.As(err, &errs) {
				t.Fatalf("err = %v, want FieldErrors", err)
			}
			if !strings.Contains(errs.Error(), tt.wantSub) {
				t.Errorf("messages = %q, want substring %q", errs.Error(), tt.wantSub)
			}
		})
	}
}

func TestUpdateMissingRowIsNotFound(t *testing.T) {
	store := &fakeStore{getErr: pgx.ErrNoRows}
	err := education.New(store, &fakeReviewer{}).Update(context.Background(), 1, 42, validInput())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}

func TestDeleteMissingRowIsNotFound(t *testing.T) {
	store := &fakeStore{getErr: pgx.ErrNoRows}
	err := education.New(store, &fakeReviewer{}).Delete(context.Background(), 1, 42)
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}
