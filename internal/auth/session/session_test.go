package session

import (
	"testing"
	"time"
)

func TestExpiryCutoff(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name     string
		lifetime time.Duration
		want     int64
	}{
		{"120 minutes", 120 * time.Minute, now.Unix() - 7200},
		{"1 minute", time.Minute, now.Unix() - 60},
		{"zero lifetime", 0, now.Unix()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expiryCutoff(now, tt.lifetime); got != tt.want {
				t.Errorf("expiryCutoff() = %d, want %d", got, tt.want)
			}
		})
	}
}
