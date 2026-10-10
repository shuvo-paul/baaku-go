package membershipplan_test

import (
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
)

func int32p(v int32) *int32 { return &v }

func TestTermEndFrom(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		plan membershipplan.Plan
		want *time.Time
	}{
		{"lifetime has no end", membershipplan.Plan{IsLifetime: true}, nil},
		{"duration adds days", membershipplan.Plan{DurationDays: int32p(30)}, ptrTime(start.AddDate(0, 0, 30))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.plan.TermEndFrom(start)
			if (got == nil) != (tt.want == nil) {
				t.Fatalf("TermEndFrom = %v, want %v", got, tt.want)
			}
			if got != nil && !got.Equal(*tt.want) {
				t.Errorf("TermEndFrom = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTermLabel(t *testing.T) {
	tests := []struct {
		name string
		plan membershipplan.Plan
		want string
	}{
		{"lifetime", membershipplan.Plan{IsLifetime: true}, "Lifetime"},
		{"days", membershipplan.Plan{DurationDays: int32p(7)}, "7 days"},
		{"one day", membershipplan.Plan{DurationDays: int32p(1)}, "1 day"},
		{"months when whole months", membershipplan.Plan{DurationDays: int32p(60)}, "2 months"},
		{"12 months", membershipplan.Plan{DurationDays: int32p(360)}, "12 months"},
		{"non-month multiple renders days", membershipplan.Plan{DurationDays: int32p(365)}, "365 days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.plan.TermLabel(); got != tt.want {
				t.Errorf("TermLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFeatureGranted(t *testing.T) {
	p := membershipplan.Plan{Features: map[string]string{"members": "1", "off": "0"}}
	if !p.FeatureGranted("members") {
		t.Error("members should be granted")
	}
	if p.FeatureGranted("off") {
		t.Error("off should not be granted")
	}
	if p.FeatureGranted("missing") {
		t.Error("missing should not be granted")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
