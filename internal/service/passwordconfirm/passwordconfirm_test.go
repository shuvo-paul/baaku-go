package passwordconfirm_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/repository/confirm"
	"github.com/shuvo-paul/baaku/internal/service/password"
	"github.com/shuvo-paul/baaku/internal/service/passwordconfirm"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeUsers struct{ users map[int64]user.User }

func (f *fakeUsers) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := f.users[id]
	if !ok {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

// fakeConfirms mirrors the repo's TTL semantics with an injectable clock so
// the expiry path runs through the same confirm.ValidAt the repo uses.
type fakeConfirms struct {
	markedAt map[int64]time.Time
	ttl      time.Duration
	now      func() time.Time
}

func (f *fakeConfirms) MarkConfirmed(_ context.Context, userID int64) error {
	if f.markedAt == nil {
		f.markedAt = map[int64]time.Time{}
	}
	f.markedAt[userID] = f.now()
	return nil
}

func (f *fakeConfirms) Confirmed(_ context.Context, userID int64) bool {
	at, ok := f.markedAt[userID]
	if !ok {
		return false
	}
	return confirm.ValidAt(at.Add(f.ttl).Unix(), f.now().Unix())
}

// harness

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newFixture(t *testing.T, ttl time.Duration) (*passwordconfirm.Service, *fakeUsers, *fakeConfirms, *clock) {
	hash, err := password.Hash("correct-horse1!")
	if err != nil {
		t.Fatal(err)
	}
	users := &fakeUsers{users: map[int64]user.User{1: {ID: 1, PasswordHash: hash}}}
	clk := &clock{t: time.Unix(1_700_000_000, 0)}
	confirms := &fakeConfirms{ttl: ttl, now: clk.now}
	return passwordconfirm.New(users, confirms), users, confirms, clk
}

// tests

func TestConfirmThenCheckTrue(t *testing.T) {
	svc, _, _, _ := newFixture(t, 3*time.Hour)
	ctx := context.Background()

	if err := svc.Confirm(ctx, 1, "correct-horse1!"); err != nil {
		t.Fatalf("Confirm(valid) = %v, want nil", err)
	}
	if !svc.Check(ctx, 1) {
		t.Error("Check after Confirm = false, want true")
	}
}

func TestConfirmWrongPassword(t *testing.T) {
	svc, _, confirms, _ := newFixture(t, 3*time.Hour)
	ctx := context.Background()

	if err := svc.Confirm(ctx, 1, "wrong-password9!"); !errors.Is(err, passwordconfirm.ErrInvalidPassword) {
		t.Errorf("Confirm(wrong) = %v, want ErrInvalidPassword", err)
	}
	if len(confirms.markedAt) != 0 {
		t.Error("confirmation recorded despite wrong password")
	}
	if svc.Check(ctx, 1) {
		t.Error("Check after failed Confirm = true, want false")
	}
}

func TestCheckExpiresAfterTTL(t *testing.T) {
	svc, _, _, clk := newFixture(t, 3*time.Hour)
	ctx := context.Background()

	if err := svc.Confirm(ctx, 1, "correct-horse1!"); err != nil {
		t.Fatalf("Confirm = %v, want nil", err)
	}
	clk.t = clk.t.Add(3*time.Hour - time.Second)
	if !svc.Check(ctx, 1) {
		t.Error("Check inside TTL = false, want true")
	}
	clk.t = clk.t.Add(2 * time.Second)
	if svc.Check(ctx, 1) {
		t.Error("Check after TTL = true, want false")
	}
}

func TestConfirmUnknownUser(t *testing.T) {
	svc, _, _, _ := newFixture(t, 3*time.Hour)
	if err := svc.Confirm(context.Background(), 99, "correct-horse1!"); err == nil {
		t.Error("Confirm(unknown user) = nil, want error")
	}
}
