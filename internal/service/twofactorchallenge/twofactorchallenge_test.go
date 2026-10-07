package twofactorchallenge_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/repository/session"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/twofactorchallenge"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// fakes

type fakeSessions struct {
	byID     map[string]generated.Session
	promotes int
	lastID   string
	lastUID  int64
	creates  []generated.UpsertSessionParams
}

func (f *fakeSessions) Load(_ context.Context, id string) (generated.Session, error) {
	s, ok := f.byID[id]
	if !ok {
		return generated.Session{}, session.ErrNotFound
	}
	return s, nil
}

func (f *fakeSessions) Promote(_ context.Context, id string, userID int64, payload string) error {
	f.promotes++
	f.lastID, f.lastUID = id, userID
	uid := userID
	s := f.byID[id]
	s.UserID, s.Payload = &uid, payload
	f.byID[id] = s
	return nil
}

func (f *fakeSessions) Create(_ context.Context, p generated.UpsertSessionParams) error {
	f.creates = append(f.creates, p)
	return nil
}

type fakeVerifier struct {
	err   error
	codes []string
}

func (v *fakeVerifier) Verify(_ context.Context, _ int64, code string) error {
	v.codes = append(v.codes, code)
	return v.err
}

// harness

func pendingSession(id string, userID int64) generated.Session {
	return generated.Session{ID: id, Payload: twofactorchallenge.PendingPayload(userID)}
}

func newFixture(verifier twofactorchallenge.Verifier) (*twofactorchallenge.Service, *fakeSessions) {
	sessions := &fakeSessions{byID: map[string]generated.Session{
		"s1": pendingSession("s1", 7),
	}}
	return twofactorchallenge.New(sessions, verifier), sessions
}

// tests

func TestChallengeValidCodePromotesSession(t *testing.T) {
	verifier := &fakeVerifier{}
	svc, sessions := newFixture(verifier)

	if err := svc.Challenge(context.Background(), "s1", "123456"); err != nil {
		t.Fatalf("Challenge = %v, want nil", err)
	}
	if len(verifier.codes) != 1 || verifier.codes[0] != "123456" {
		t.Errorf("verifier codes = %v, want [123456]", verifier.codes)
	}
	s := sessions.byID["s1"]
	if s.UserID == nil || *s.UserID != 7 {
		t.Errorf("session user_id = %v, want 7", s.UserID)
	}
	if s.Payload != "{}" {
		t.Errorf("payload = %s, want {} (pending flag cleared)", s.Payload)
	}
}

func TestChallengeInvalidCodeLeavesSessionUnchanged(t *testing.T) {
	verifier := &fakeVerifier{err: twofactor.ErrInvalidCode}
	svc, sessions := newFixture(verifier)
	before := sessions.byID["s1"]

	if err := svc.Challenge(context.Background(), "s1", "000000"); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Errorf("Challenge = %v, want ErrInvalidCode", err)
	}
	if sessions.promotes != 0 {
		t.Errorf("Promote called %d times on invalid code, want 0", sessions.promotes)
	}
	after := sessions.byID["s1"]
	if after.UserID != before.UserID || after.Payload != before.Payload {
		t.Errorf("session changed on invalid code: %+v → %+v", before, after)
	}
}

func TestChallengeNoPendingSession(t *testing.T) {
	verifier := &fakeVerifier{}
	svc, sessions := newFixture(verifier)
	ctx := context.Background()

	// Missing row.
	if err := svc.Challenge(ctx, "missing", "123456"); !errors.Is(err, twofactorchallenge.ErrNoPendingChallenge) {
		t.Errorf("Challenge(missing) = %v, want ErrNoPendingChallenge", err)
	}
	// Already authenticated.
	uid := int64(7)
	sessions.byID["auth"] = generated.Session{ID: "auth", UserID: &uid, Payload: "{}"}
	if err := svc.Challenge(ctx, "auth", "123456"); !errors.Is(err, twofactorchallenge.ErrNoPendingChallenge) {
		t.Errorf("Challenge(authenticated) = %v, want ErrNoPendingChallenge", err)
	}
	// No pending flag in payload.
	sessions.byID["plain"] = generated.Session{ID: "plain", Payload: `{"flash":{"status":"x"}}`}
	if err := svc.Challenge(ctx, "plain", "123456"); !errors.Is(err, twofactorchallenge.ErrNoPendingChallenge) {
		t.Errorf("Challenge(no flag) = %v, want ErrNoPendingChallenge", err)
	}
	if len(verifier.codes) != 0 {
		t.Errorf("verifier called for non-pending sessions: %v", verifier.codes)
	}
}

