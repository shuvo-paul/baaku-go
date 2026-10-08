package completeprofile_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/completeprofile"
)

// fakes

type fakeStore struct {
	userID             int64
	gender, blood      string
	present, permanent string
	err                error
}

func (f *fakeStore) UpsertDetails(_ context.Context, userID int64, gender, bloodGroup, presentAddress, permanentAddress string) error {
	f.userID, f.gender, f.blood = userID, gender, bloodGroup
	f.present, f.permanent = presentAddress, permanentAddress
	return f.err
}

// harness

func validInput() completeprofile.Input {
	return completeprofile.Input{
		Gender:           "female",
		BloodGroup:       "O+",
		PresentAddress:   "12 Lake Road",
		PermanentAddress: "12 Lake Road",
	}
}

// tests

func TestCompletePersistsGateFields(t *testing.T) {
	store := &fakeStore{}
	err := completeprofile.New(store).Complete(context.Background(), 7, validInput())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if store.userID != 7 || store.gender != "female" || store.blood != "O+" ||
		store.present != "12 Lake Road" || store.permanent != "12 Lake Road" {
		t.Errorf("stored = %+v for user %d", store, store.userID)
	}
}

func TestCompleteValidationMessages(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*completeprofile.Input)
		wantSub string
	}{
		{"gender required", func(in *completeprofile.Input) { in.Gender = "" }, "The gender field is required."},
		{"gender invalid", func(in *completeprofile.Input) { in.Gender = "robot" }, "The selected gender is invalid."},
		{"blood required", func(in *completeprofile.Input) { in.BloodGroup = "" }, "The blood group field is required."},
		{"blood invalid", func(in *completeprofile.Input) { in.BloodGroup = "Zz" }, "The selected blood group is invalid."},
		{"present required", func(in *completeprofile.Input) { in.PresentAddress = "" }, "The present address field is required."},
		{"permanent required", func(in *completeprofile.Input) { in.PermanentAddress = "" }, "The permanent address field is required."},
		{"present too long", func(in *completeprofile.Input) { in.PresentAddress = strings.Repeat("a", 256) }, "The present address field must not be greater than 255 characters."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)
			err := completeprofile.New(&fakeStore{}).Complete(context.Background(), 7, in)
			var errs completeprofile.FieldErrors
			if !errors.As(err, &errs) {
				t.Fatalf("err = %v, want FieldErrors", err)
			}
			if !strings.Contains(errs.Error(), tt.wantSub) {
				t.Errorf("messages = %q, want substring %q", errs.Error(), tt.wantSub)
			}
		})
	}
}

func TestCompleteStoreErrorPropagates(t *testing.T) {
	want := errors.New("boom")
	err := completeprofile.New(&fakeStore{err: want}).Complete(context.Background(), 7, validInput())
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
