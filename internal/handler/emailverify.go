package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/emailverify"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// VerificationLinkSent is the 'verification-link-sent' flash value (Fortify
// VERIFICATION_LINK_SENT); the notice page maps it to the lang line below.
const VerificationLinkSent = "verification-link-sent"

// VerificationResentMessage is auth.verify_email_sent.
const VerificationResentMessage = "A new verification link has been sent to the email address you provided during registration."

// Verifier is the signed-URL + persistence slice email verification needs.
// *user.Repo satisfies it.
type Verifier interface {
	GetByID(ctx context.Context, id int64) (user.User, error)
	SetEmailVerified(ctx context.Context, id int64) error
	UpdateState(ctx context.Context, id int64, state user.UserState) error
}

// Resender resends the signed link (*emailverify.Resender).
type Resender interface {
	Resend(ctx context.Context, userID int64) error
}

// EmailVerify handles the verification notice, the signed verify URL, and
// the resend endpoint (Fortify verification.notice / verification.verify /
// verification.send).
type EmailVerify struct {
	users    Verifier
	resender Resender
	key      []byte // raw APP_KEY string as bytes (emailverify.Sign's key)
	appName  string
}

func NewEmailVerify(users Verifier, resender Resender, key []byte, appName string) *EmailVerify {
	return &EmailVerify{users: users, resender: resender, key: key, appName: appName}
}

// Notice renders GET /email/verify (auth-required prompt).
func (h *EmailVerify) Notice(w http.ResponseWriter, r *http.Request) {
	status := ""
	if flash := middleware.FlashFromContext(r.Context()); flash["status"] == VerificationLinkSent {
		status = VerificationResentMessage
	}
	views.VerifyEmailPage(h.appName, middleware.TokenFromContext(r.Context()), status).Render(r.Context(), w)
}

// Verify handles GET /email/verify/{id}/{hash} (signed, auth). The signature
// covers "path?expires=N" keyed by the raw APP_KEY string, so {id}/{hash}
// cannot be forged. Fortify's VerifyEmailController marks the authenticated
// user — the URL segments ride along inside the signed path.
func (h *EmailVerify) Verify(w http.ResponseWriter, r *http.Request) {
	if _, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64); err != nil {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	expires, err := strconv.ParseInt(q.Get("expires"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := emailverify.Verify(r.URL.Path, expires, q.Get("signature"), h.key); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := emailverify.MarkVerified(r.Context(), h.users, u.ID); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.Redirect(w, r, intendedDest(r, middleware.DashboardPath))
}

// Resend handles POST /email/verification-notification (auth).
func (h *EmailVerify) Resend(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := h.resender.Resend(r.Context(), u.ID); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/email/verify", "status", VerificationLinkSent)
}