func TestChallengePreservesOtherPayloadKeys(t *testing.T) {
	verifier := &fakeVerifier{}
	svc, sessions := newFixture(verifier)
	sessions.byID["s2"] = generated.Session{
		ID:      "s2",
		Payload: `{"flash":{"status":"code required"},"login.two_factor":7}`,
	}

	if err := svc.Challenge(context.Background(), "s2", "123456"); err != nil {
		t.Fatalf("Challenge = %v, want nil", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(sessions.byID["s2"].Payload), &payload); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if _, ok := payload["login.two_factor"]; ok {
		t.Error("pending flag survived promotion")
	}
	if payload["flash"] == nil {
		t.Error("flash bucket dropped during promotion")
	}
}

func TestPendingPayloadRoundTrip(t *testing.T) {
	if got := twofactorchallenge.PendingPayload(42); got != `{"login.two_factor":42}` {
		t.Errorf("PendingPayload(42) = %s", got)
	}
}

func TestBeginCreatesPendingSession(t *testing.T) {
	svc, sessions := newFixture(&fakeVerifier{})
	sid, err := svc.Begin(context.Background(), 7)
	if err != nil {
		t.Fatalf("Begin = %v, want nil", err)
	}
	if sid == "" {
		t.Fatal("Begin returned empty session id")
	}
	if len(sessions.creates) != 1 {
		t.Fatalf("Create called %d times, want 1", len(sessions.creates))
	}
	p := sessions.creates[0]
	if p.ID != sid {
		t.Errorf("created row id = %q, want %q", p.ID, sid)
	}
	if p.UserID != nil {
		t.Errorf("pending session user_id = %v, want NULL (unguarded guest until Challenge)", p.UserID)
	}
	if want := `{"login.two_factor":7}`; p.Payload != want {
		t.Errorf("payload = %s, want %s", p.Payload, want)
	}
}

// integration: real twofactor.Service — recovery code consumed exactly once.

type memUserStore struct{ users map[int64]user.User }

func (m *memUserStore) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := m.users[id]
	if !ok {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

func (m *memUserStore) SetTwoFactor(_ context.Context, id int64, secret, codes string, confirmedAt *time.Time) error {
	u := m.users[id]
	s, c := secret, codes
	u.TwoFactorSecret, u.TwoFactorRecoveryCodes, u.TwoFactorConfirmedAt = &s, &c, confirmedAt
	m.users[id] = u
	return nil
}

func (m *memUserStore) ClearTwoFactor(_ context.Context, id int64) error {
	u := m.users[id]
	u.TwoFactorSecret, u.TwoFactorRecoveryCodes, u.TwoFactorConfirmedAt = nil, nil, nil
	m.users[id] = u
	return nil
}

type memGuard struct{ claimed map[string]bool }

func (g *memGuard) Claim(_ context.Context, code string, _, _ time.Time) (bool, error) {
	if g.claimed[code] {
		return false, nil
	}
	g.claimed[code] = true
	return true, nil
}

type alwaysConfirmed struct{}

func (alwaysConfirmed) Confirmed(context.Context, int64) bool { return true }

func testKey() []byte {
	key := make([]byte, 32)
	for i := range key {
		key[i] = 0x42
	}
	return key
}

func TestChallengeRecoveryCodeConsumedExactlyOnce(t *testing.T) {
	ctx := context.Background()
	key := testKey()

	secret, err := twofactor.GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	codes, err := twofactor.NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	encSecret, err := twofactor.Encrypt(key, secret)
	if err != nil {
		t.Fatal(err)
	}
	encCodes, err := twofactor.EncryptCodes(key, codes)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	users := &memUserStore{users: map[int64]user.User{
		7: {ID: 7, TwoFactorSecret: &encSecret, TwoFactorRecoveryCodes: &encCodes, TwoFactorConfirmedAt: &now},
	}}
	tfa := twofactor.NewService(users, &memGuard{claimed: map[string]bool{}}, alwaysConfirmed{}, key, "Baaku")

	sessions := &fakeSessions{byID: map[string]generated.Session{
		"pending": pendingSession("pending", 7),
	}}
	svc := twofactorchallenge.New(sessions, tfa)

	if err := svc.Challenge(ctx, "pending", codes[0]); err != nil {
		t.Fatalf("Challenge(recovery code) = %v, want nil", err)
	}
	if sessions.byID["pending"].UserID == nil {
		t.Fatal("session not authenticated after recovery code")
	}

	// The consumed code must be gone: a new pending session retrying it fails.
	sessions.byID["retry"] = pendingSession("retry", 7)
	if err := svc.Challenge(ctx, "retry", codes[0]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Errorf("Challenge(reused recovery code) = %v, want ErrInvalidCode", err)
	}
	if sessions.byID["retry"].UserID != nil {
		t.Error("session authenticated with reused recovery code")
	}
}
