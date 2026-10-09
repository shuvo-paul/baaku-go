package permission_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/permission"
)

type fakeStore struct {
	names []string
	err   error
}

func (f *fakeStore) UserPermissionNames(_ context.Context, _ int64) ([]string, error) {
	return f.names, f.err
}

func TestCan(t *testing.T) {
	tests := []struct {
		name  string
		store *fakeStore
		want  bool
	}{
		{"granted", &fakeStore{names: []string{"manage roles", "view activity log"}}, true},
		{"not granted", &fakeStore{names: []string{"manage roles"}}, false},
		{"no permissions", &fakeStore{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := permission.New(tt.store).Can(context.Background(), 1, "view activity log")
			if err != nil {
				t.Fatalf("Can: %v", err)
			}
			if got != tt.want {
				t.Errorf("Can = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanStoreError(t *testing.T) {
	_, err := permission.New(&fakeStore{err: errors.New("boom")}).Can(context.Background(), 1, "x")
	if err == nil {
		t.Fatal("want error, got nil")
	}
}
