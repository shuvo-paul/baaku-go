package twofactor

import (
	"context"
	"encoding/base32"
	"errors"
	"testing"
	"time"

	"github.com/shuvo-paul/baaku/internal/auth/user"
)

// fakes

type fakeStore struct {
	users    map[int64]user.User
	setCalls int
}

func (f *fakeStore) GetByID(_ context.Context, id int64) (user.User, error) {
	u, ok := f.users[id]
	if !ok {
		return user.User{}, errors.New("user not found")
	}
	return u, nil
}

func (f *fakeStore) SetTwoFactor(_ context.Context, id int64, secret, codes string, confirmedAt *time.Time) error {
	u := f.users[id]
	sec, cod := secret, codes
	u.TwoFactorSecret, u.TwoFactorRecoveryCodes, u.TwoFactorConfirmedAt = &sec, &cod, confirmedAt
	f.users[id] = u
	f.setCalls++
	return nil
}

type fakeGuard struct{ claimed map[string]bool }

func (g *fakeGuard) Claim(_ context.Context, code string, _, _ time.Time) (bool, error) {
	if g.claimed[code] {
		return false, nil
	}
	g.claimed[code] = true
	return true, nil
}

type fakePC struct{ ok bool }

func (p fakePC) Confirmed(context.Context, int64) bool { return p.ok }

// harness

type harness struct {
	svc   *Service
	store *fakeStore
	guard *fakeGuard
}

func newHarness(t *testing.T, pc bool) *harness {
	t.Helper()
	store := &fakeStore{users: map[int64]user.User{
		1: {ID: 1, Email: "user@example.com"},
	}}
	guard := &fakeGuard{claimed: map[string]bool{}}
	svc := NewService(store, guard, fakePC{pc}, testKey(), "Baaku")
	return &harness{svc: svc, store: store, guard: guard}
}

func totpAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	return hotp(key, at.Unix()/Step)
}

func mustNotCode(t *testing.T, secret string) string {
	t.Helper()
	for _, c := range []string{"000000", "111111", "222222", "999999"} {
		if !VerifyCode(secret, c, time.Now()) {
			return c
		}
	}
	t.Fatal("no invalid code found")
	return ""
}

// tests

func TestEnableStoresUnconfirmedState(t *testing.T) {
	h := newHarness(t, false)
	res, err := h.svc.Enable(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RecoveryCodes) != CodeCount {
		t.Errorf("got %d recovery codes, want %d", len(res.RecoveryCodes), CodeCount)
	}
	wantURI := "otpauth://totp/Baaku:user@example.com?secret=" + res.Secret + "&issuer=Baaku"
	if res.URI != wantURI {
		t.Errorf("URI = %q, want %q", res.URI, wantURI)
	}

	u := h.store.users[1]
	if u.TwoFactorSecret == nil || u.TwoFactorRecoveryCodes == nil {
		t.Fatal("secret/codes not stored")
	}
	if u.TwoFactorConfirmedAt != nil {
		t.Error("confirmed_at set on enable, want nil (unconfirmed)")
	}
	secret, err := Decrypt(testKey(), *u.TwoFactorSecret)
	if err != nil {
		t.Fatal(err)
	}
	if secret != res.Secret {
		t.Error("stored secret differs from returned secret")
	}
	codes, err := DecryptCodes(testKey(), *u.TwoFactorRecoveryCodes)
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range codes {
		if c != res.RecoveryCodes[i] {
			t.Errorf("stored code %d = %q, want %q", i, c, res.RecoveryCodes[i])
		}
	}
}

