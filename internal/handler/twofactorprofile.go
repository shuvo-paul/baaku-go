package handler

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/repository/profile"
	"github.com/shuvo-paul/baaku/internal/service/career"
	"github.com/shuvo-paul/baaku/internal/service/education"
	"github.com/shuvo-paul/baaku/internal/service/twofactor"
	"github.com/shuvo-paul/baaku/internal/service/user"
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

// ProfileLoader reads the full profile row for the details tab.
type ProfileLoader interface {
	Get(ctx context.Context, userID int64) (profile.Profile, error)
}

// EducationLister lists the user's educations.
type EducationLister interface {
	List(ctx context.Context, userID int64) ([]education.Education, error)
}

// CareerLister lists the user's careers.
type CareerLister interface {
	List(ctx context.Context, userID int64) ([]career.Career, error)
}

// ProfilePage renders GET /dashboard/profile (reference ProfileController +
// profile/show.blade.php: details, education, career and security tabs).
type ProfilePage struct {
	svc        TwoFactorService
	profiles   ProfileLoader
	educations EducationLister
	careers    CareerLister
	cfg        *config.Config
	appName    string
}

func NewProfilePage(svc TwoFactorService, profiles ProfileLoader, educations EducationLister, careers CareerLister, cfg *config.Config, appName string) *ProfilePage {
	return &ProfilePage{svc: svc, profiles: profiles, educations: educations, careers: careers, cfg: cfg, appName: appName}
}

// Show renders the page. The 2FA section is flash-driven exactly like the
// reference blade; the details/education/career tabs load their rows fresh.
func (h *ProfilePage) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	ctx := r.Context()
	flash := middleware.FlashFromContext(r.Context())
	status, confirmation := flash["status"], flash["confirmation"]
	setupKey := ""
	if status == TwoFactorEnabledFlash && confirmation == confirmationRequired {
		key, err := h.svc.SetupKey(ctx, u.ID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		setupKey = key
	}

	var phone string
	if u.Phone != nil {
		phone = *u.Phone
	}
	d := views.ProfilePageData{
		AppName:   h.appName,
		CSRFToken: middleware.TokenFromContext(r.Context()),
		Sidebar: views.NewSidebarData(
			h.appName, middleware.TokenFromContext(r.Context()),
			u.Name, u.Email, "/dashboard/profile", u.State == user.StateSuspended,
		),
		User:            views.ProfileUser{Name: u.Name, Email: u.Email, Phone: phone},
		EmploymentTypes: toViewOptions(h.cfg.Career.EmploymentTypes),
		Levels:          h.cfg.Education.Levels,
		Institutions:    h.cfg.Education.Institutions,
		Subjects:        h.cfg.Education.Subjects,
		Status:          status,
		ErrMsg:          flash["error"],
		Confirmation:    confirmation,
		RecoveryCodes:   flash["recoveryCodes"],
		SetupKey:        setupKey,
	}

	// Profile row (missing row renders an empty form — the upsert creates it).
	if p, err := h.profiles.Get(ctx, u.ID); err == nil {
		d.Profile = views.ProfileData{
			PhotoURL:         photoURL(p.PhotoPath),
			DateOfBirth:      p.DateOfBirth,
			Gender:           deref(p.Gender),
			BloodGroup:       deref(p.BloodGroup),
			PresentAddress:   deref(p.PresentAddress),
			PermanentAddress: deref(p.PermanentAddress),
			Website:          deref(p.Website),
			SocialLinks:      p.SocialLinks,
			EmergencyContact: p.EmergencyContact,
			LocalNames:       p.LocalNames,
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// Local-name inputs render whenever configured, even before a profile row
	// exists (reference reads config('app.local_names'), not the row).
	for _, ln := range h.cfg.App.LocalNames {
		d.LocalNames = append(d.LocalNames, views.LocalNameField{
			Code: ln.Code, Label: ln.Label, Required: ln.Required,
			Value: d.Profile.LocalNames[ln.Code],
		})
	}

	if eds, err := h.educations.List(ctx, u.ID); err == nil {
		for _, e := range eds {
			d.Educations = append(d.Educations, views.EducationView{
				ID: e.ID, Level: e.Level, Institution: e.Institution,
				StudentID: deref(e.StudentID), Subject: e.Subject, IsCurrent: e.IsCurrent,
				StartYear: e.StartYear, StartMonth: derefI16(e.StartMonth),
				EndYear: derefI32(e.EndYear), EndMonth: derefI16(e.EndMonth),
			})
		}
	} else {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if cars, err := h.careers.List(ctx, u.ID); err == nil {
		for _, c := range cars {
			d.Careers = append(d.Careers, views.CareerView{
				ID: c.ID, EmploymentType: c.EmploymentType, JobTitle: c.JobTitle,
				Company: c.Company, Industry: deref(c.Industry), Location: deref(c.Location),
				StartYear: c.StartYear, StartMonth: derefI16(c.StartMonth), IsCurrent: c.IsCurrent,
				EndYear: derefI32(c.EndYear), EndMonth: derefI16(c.EndMonth), Description: deref(c.Description),
			})
		}
	} else {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	views.ProfilePage(d).Render(ctx, w)
}

// photoURL maps a stored photo_path to its streamed media URL ("" when none).
func photoURL(path *string) string {
	if path == nil || *path == "" {
		return ""
	}
	return "/media/profile-photos/" + filepath.Base(*path)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefI16(p *int16) int16 {
	if p == nil {
		return 0
	}
	return *p
}

func derefI32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

// toViewOptions converts config select options to view options.
func toViewOptions(in []config.Option) []views.Option {
	out := make([]views.Option, 0, len(in))
	for _, o := range in {
		out = append(out, views.Option{Value: o.Value, Label: o.Label})
	}
	return out
}
