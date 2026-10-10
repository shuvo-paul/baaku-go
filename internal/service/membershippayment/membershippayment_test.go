package membershippayment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
)

// fakeLogger is a no-op activity logger.
type fakeLogger struct{}

func (fakeLogger) Log(context.Context, activitylog.Entry) error { return nil }

// fakeStore is a no-op payment store.
type fakeStore struct{}

func (fakeStore) GetByID(context.Context, int64) (membershippayment.Payment, error) {
	return membershippayment.Payment{}, nil
}
func (fakeStore) GetPendingForUser(context.Context, int64) (membershippayment.Payment, error) {
	return membershippayment.Payment{}, nil
}
func (fakeStore) ListForUser(context.Context, int64, int64, int64) ([]membershippayment.Payment, error) {
	return nil, nil
}
func (fakeStore) CountForUser(context.Context, int64) (int64, error) { return 0, nil }
func (fakeStore) List(context.Context, *string, int64, int64) ([]membershippayment.Payment, error) {
	return nil, nil
}
func (fakeStore) Count(context.Context, *string) (int64, error) { return 0, nil }
func (fakeStore) ListForMembership(context.Context, int64) ([]membershippayment.Payment, error) {
	return nil, nil
}
func (fakeStore) Create(context.Context, membershippayment.Payment) (int64, error) { return 0, nil }
func (fakeStore) Reject(context.Context, int64, string, *int64) error              { return nil }

// fakePlans returns a fixed active plan.
type fakePlans struct {
	price    string
	isActive bool
}

func (f fakePlans) Get(context.Context, int64) (membershipplan.Plan, error) {
	return membershipplan.Plan{ID: 1, Price: f.price, IsActive: f.isActive}, nil
}

// fakeMethods returns fixed active method types.
type fakeMethods struct{ types []string }

func (f fakeMethods) ActiveTypes(context.Context) ([]string, error) { return f.types, nil }

// fakeMembers is a no-op activator.

func validate(t *testing.T, plans fakePlans, methods fakeMethods, in membershippayment.SubmitInput) membershippayment.FieldErrors {
	t.Helper()
	// members is nil: validation never reaches activation (Activate stays false).
	svc := membershippayment.New(fakeStore{}, nil, plans, methods, fakeLogger{})
	_, _, err := svc.Submit(context.Background(), in)
	var fieldErrs membershippayment.FieldErrors
	if err != nil {
		if !errors.As(err, &fieldErrs) {
			t.Fatalf("Submit err = %v, want FieldErrors", err)
		}
	}
	return fieldErrs
}

func baseInput() membershippayment.SubmitInput {
	return membershippayment.SubmitInput{
		UserID: 1, PlanID: 1, Amount: "1500.00", Method: "bkash",
		PaidAt: time.Now(), CreatedBy: 1,
	}
}

func TestSubmitValidation(t *testing.T) {
	okPlans := fakePlans{price: "1500.00", isActive: true}
	okMethods := fakeMethods{types: []string{"bkash"}}

	tests := []struct {
		name    string
		plans   fakePlans
		methods fakeMethods
		mutate  func(*membershippayment.SubmitInput)
		wantKey string
	}{
		{"valid input has no errors", okPlans, okMethods, func(*membershippayment.SubmitInput) {}, ""},
		{"amount mismatch", okPlans, okMethods, func(in *membershippayment.SubmitInput) { in.Amount = "10.00" }, "amount"},
		{"inactive plan", fakePlans{price: "1500.00", isActive: false}, okMethods, func(*membershippayment.SubmitInput) {}, "membership_plan_id"},
		{"invalid method", okPlans, fakeMethods{types: []string{"nagad"}}, func(*membershippayment.SubmitInput) {}, "method"},
		{"future paid date", okPlans, okMethods, func(in *membershippayment.SubmitInput) { in.PaidAt = time.Now().AddDate(0, 0, 1) }, "paid_at"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := baseInput()
			tt.mutate(&in)
			errs := validate(t, tt.plans, tt.methods, in)
			if tt.wantKey == "" {
				if len(errs) != 0 {
					t.Fatalf("errs = %v, want none", errs)
				}
				return
			}
			if errs[tt.wantKey] == "" {
				t.Errorf("errs = %v, want key %q", errs, tt.wantKey)
			}
		})
	}
}
