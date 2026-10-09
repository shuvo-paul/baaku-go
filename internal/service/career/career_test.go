package career_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/service/career"
)

type fakeStore struct {
	created   []career.Career
	updated   map[int64]career.Career
	deleted   []int64
	profileID int64
	getErr    error
}

func (f *fakeStore) List(_ context.Context, _ int64) ([]career.Career, error) { return nil, nil }

func (f *fakeStore) Get(_ context.Context, _, id int64) (career.Career, error) {
	if f.getErr != nil {
		return career.Career{}, f.getErr
	}
	if c, ok := f.updated[id]; ok {
		return c, nil
	}
	return career.Career{ID: id}, nil
}

func (f *fakeStore) ProfileID(_ context.Context, _ int64) (int64, error) { return f.profileID, nil }

func (f *fakeStore) Create(_ context.Context, profileID int64, c career.Career) error {
	f.profileID = profileID
	f.created = append(f.created, c)
	return nil
}

func (f *fakeStore) Update(_ context.Context, id int64, c career.Career) error {
	if f.updated == nil {
		f.updated = map[int64]career.Career{}
	}
	f.updated[id] = c
	return nil
}

func (f *fakeStore) Delete(_ context.Context, id int64) error {
	f.deleted = append(f.deleted, id)
	return nil
}

type fakeReviewer struct{ calls int }

func (f *fakeReviewer) Submit(_ context.Context, _ int64) error { f.calls++; return nil }

func validInput() career.Input {
	return career.Input{
		EmploymentType: "full_time", JobTitle: "Engineer", Company: "Acme",
		StartYear: "2021", StartMonth: "5", EndYear: "2023",
	}
}

func TestCreatePersistsUnderProfile(t *testing.T) {
	store := &fakeStore{profileID: 42}
	rev := &fakeReviewer{}
	if err := career.New(store, rev).Create(context.Background(), 7, validInput()); err != nil {
		t.Fatalf("Create = %v, want nil", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("created = %d rows, want 1", len(store.created))
	}
	c := store.created[0]
	if store.profileID != 42 || c.JobTitle != "Engineer" || c.EmploymentType != "full_time" {
		t.Errorf("stored = %+v (profile %d)", c, store.profileID)
	}
	if rev.calls != 1 {
		t.Errorf("reviewer calls = %d, want 1", rev.calls)
	}
}

func TestCreateCurrentSkipsEnd(t *testing.T) {
	in := validInput()
	in.IsCurrent = true
	store := &fakeStore{}
	if err := career.New(store, &fakeReviewer{}).Create(context.Background(), 1, in); err != nil {
		t.Fatalf("Create = %v", err)
	}
	if store.created[0].EndYear != nil {
		t.Error("end_year set although is_current")
	}
}

func TestValidationMessages(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*career.Input)
		wantSub string
	}{
		{"employment type invalid", func(in *career.Input) { in.EmploymentType = "robot" }, "employment type is invalid"},
		{"job title required", func(in *career.Input) { in.JobTitle = "" }, "The job title field is required."},
		{"company required", func(in *career.Input) { in.Company = "" }, "The company field is required."},
		{"start year required", func(in *career.Input) { in.StartYear = "" }, "The start_year field is required."},
		{"end before start", func(in *career.Input) { in.EndYear = "2020" }, "greater than or equal"},
		{"description too long", func(in *career.Input) { in.Description = strings.Repeat("x", 5001) }, "must not be greater than 5000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			err := career.New(&fakeStore{}, &fakeReviewer{}).Create(context.Background(), 1, in)
			var errs career.FieldErrors
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
	err := career.New(store, &fakeReviewer{}).Update(context.Background(), 1, 42, validInput())
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("err = %v, want pgx.ErrNoRows", err)
	}
}
