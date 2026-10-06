package emailverify

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/auth/user"
)

// testKey mimics a real APP_KEY, "base64:"-prefixed and passed to the HMAC
// verbatim, exactly as UrlGenerator's key resolver does.
var testKey = []byte("base64:secret")

func TestSignVerifyRoundtrip(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	signed := Sign("/email/verify/5/abc123", time.Unix(expires, 0), testKey)

	path, rest, ok := strings.Cut(signed, "?expires=")
	if !ok {
		t.Fatalf("signed URL missing expires: %s", signed)
	}
	expStr, sig, ok := strings.Cut(rest, "&signature=")
	if !ok {
		t.Fatalf("signed URL missing signature: %s", signed)
	}
	expires, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		t.Fatalf("expires not a unix timestamp: %q", expStr)
	}
	if err := Verify(path, expires, sig, testKey); err != nil {
		t.Errorf("Verify(roundtrip) = %v, want nil", err)
	}
}

// TestSignLaravelFormat pins the wire format against an HMAC computed by
// openssl over the exact string Laravel signs: "path?expires=<unix>" with the
// raw APP_KEY as key, hex output. If this breaks, links are no longer
// Laravel-compatible.
func TestSignLaravelFormat(t *testing.T) {
	const wantSig = "62a727db5d9f10886f07ae3f87c97ef899a9bff61ac6a68554f190eaafd2da06"
	got := Sign("/email/verify/5/abc123", time.Unix(1700000000, 0), testKey)
	const want = "/email/verify/5/abc123?expires=1700000000&signature=" + wantSig
	if got != want {
		t.Errorf("Sign() =\n %s\nwant\n %s", got, want)
	}
}

func TestVerifyTamperedPath(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	signed := Sign("/email/verify/5/abc123", time.Unix(expires, 0), testKey)
	_, rest, _ := strings.Cut(signed, "?expires=")
	expStr, sig, _ := strings.Cut(rest, "&signature=")

	err := Verify("/email/verify/99/abc123", mustParseInt64(t, expStr), sig, testKey)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify(tampered path) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifyTamperedExpires(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	signed := Sign("/email/verify/5/abc123", time.Unix(expires, 0), testKey)
	_, rest, _ := strings.Cut(signed, "?expires=")
	_, sig, _ := strings.Cut(rest, "&signature=")

	err := Verify("/email/verify/5/abc123", expires+1000, sig, testKey)
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify(tampered expires) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifyWrongKey(t *testing.T) {
	expires := time.Now().Add(time.Hour).Unix()
	signed := Sign("/email/verify/5/abc123", time.Unix(expires, 0), testKey)
	_, rest, _ := strings.Cut(signed, "?expires=")
	expStr, sig, _ := strings.Cut(rest, "&signature=")

	err := Verify("/email/verify/5/abc123", mustParseInt64(t, expStr), sig, []byte("base64:other"))
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("Verify(wrong key) = %v, want ErrInvalidSignature", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	expires := time.Now().Add(-time.Minute).Unix()
	signed := Sign("/email/verify/5/abc123", time.Unix(expires, 0), testKey)
	_, rest, _ := strings.Cut(signed, "?expires=")
	expStr, sig, _ := strings.Cut(rest, "&signature=")

	err := Verify("/email/verify/5/abc123", mustParseInt64(t, expStr), sig, testKey)
	if !errors.Is(err, ErrExpired) {
		t.Errorf("Verify(expired) = %v, want ErrExpired", err)
	}
}

// fakeStore records writes and serves one canned user.
type fakeStore struct {
	u           user.User
	verifiedID  int64
	stateID     int64
	state       user.UserState
	verifiedSet bool
	getErr      error
}

func (f *fakeStore) GetByID(context.Context, int64) (user.User, error) {
	return f.u, f.getErr
}

func (f *fakeStore) SetEmailVerified(_ context.Context, id int64) error {
	f.verifiedID = id
	f.verifiedSet = true
	return nil
}

func (f *fakeStore) UpdateState(_ context.Context, id int64, state user.UserState) error {
	f.stateID = id
	f.state = state
	return nil
}

// Mirrors MarkUserPendingOnVerification: unverified → pending + verified_at set.
func TestMarkVerifiedTransitionsUnverifiedToPending(t *testing.T) {
	f := &fakeStore{u: user.User{ID: 5, State: user.StateUnverified}}

	if err := MarkVerified(context.Background(), f, 5); err != nil {
		t.Fatal(err)
	}
	if !f.verifiedSet || f.verifiedID != 5 {
		t.Errorf("email not verified: set=%v id=%d", f.verifiedSet, f.verifiedID)
	}
	if f.state != user.StatePending || f.stateID != 5 {
		t.Errorf("state = %s (id %d), want pending (id 5)", f.state, f.stateID)
	}
}

// The listener only fires the transition when state is unverified; active users
// keep their state even though verified_at is refreshed.
func TestMarkVerifiedLeavesOtherStates(t *testing.T) {
	for _, state := range []user.UserState{user.StateActive, user.StatePending, user.StateSuspended, user.StateRejected} {
		f := &fakeStore{u: user.User{ID: 5, State: state}}
		if err := MarkVerified(context.Background(), f, 5); err != nil {
			t.Fatalf("state %s: %v", state, err)
		}
		if !f.verifiedSet {
			t.Errorf("state %s: email not verified", state)
		}
		if f.state != "" {
			t.Errorf("state %s: UpdateState called with %s, want no call", state, f.state)
		}
	}
}

// Controller redirects without writing when the email is already verified.
func TestMarkVerifiedNoopWhenAlreadyVerified(t *testing.T) {
	now := time.Now()
	f := &fakeStore{u: user.User{ID: 5, State: user.StatePending, EmailVerifiedAt: &now}}

	if err := MarkVerified(context.Background(), f, 5); err != nil {
		t.Fatal(err)
	}
	if f.verifiedSet || f.state != "" {
		t.Errorf("writes happened on already-verified user: verified=%v state=%q", f.verifiedSet, f.state)
	}
}

func mustParseInt64(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatalf("not a number: %q", s)
	}
	return n
}
