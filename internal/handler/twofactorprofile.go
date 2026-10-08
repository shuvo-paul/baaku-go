package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/views"
)

// 2FA management flash keys/values (Fortify TwoFactorAuthenticationController
// responses; the reference profile/show.blade.php reads them off the session).
const (
	TwoFactorEnabledFlash   = "two-factor-authentication-enabled"
	TwoFactorConfirmedFlash = "two-factor-authentication-confirmed"
	RecoveryCodesGenerated  = "two-factor-recovery-codes-generated"
	confirmationRequired    = "required"
)

// ProfilePagePath is the profile page (reference dashboard/profile — the
// security tab: password change + 2FA).
const ProfilePagePath = "/dashboard/profile"

// TwoFactorService is the 2FA management surface; *twofactor.Service satisfies it.
type TwoFactorService interface {
	Enable(ctx context.Context, userID int64) (twofactor.EnableResult, error)
	Confirm(ctx context.Context, userID int64, code string) error
	Disable(ctx context.Context, userID int64) error
	RegenerateRecoveryCodes(ctx context.Context, userID int64) ([]string, error)
	QRCodePNG(ctx context.Context, userID int64) ([]byte, error)
	SetupKey(ctx context.Context, userID int64) (string, error)
}

// TwoFactorSettings serves the Fortify two-factor management endpoints
// (twoFactorAuthentication with confirmPassword => true — every route sits
// behind auth + password.confirm in main.go).
type TwoFactorSettings struct {
	svc TwoFactorService
}

func NewTwoFactorSettings(svc TwoFactorService) *TwoFactorSettings {
	return &TwoFactorSettings{svc: svc}
}

// Enable handles POST /user/two-factor-authentication: enroll a TOTP secret
// and bounce back with the enable flash — status, confirmation='required',
// and the plaintext recovery codes (newline-joined in one flash value;
// Laravel keeps them as an array).
func (h *TwoFactorSettings) Enable(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	res, err := h.svc.Enable(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.SetFlashMany(r.Context(), map[string]string{
		"status":        TwoFactorEnabledFlash,
		"confirmation":  confirmationRequired,
		"recoveryCodes": strings.Join(res.RecoveryCodes, "\n"),
	})
	middleware.Redirect(w, r, backURL(r))
}

// Confirm handles POST /user/confirmed-two-factor-authentication (Fortify
// ConfirmedTwoFactorAuthenticationController; message from Fortify's default
// ConfirmTwoFactorAuthentication action).
func (h *TwoFactorSettings) Confirm(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	err := h.svc.Confirm(r.Context(), u.ID, strings.TrimSpace(r.FormValue("code")))
	switch {
	case errors.Is(err, twofactor.ErrInvalidCode):
		middleware.RedirectWithFlash(w, r, backURL(r), "error", "The provided two factor code was invalid.")
	case errors.Is(err, twofactor.ErrPasswordNotConfirmed):
		http.Error(w, "forbidden", http.StatusForbidden)
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	default:
		middleware.RedirectWithFlash(w, r, backURL(r), "status", TwoFactorConfirmedFlash)
	}
}

// Disable handles DELETE /user/two-factor-authentication (Fortify
// TwoFactorAuthenticationController@destroy).
func (h *TwoFactorSettings) Disable(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := h.svc.Disable(r.Context(), u.ID); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.Redirect(w, r, backURL(r))
}

// QRCode handles GET /user/two-factor-qr-code (Fortify TwoFactorQrCodeController):
// PNG of the user's provisioning URI.
func (h *TwoFactorSettings) QRCode(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	png, err := h.svc.QRCodePNG(r.Context(), u.ID)
	switch {
	case errors.Is(err, twofactor.ErrNotEnabled):
		http.NotFound(w, r)
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	default:
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}
}

// Regenerate handles POST /user/two-factor-recovery-codes (Fortify
// RecoveryCodeController@store). The reference blade flashes
// two-factor-recovery-codes-generated and shows no codes afterwards — parity
// kept.
func (h *TwoFactorSettings) Regenerate(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if _, err := h.svc.RegenerateRecoveryCodes(r.Context(), u.ID); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, backURL(r), "status", RecoveryCodesGenerated)
}

// ProfilePage renders GET /dashboard/profile (reference ProfileController +
// the security tab of profile/show.blade.php: the updatePasswords and
// twoFactorAuthentication cards; details/educations/careers tabs land in
// later waves).
type ProfilePage struct {
	svc     TwoFactorService
	appName string
}

func NewProfilePage(svc TwoFactorService, appName string) *ProfilePage {
	return &ProfilePage{svc: svc, appName: appName}
}

// Show renders the page. The 2FA section is flash-driven exactly like the
// reference blade: enable/confirm flashes carry the one-time display state;
// the setup key is read from the user row (the reference decrypts it during
// render).
func (h *ProfilePage) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	flash := middleware.FlashFromContext(r.Context())
	status, confirmation := flash["status"], flash["confirmation"]
	setupKey := ""
	if status == TwoFactorEnabledFlash && confirmation == confirmationRequired {
		key, err := h.svc.SetupKey(r.Context(), u.ID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		setupKey = key
	}
	views.ProfilePage(h.appName, middleware.TokenFromContext(r.Context()),
		status, confirmation, flash["recoveryCodes"], flash["error"], setupKey).Render(r.Context(), w)
}
