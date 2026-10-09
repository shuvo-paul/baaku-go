package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/profiledetails"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// maxPhotoBytes caps the upload (reference 'max:2048' → 2048 KB).
const maxPhotoBytes = 2048 * 1024

// ProfileDetailsService is the details-update surface;
// *profiledetails.Service satisfies it.
type ProfileDetailsService interface {
	Update(ctx context.Context, userID int64, in profiledetails.Input) error
	PhotoPathOf(ctx context.Context, userID int64) (string, error)
}

// ProfileDetails serves PUT /dashboard/profile/details (reference
// ProfileDetailsController@update).
type ProfileDetails struct {
	svc      ProfileDetailsService
	cfg      *config.Config
	photoDir string
	appName  string
}

func NewProfileDetails(svc ProfileDetailsService, cfg *config.Config, appName string) *ProfileDetails {
	return &ProfileDetails{svc: svc, cfg: cfg, photoDir: photoDir(), appName: appName}
}

// Update handles the details form submission.
func (h *ProfileDetails) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := profiledetails.Input{
		Name:             strings.TrimSpace(r.FormValue("name")),
		Email:            strings.TrimSpace(r.FormValue("email")),
		Phone:            strings.TrimSpace(r.FormValue("phone")),
		DateOfBirth:      strings.TrimSpace(r.FormValue("date_of_birth")),
		Gender:           strings.TrimSpace(r.FormValue("gender")),
		BloodGroup:       strings.TrimSpace(r.FormValue("blood_group")),
		PresentAddress:   strings.TrimSpace(r.FormValue("present_address")),
		PermanentAddress: strings.TrimSpace(r.FormValue("permanent_address")),
		Website:          strings.TrimSpace(r.FormValue("website")),
		SocialLinks:      nestedMap(r, "social_links", "facebook", "linkedin"),
		EmergencyContact: nestedMap(r, "emergency_contact", "name", "phone", "relation"),
		LocalNames:       localNamesFrom(r, h.cfg),
	}

	// Store any uploaded photo first; the service keeps the previous path when
	// none arrives (replace semantics handled here so the service stays pure).
	if path, ok := h.storePhoto(r); ok {
		in.PhotoPath = &path
		if old, err := h.svc.PhotoPathOf(r.Context(), u.ID); err == nil && old != "" && old != path {
			_ = os.Remove(filepath.Join(h.photoDir, filepath.Base(old)))
		}
	}

	if err := h.svc.Update(r.Context(), u.ID, in); err != nil {
		var fieldErrs profiledetails.FieldErrors
		if errors.As(err, &fieldErrs) {
			middleware.RedirectWithFlash(w, r, "/dashboard/profile#profile", "error", fieldErrs.Error())
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#profile", "status", "profile-details-updated")
}

// storePhoto persists an uploaded profile photo to the public disk and returns
// its relative path (ok=false when no file was sent).
func (h *ProfileDetails) storePhoto(r *http.Request) (string, bool) {
	file, header, err := r.FormFile("photo")
	if err != nil {
		return "", false
	}
	defer file.Close()

	if header.Size > maxPhotoBytes {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isImageExt(ext) {
		return "", false
	}
	if err := os.MkdirAll(h.photoDir, 0o755); err != nil {
		return "", false
	}
	name := randomFileName(ext)
	dst, err := os.Create(filepath.Join(h.photoDir, name))
	if err != nil {
		return "", false
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", false
	}
	return filepath.Join("profile-photos", name), true
}

// nestedMap collects r.FormValue(prefix[key]) for each key (reference
// social_links[facebook] etc).
func nestedMap(r *http.Request, prefix string, keys ...string) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		if v := strings.TrimSpace(r.FormValue(prefix + "[" + k + "]")); v != "" {
			out[k] = v
		}
	}
	return out
}

func localNamesFrom(r *http.Request, cfg *config.Config) map[string]string {
	out := map[string]string{}
	for _, ln := range cfg.App.LocalNames {
		if v := strings.TrimSpace(r.FormValue("local_names[" + ln.Code + "]")); v != "" {
			out[ln.Code] = v
		}
	}
	return out
}

// photoDir is the public-disk profile-photos folder (relative to CWD).
func photoDir() string {
	return filepath.Join("storage", "app", "public", "profile-photos")
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp":
		return true
	}
	return false
}

// randomFileName builds a collision-resistant photo filename.
func randomFileName(ext string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "photo" + ext
	}
	return hex.EncodeToString(b[:]) + ext
}
