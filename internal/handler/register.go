// Package handler holds the HTTP handlers: parse, validate, render, delegate.
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/register"
	"github.com/shuvo-paul/baaku/internal/service/session"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// RegisterService is the registration port; *register.RegisterService.
type RegisterService interface {
	Register(ctx context.Context, in register.RegisterInput) (user.User, error)
}

// SessionOpener opens the authenticated session after register (Fortify
// RegisterController logs the user straight in). *session.Store satisfies it.
type SessionOpener interface {
	CreateSession(ctx context.Context, userID int64) (string, error)
}

// VerificationSender emails the signed verification link
// (*emailverify.Resender satisfies it).
type VerificationSender interface {
	Resend(ctx context.Context, userID int64) error
}

// Register renders and processes GET/POST /register (Fortify
// RegisteredUserController + CreateNewUser).
type Register struct {
	svc      RegisterService
	sessions SessionOpener
	resend   VerificationSender
	sessCfg  config.Session
	appName  string
}

func NewRegister(svc RegisterService, sessions SessionOpener, resend VerificationSender, sessCfg config.Session, appName string) *Register {
	return &Register{svc: svc, sessions: sessions, resend: resend, sessCfg: sessCfg, appName: appName}
}

// Show renders GET /register; authenticated visitors bounce to the dashboard.
func (h *Register) Show(w http.ResponseWriter, r *http.Request) {
	if sess, ok := middleware.SessionFromContext(r.Context()); ok && sess.UserID != nil {
		middleware.Redirect(w, r, middleware.DashboardPath)
		return
	}
	views.RegisterPage(h.appName, middleware.TokenFromContext(r.Context()), "").Render(r.Context(), w)
}

// Store handles POST /register: validate + insert (service), open a session
// (Fortify logs the user in), fire the verification email, then redirect to
// the dashboard — RequireVerified bounces the unverified user to /email/verify.
func (h *Register) Store(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := register.RegisterInput{
		Name:                 strings.TrimSpace(r.FormValue("name")),
		Email:                strings.TrimSpace(r.FormValue("email")),
		Phone:                strings.TrimSpace(r.FormValue("phone")),
		Password:             r.FormValue("password"),
		PasswordConfirmation: r.FormValue("password_confirmation"),
		Educations:           parseEducations(r),
	}

	u, err := h.svc.Register(r.Context(), in)
	var fieldErrs register.FieldErrors
	switch {
	case errors.As(err, &fieldErrs):
		h.renderForm(w, r, fieldErrs.Error())
		return
	case err != nil:
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	sid, err := h.sessions.CreateSession(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, session.Cookie(h.sessCfg, sid))

	// Best-effort verification email: the reference sends it from a queued
	// listener, so a mailer outage never rolls back the created user — the
	// user can hit "Resend verification email" instead.
	_ = h.resend.Resend(r.Context(), u.ID) //nolint:errcheck // queued-listener parity

	middleware.Redirect(w, r, intendedDest(r, middleware.DashboardPath))
}

func (h *Register) renderForm(w http.ResponseWriter, r *http.Request, errMsg string) {
	views.RegisterPage(h.appName, middleware.TokenFromContext(r.Context()), errMsg).Render(r.Context(), w)
}

// parseEducations collects educations[i][*] rows until the keys run out.
// Numeric fields that fail to parse become 0 so the service's own
// validation messages apply (required / must be 4 digits).
func parseEducations(r *http.Request) []register.EducationInput {
	var out []register.EducationInput
	for i := 0; ; i++ {
		p := fmt.Sprintf("educations[%d][", i)
		if r.FormValue(p+"level]") == "" && r.FormValue(p+"institution]") == "" &&
			r.FormValue(p+"subject]") == "" && r.FormValue(p+"start_year]") == "" {
			break
		}
		ed := register.EducationInput{
			Level:       strings.TrimSpace(r.FormValue(p + "level]")),
			Institution: strings.TrimSpace(r.FormValue(p + "institution]")),
			Subject:     strings.TrimSpace(r.FormValue(p + "subject]")),
			IsCurrent:   r.FormValue(p+"is_current]") == "1",
			StartYear:   formInt32(r.FormValue(p + "start_year]")),
		}
		if sid := strings.TrimSpace(r.FormValue(p + "student_id]")); sid != "" {
			ed.StudentID = &sid
		}
		if v := formInt16(r.FormValue(p + "start_month]")); v != nil {
			ed.StartMonth = v
		}
		if y := formInt32(r.FormValue(p + "end_year]")); y != 0 {
			ed.EndYear = &y
		}
		if v := formInt16(r.FormValue(p + "end_month]")); v != nil {
			ed.EndMonth = v
		}
		out = append(out, ed)
	}
	return out
}

func formInt32(s string) int32 {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return int32(n)
}

func formInt16(s string) *int16 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		n = -1 // non-numeric garbage → out of range → service's 1..12 message
	}
	v := int16(n)
	return &v
}
