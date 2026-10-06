package confirm_test

import (
	"testing"

	"github.com/shuvo-paul/baaku/internal/repository/confirm"
)

func TestValidAt(t *testing.T) {
	now := int64(1_700_000_000)
	tests := []struct {
		name       string
		expiration int64
		want       bool
	}{
		{"expired one second ago", now - 1, false},
		{"expired long ago", now - 10800, false},
		{"expires exactly now", now, false}, // RequirePassword: now - markedAt < timeout
		{"valid one second left", now + 1, true},
		{"freshly marked", now + 10800, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confirm.ValidAt(tt.expiration, now); got != tt.want {
				t.Errorf("ValidAt(%d, %d) = %v, want %v", tt.expiration, now, got, tt.want)
			}
		})
	}
}
