package emailverify

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/shuvo-paul/baaku/internal/mailer"
)

// ResendValidity is the signed-link window (Laravel VerifyEmail notification:
// temporarySignedRoute(..., now()->addMinutes(60), ...)).
const ResendValidity = 60 * time.Minute

// SendFunc delivers one email; wiring binds it to mailer.Send with the SMTP
// config. Tests swap in a capturing fake.
type SendFunc func(to, subject, htmlBody string) error

// Resender rebuilds and re-sends the signed email-verification link
// (Fortify GET /email/verify-notification).
type Resender struct {
	users  Store
	send   SendFunc
	appURL string
	key    []byte // raw APP_KEY string as bytes (Sign's key)
}

// NewResender wires the resend flow. key is the raw APP_KEY string as bytes.
func NewResender(users Store, send SendFunc, appURL string, key []byte) *Resender {
	return &Resender{users: users, send: send, appURL: appURL, key: key}
}

// Resend signs a fresh verification URL for the user and emails it.
func (r *Resender) Resend(ctx context.Context, userID int64) error {
	u, err := r.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/email/verify/%d/%s", u.ID, sha1hex(u.Email))
	signed := Sign(path, time.Now().Add(ResendValidity), r.key)
	subject, body, err := mailer.VerifyEmail(mailer.VerifyEmailData{
		Name: u.Name,
		URL:  r.appURL + signed,
	})
	if err != nil {
		return err
	}
	return r.send(u.Email, subject, body)
}

// sha1hex is the Laravel verification hash: sha1(user email).
func sha1hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}
