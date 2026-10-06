// Package emailverify provides Laravel-compatible signed email-verification
// URLs and the post-verification state transition. The signature format
// mirrors Illuminate\Routing\UrlGenerator (laravel/framework v13.30.0): an
// HMAC-SHA256 over "path?expires=<unix>", keyed by the raw APP_KEY string
// (including any "base64:" prefix — the key resolver passes config verbatim),
// hex-encoded. Note: hex, not base64, despite what older task notes said.
package emailverify

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/shuvo-paul/baaku/internal/auth/user"
)

var (
	// ErrInvalidSignature is returned when the signature does not match the
	// path and expires timestamp.
	ErrInvalidSignature = errors.New("emailverify: invalid signature")
	// ErrExpired is returned when the expires timestamp is in the past.
	ErrExpired = errors.New("emailverify: signed URL expired")
)

// Store is the user persistence MarkVerified needs. *user.Repo satisfies it.
type Store interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	SetEmailVerified(ctx context.Context, id int64) error
	UpdateState(ctx context.Context, id int64, state user.UserState) error
}

var _ Store = (*user.Repo)(nil) // wiring must keep satisfying this

// Sign returns path with expires and signature query params appended:
//
//	/email/verify/5/abc?expires=1700000000&signature=<hex>
//
// key is the raw APP_KEY string as bytes.
func Sign(path string, expires time.Time, key []byte) string {
	unsigned := canonical(path, expires.Unix())
	return unsigned + "&signature=" + sign(unsigned, key)
}

// Verify reports whether the signature over path?expires=<expires> matches key
// and the expiry has not passed. Signature is checked first, matching
// UrlGenerator::hasValidSignature.
func Verify(path string, expires int64, signature string, key []byte) error {
	unsigned := canonical(path, expires)
	if !hmac.Equal([]byte(sign(unsigned, key)), []byte(signature)) {
		return ErrInvalidSignature
	}
	if time.Now().Unix() > expires {
		return ErrExpired
	}
	return nil
}

// MarkVerified mirrors MarkUserPendingOnVerification: it records
// email_verified_at, then — only when the user is still unverified — moves
// them to pending for the admin approval queue. The two writes are separate
// statements, exactly as in the reference (no transaction there either).
func MarkVerified(ctx context.Context, store Store, id int64) error {
	u, err := store.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("emailverify: get user: %w", err)
	}
	if u.EmailVerifiedAt != nil {
		return nil // controller no-ops on already-verified users
	}
	if err := store.SetEmailVerified(ctx, id); err != nil {
		return fmt.Errorf("emailverify: set email verified: %w", err)
	}
	if u.State == user.StateUnverified {
		if err := store.UpdateState(ctx, id, user.StatePending); err != nil {
			return fmt.Errorf("emailverify: set pending: %w", err)
		}
	}
	return nil
}

// canonical is the exact string Laravel signs: the path plus the expires
// param (the only query param besides signature on verification URLs).
func canonical(path string, expires int64) string {
	return path + "?expires=" + strconv.FormatInt(expires, 10)
}

func sign(unsigned string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(unsigned))
	return hex.EncodeToString(mac.Sum(nil))
}