func TestEnableIsIdempotent(t *testing.T) {
	h := newHarness(t, false)
	first, err := h.svc.Enable(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.svc.Enable(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if second.Secret != first.Secret {
		t.Error("second enable regenerated the secret; Fortify no-ops when a secret exists")
	}
	if h.store.setCalls != 1 {
		t.Errorf("SetTwoFactor called %d times, want 1", h.store.setCalls)
	}
}

func TestConfirmRequiresPasswordConfirmation(t *testing.T) {
	h := newHarness(t, false)
	if _, err := h.svc.Enable(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	err := h.svc.Confirm(context.Background(), 1, "123456")
	if !errors.Is(err, ErrPasswordNotConfirmed) {
		t.Fatalf("Confirm = %v, want ErrPasswordNotConfirmed", err)
	}
	if h.store.users[1].TwoFactorConfirmedAt != nil {
		t.Error("confirmed_at set despite missing password confirmation")
	}
}

func TestConfirmSuccessAndReplayRejected(t *testing.T) {
	h := newHarness(t, true)
	res, err := h.svc.Enable(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	code := totpAt(t, res.Secret, time.Now())
	if err := h.svc.Confirm(context.Background(), 1, code); err != nil {
		t.Fatalf("Confirm(%s) = %v, want nil", code, err)
	}
	if h.store.users[1].TwoFactorConfirmedAt == nil {
		t.Fatal("confirmed_at not set after valid confirm")
	}
	if err := h.svc.Confirm(context.Background(), 1, code); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replayed code accepted: Confirm = %v, want ErrInvalidCode", err)
	}
}

func TestConfirmInvalidCode(t *testing.T) {
	h := newHarness(t, true)
	res, err := h.svc.Enable(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	bad := mustNotCode(t, res.Secret)
	if err := h.svc.Confirm(context.Background(), 1, bad); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("Confirm(%s) = %v, want ErrInvalidCode", bad, err)
	}
	if h.store.users[1].TwoFactorConfirmedAt != nil {
		t.Error("confirmed_at set on invalid code")
	}
}

func TestConfirmWithoutSecret(t *testing.T) {
	h := newHarness(t, true)
	if err := h.svc.Confirm(context.Background(), 1, "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("Confirm without enable = %v, want ErrInvalidCode", err)
	}
}

// seedConfirmed loads a confirmed 2FA user directly and returns the secret.
func seedConfirmed(t *testing.T, h *harness) string {
	t.Helper()
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	codes, err := NewRecoveryCodes()
	if err != nil {
		t.Fatal(err)
	}
	encSecret, err := Encrypt(testKey(), secret)
	if err != nil {
		t.Fatal(err)
	}
	encCodes, err := EncryptCodes(testKey(), codes)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	u := h.store.users[1]
	u.TwoFactorSecret, u.TwoFactorRecoveryCodes, u.TwoFactorConfirmedAt = &encSecret, &encCodes, &now
	h.store.users[1] = u
	return secret
}

func TestVerifyRecoveryCodeConsumesOnce(t *testing.T) {
	h := newHarness(t, false)
	seedConfirmed(t, h)
	ctx := context.Background()
	codes, err := DecryptCodes(testKey(), *h.store.users[1].TwoFactorRecoveryCodes)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.svc.Verify(ctx, 1, codes[0]); err != nil {
		t.Fatalf("Verify(recovery code) = %v, want nil", err)
	}
	stored, err := DecryptCodes(testKey(), *h.store.users[1].TwoFactorRecoveryCodes)
	if err != nil {
		t.Fatal(err)
	}
	orig := map[string]bool{}
	for _, c := range codes {
		orig[c] = true
	}
	var replacement string
	for _, c := range stored {
		if !orig[c] {
			replacement = c
		}
	}
	if replacement == "" {
		t.Fatal("used recovery code was not replaced with a fresh one")
	}

	if err := h.svc.Verify(ctx, 1, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("reused recovery code accepted: Verify = %v, want ErrInvalidCode", err)
	}
	if err := h.svc.Verify(ctx, 1, replacement); err != nil {
		t.Errorf("Verify(replacement code) = %v, want nil", err)
	}
}

func TestVerifyTOTPAndReplayRejected(t *testing.T) {
	h := newHarness(t, false)
	secret := seedConfirmed(t, h)
	code := totpAt(t, secret, time.Now())
	if err := h.svc.Verify(context.Background(), 1, code); err != nil {
		t.Fatalf("Verify(TOTP) = %v, want nil", err)
	}
	if err := h.svc.Verify(context.Background(), 1, code); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replayed TOTP accepted: Verify = %v, want ErrInvalidCode", err)
	}
}

func TestVerifyGuards(t *testing.T) {
	h := newHarness(t, false)
	ctx := context.Background()
	if err := h.svc.Verify(ctx, 1, "123456"); !errors.Is(err, ErrNotEnabled) {
		t.Errorf("Verify without enable = %v, want ErrNotEnabled", err)
	}

	secret, _ := GenerateSecret()
	enc, _ := Encrypt(testKey(), secret)
	u := h.store.users[1]
	u.TwoFactorSecret = &enc
	h.store.users[1] = u
	if err := h.svc.Verify(ctx, 1, "123456"); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("Verify unconfirmed = %v, want ErrNotConfirmed", err)
	}

	confirmed := time.Now()
	u.TwoFactorConfirmedAt = &confirmed
	h.store.users[1] = u
	if err := h.svc.Verify(ctx, 1, ""); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("Verify empty code = %v, want ErrInvalidCode", err)
	}
}

func TestHasEnabledTwoFactor(t *testing.T) {
	h := newHarness(t, false)
	ctx := context.Background()
	if ok, _ := h.svc.HasEnabledTwoFactor(ctx, 1); ok {
		t.Error("enabled for user without 2FA")
	}
	secret, _ := GenerateSecret()
	enc, _ := Encrypt(testKey(), secret)
	u := h.store.users[1]
	u.TwoFactorSecret = &enc
	h.store.users[1] = u
	if ok, _ := h.svc.HasEnabledTwoFactor(ctx, 1); ok {
		t.Error("enabled for unconfirmed secret (reference has confirm => true)")
	}
	confirmed := time.Now()
	u.TwoFactorConfirmedAt = &confirmed
	h.store.users[1] = u
	if ok, _ := h.svc.HasEnabledTwoFactor(ctx, 1); !ok {
		t.Error("not enabled for secret + confirmed_at")
	}
}
