package passwordchange_test

import (
	"context"
	"errors"
	"testing"

	"github.com/shuvo-paul/baaku/internal/service/password"
	"github.com/shuvo-paul/baaku/internal/service/passwordchange"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

type fakePasswordStore struct {
	u       user.User
	setID   int64
	setHash string
}

func (f *fakePasswordStore) GetByID(ctx context.Context, id int64) (user.User, error) {
	return f.u, nil
}

func (f *fakePasswordStore) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	f.setID, f.setHash = id, passwordHash
	return nil
}

func TestChangePassword(t *testing.T) {
	oldHash, err := password.Hash("OldPass1!")
	if err != nil {
		t.Fatal(err)
	}
	stores := []struct {
		name    string
		current string
		new     string
		confirm string
		wantErr error
	}{
		{"wrong current password rejected", "nope", "NewPass2!", "NewPass2!", passwordchange.ErrCurrentPasswordInvalid},
		{"weak new password rejected", "OldPass1!", "short", "short", nil}, // Validate error, non-nil
		{"unconfirmed new password rejected", "OldPass1!", "NewPass2!", "Other999@", password.ErrPasswordUnconfirmed},
		{"success updates hash", "OldPass1!", "NewPass2!", "NewPass2!", nil},
	}
	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			st := &fakePasswordStore{u: user.User{ID: 7, PasswordHash: oldHash}}
			err := passwordchange.ChangePassword(context.Background(), st, 7, tc.current, tc.new, tc.confirm)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			} else if tc.name == "weak new password rejected" {
				if err == nil {
					t.Fatal("err = nil, want validation error")
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			switch tc.name {
			case "success updates hash":
				if st.setID != 7 || st.setHash == oldHash {
					t.Fatalf("SetPassword(%d, %q) not called with new hash", st.setID, st.setHash)
				}
				if !password.Compare(st.setHash, "NewPass2!") {
					t.Fatalf("stored hash %q does not match new password", st.setHash)
				}
			default:
				if st.setHash != "" {
					t.Fatalf("SetPassword called on rejected change")
				}
			}
		})
	}
}
