// Package handler — membership handlers port the reference membership
// controllers: the member-facing membership/plans/payment pages
// (MyMembershipController) and the admin memberships/payments/plans/
// payment-methods surfaces. Handlers only parse, validate and render; all
// business logic lives in the services.
package handler

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// membershipSidebar builds the dashboard chrome for a membership page.
func membershipSidebar(appName string, r *http.Request, u user.User, activePath string) views.SidebarData {
	return views.NewSidebarData(appName, middleware.TokenFromContext(r.Context()), u.Name, u.Email, activePath, string(u.State), middleware.PermissionsFromContext(r.Context()))
}

// proofDir is the public-disk membership-payment-proofs folder (relative to
// CWD), mirroring the reference Storage::disk('public') path.
func proofDir() string {
	return filepath.Join("storage", "app", "public", "membership-payment-proofs")
}

// storeProof persists an uploaded payment proof to the public disk and returns
// its relative path (ok=false when no file was sent or it is rejected).
func storeProof(r *http.Request, cfg *config.Config) (string, bool) {
	file, header, err := r.FormFile("proof")
	if err != nil {
		return "", false
	}
	defer file.Close()

	maxBytes := cfg.Membership.ProofMaxKB * 1024
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	if header.Size > maxBytes {
		return "", false
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !isProofExt(ext, cfg.Membership.ProofMimes) {
		return "", false
	}
	if err := os.MkdirAll(proofDir(), 0o755); err != nil {
		return "", false
	}
	name := randomFileName(ext)
	dst, err := os.Create(filepath.Join(proofDir(), name))
	if err != nil {
		return "", false
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		return "", false
	}
	return filepath.Join("membership-payment-proofs", name), true
}

// isProofExt reports whether ext is an allowed proof mime extension.
func isProofExt(ext string, allowed []string) bool {
	ext = strings.TrimPrefix(ext, ".")
	for _, a := range allowed {
		if strings.EqualFold(a, ext) {
			return true
		}
	}
	return false
}

// proofURL maps a stored proof_path to its streamed media URL ("" when none),
// mirroring photoURL.
func proofURL(path *string) string {
	if path == nil || *path == "" {
		return ""
	}
	return "/media/membership-proofs/" + filepath.Base(*path)
}

// parseTime parses a yyyy-mm-dd form value (zero time when empty/invalid).
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// optString returns a pointer to the trimmed value, or nil when empty.
func optString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// planOptions maps active plans to payment-form select options.
func planOptions(plans []membershipplan.Plan) []views.PlanOption {
	out := make([]views.PlanOption, 0, len(plans))
	for _, p := range plans {
		out = append(out, views.PlanOption{ID: p.ID, Name: p.Name, Price: p.Price})
	}
	return out
}

// methodOptions maps active methods to payment-form select options with their
// instructions rendered to HTML.
func methodOptions(methods []membershippaymentmethod.Method) []views.MethodOption {
	out := make([]views.MethodOption, 0, len(methods))
	for _, m := range methods {
		out = append(out, views.MethodOption{
			Type:         string(m.Type),
			Label:        m.Type.Label(),
			Instructions: m.InstructionsHTML(),
		})
	}
	return out
}

// methodTypeOptions maps the known method types to create-form select options.
func methodTypeOptions() []views.MethodOption {
	out := []views.MethodOption{}
	for _, t := range membershippaymentmethod.AllMethodTypes() {
		out = append(out, views.MethodOption{Type: string(t), Label: t.Label()})
	}
	return out
}

// featureKeys returns the configured gateable-feature keys for the plan editor.
func featureKeys(cfg *config.Config) []string {
	return cfg.Membership.GateableFeatures
}

// boolFromForm reads a checkbox value ("1" present) as a *bool for the
// store/update inputs; absent → nil (field not submitted).
func boolFromForm(r *http.Request, name string) *bool {
	if _, ok := r.Form[name]; !ok {
		return nil
	}
	v := r.FormValue(name) == "1"
	return &v
}

// planFeatureMap collects the feature_{key} checkboxes into the stored map.
func planFeatureMap(r *http.Request, keys []string) map[string]string {
	out := map[string]string{}
	for _, k := range keys {
		if r.FormValue("feature_"+k) == "1" {
			out[k] = "1"
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// featureSelected converts a stored features map to the form-checked set.
func featureSelected(features map[string]string) map[string]bool {
	out := make(map[string]bool, len(features))
	for k, v := range features {
		if v != "" && v != "0" {
			out[k] = true
		}
	}
	return out
}

// atoi64 parses a form value to int64 (0 when empty/invalid).
func atoi64(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
