package twofactor

import (
	"context"
	"errors"
	"time"

	"github.com/shuvo-paul/baaku/internal/auth/user"
)

// Errors returned by the service flows.
var (
	ErrNotEnabled           = errors.New("twofactor: two factor authentication is not enabled")
	ErrNotConfirmed         = errors.New("twofactor: two factor authentication is not confirmed")
	ErrInvalidCode          = errors.New("twofactor: invalid two factor code")
	ErrPasswordNotConfirmed = errors.New("twofactor: password confirmation required")
)

// replayTTL matches Fortify's replay-guard cache TTL: (window ?: 1) * 60s.
const replayTTL = Window * 60 * time.Second

// Store is the persistence surface the service needs. *user.Repo satisfies
// GetByID and SetTwoFactor; *Repo adds the replay guard.
type Store interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	SetTwoFactor(ctx context.Context, id int64, secret, recoveryCodes string, confirmedAt *time.Time) error
}

// ReplayGuard claims a TOTP code for single use (Fortify's cache-backed
// verifyKeyNewer). Claim reports whether the code was newly claimed.
type ReplayGuard interface {
	Claim(ctx context.Context, code string, usedAt, expiresAt time.Time) (bool, error)
}

// PasswordConfirmation is implemented by the Task 09 confirm package
// (internal/auth/confirm): has the user confirmed their password recently?
type PasswordConfirmation interface {
	Confirmed(ctx context.Context, userID int64) bool
}

// Service orchestrates the 2FA flows: enable, confirm, and login challenge.
type Service struct {
	store   Store
	guard   ReplayGuard
	pc      PasswordConfirmation
	key     []byte // decoded 32-byte APP_KEY
	appName string // APP_NAME for the otpauth:// issuer
}

// NewService wires the service. key is the decoded APP_KEY (config.App.KeyBytes).
func NewService(store Store, guard ReplayGuard, pc PasswordConfirmation, key []byte, appName string) *Service {
	return &Service{store: store, guard: guard, pc: pc, key: key, appName: appName}
}

// EnableResult carries the one-time secrets shown to the user after enable.
type EnableResult struct {
	Secret        string   // base32 TOTP secret
	URI           string   // otpauth:// provisioning URI
	RecoveryCodes []string // plaintext recovery codes
}

// Enable stores a new unconfirmed TOTP secret with CodeCount encrypted
// recovery codes (Fortify EnableTwoFactorAuthentication). If a secret
// already exists, enable is a no-op and the stored state is returned, like
// Fortify re-rendering the QR/secret/recovery-code views from the user row.
func (s *Service) Enable(ctx context.Context, userID int64) (EnableResult, error) {
	u, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return EnableResult{}, err
	}
	if u.TwoFactorSecret != nil {
		return s.resultFor(u)
	}

	secret, err := GenerateSecret()
	if err != nil {
		return EnableResult{}, err
	}
	codes, err := NewRecoveryCodes()
	if err != nil {
		return EnableResult{}, err
	}
	encSecret, err := Encrypt(s.key, secret)
	if err != nil {
		return EnableResult{}, err
	}
	encCodes, err := EncryptCodes(s.key, codes)
	if err != nil {
		return EnableResult{}, err
	}
	if err := s.store.SetTwoFactor(ctx, userID, encSecret, encCodes, nil); err != nil {
		return EnableResult{}, err
	}
	return EnableResult{
		Secret:        secret,
		URI:           ProvisioningURI(s.appName, u.Email, secret),
		RecoveryCodes: codes,
	}, nil
}

// Confirm validates code against the stored secret and marks 2FA confirmed
// (Fortify ConfirmTwoFactorAuthentication). Requires a valid TOTP plus
// recent password confirmation via the Task 09 interface; the reference app
// enables fortify 'confirm' => true, so confirmation gates enablement.
func (s *Service) Confirm(ctx context.Context, userID int64, code string) error {
	if !s.pc.Confirmed(ctx, userID) {
		return ErrPasswordNotConfirmed
	}
	u, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.TwoFactorSecret == nil || code == "" {
		return ErrInvalidCode
	}
	secret, err := Decrypt(s.key, *u.TwoFactorSecret)
	if err != nil {
		return err
	}
	ok, err := s.verifyWithReplay(ctx, secret, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}
	now := time.Now().UTC()
	return s.store.SetTwoFactor(ctx, userID, *u.TwoFactorSecret, *u.TwoFactorRecoveryCodes, &now)
}

// Verify checks code during the login challenge (Fortify
// TwoFactorLoginController::store): a stored recovery code is accepted and
// consumed (replaced with a fresh code); anything else must be a valid TOTP.
func (s *Service) Verify(ctx context.Context, userID int64, code string) error {
	u, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if u.TwoFactorSecret == nil {
		return ErrNotEnabled
	}
	if u.TwoFactorConfirmedAt == nil {
		return ErrNotConfirmed
	}
	if code == "" {
		return ErrInvalidCode
	}

	codes, err := DecryptCodes(s.key, deref(u.TwoFactorRecoveryCodes))
	if err != nil {
		return err
	}
	for _, c := range codes {
		if c == code {
			updated, found, err := ReplaceRecoveryCode(s.key, deref(u.TwoFactorRecoveryCodes), code)
			if err != nil {
				return err
			}
			if !found {
				return ErrInvalidCode
			}
			return s.store.SetTwoFactor(ctx, userID, *u.TwoFactorSecret, updated, u.TwoFactorConfirmedAt)
		}
	}

	secret, err := Decrypt(s.key, *u.TwoFactorSecret)
	if err != nil {
		return err
	}
	ok, err := s.verifyWithReplay(ctx, secret, code)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInvalidCode
	}
	return nil
}

// HasEnabledTwoFactor reports whether 2FA is on for the user, mirroring
// TwoFactorAuthenticatable::hasEnabledTwoFactorAuthentication with the
// reference app's confirm option: secret AND confirmed_at required.
func (s *Service) HasEnabledTwoFactor(ctx context.Context, userID int64) (bool, error) {
	u, err := s.store.GetByID(ctx, userID)
	if err != nil {
		return false, err
	}
	return u.TwoFactorSecret != nil && u.TwoFactorConfirmedAt != nil, nil
}

// verifyWithReplay checks the TOTP code and claims it for single use within
// replayTTL (Fortify's verifyKeyNewer + cache). ponytail: the replay claim
// key is the bare code, exactly like Fortify's md5(code) cache key, so two
// users sharing the same code in the same window can false-reject; key on
// userID+code if that ever bites.
func (s *Service) verifyWithReplay(ctx context.Context, secret, code string) (bool, error) {
	now := time.Now()
	if !VerifyCode(secret, code, now) {
		return false, nil
	}
	claimed, err := s.guard.Claim(ctx, code, now, now.Add(replayTTL))
	if err != nil {
		return false, err
	}
	return claimed, nil
}

// resultFor decrypts the stored 2FA state into an EnableResult.
func (s *Service) resultFor(u user.User) (EnableResult, error) {
	secret, err := Decrypt(s.key, *u.TwoFactorSecret)
	if err != nil {
		return EnableResult{}, err
	}
	codes, err := DecryptCodes(s.key, deref(u.TwoFactorRecoveryCodes))
	if err != nil {
		return EnableResult{}, err
	}
	return EnableResult{
		Secret:        secret,
		URI:           ProvisioningURI(s.appName, u.Email, secret),
		RecoveryCodes: codes,
	}, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
