package confirm

import "testing"

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
			if got := validAt(tt.expiration, now); got != tt.want {
				t.Errorf("validAt(%d, %d) = %v, want %v", tt.expiration, now, got, tt.want)
			}
		})
	}
}

func TestKey(t *testing.T) {
	if got := key(42); got != "password.confirmation.42" {
		t.Errorf("key(42) = %q, want password.confirmation.42", got)
	}
}
