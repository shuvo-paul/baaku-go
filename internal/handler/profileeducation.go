package handler

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/education"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// formSidebar builds the dashboard chrome for the education/career form pages,
// which all live under /dashboard/profile. permissions comes from
// middleware.LoadPermissions and gates nav items (reference
// auth()->user()->can() checks in the dashboard layout).
func formSidebar(appName, csrf string, u user.User, permissions []string) views.SidebarData {
	return views.NewSidebarData(appName, csrf, u.Name, u.Email, "/dashboard/profile", string(u.State), permissions)
}

// EducationService is the education CRUD surface; *education.Service
// satisfies it.
type EducationService interface {
	List(ctx context.Context, userID int64) ([]education.Education, error)
	Get(ctx context.Context, userID, id int64) (education.Education, error)
	Create(ctx context.Context, userID int64, in education.Input) error
	Update(ctx context.Context, userID, id int64, in education.Input) error
	Delete(ctx context.Context, userID, id int64) error
}

// ProfileEducation serves the user-owned education routes (reference
// ProfileEducationController).
type ProfileEducation struct {
	svc     EducationService
	cfg     *config.Config
	appName string
}

func NewProfileEducation(svc EducationService, cfg *config.Config, appName string) *ProfileEducation {
	return &ProfileEducation{svc: svc, cfg: cfg, appName: appName}
}

// Create renders the add-education form.
func (h *ProfileEducation) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.EducationFormPage(views.EducationForm{
		AppName: h.appName, CSRFToken: middleware.TokenFromContext(r.Context()),
		Sidebar: formSidebar(h.appName, middleware.TokenFromContext(r.Context()), u, middleware.PermissionsFromContext(r.Context())),
		Levels:  h.cfg.Education.Levels, Institutions: h.cfg.Education.Institutions, Subjects: h.cfg.Education.Subjects,
	}).Render(r.Context(), w)
}

// Store handles POST /dashboard/profile/educations.
func (h *ProfileEducation) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := parseEducationInput(r)
	if err := h.svc.Create(r.Context(), u.ID, in); err != nil {
		h.renderFormError(w, r, 0, in, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#education", "status", "education-created")
}

// Edit renders the edit-education form (ownership-scoped fetch).
func (h *ProfileEducation) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	e, err := h.svc.Get(r.Context(), u.ID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.EducationFormPage(views.EducationForm{
		AppName: h.appName, CSRFToken: middleware.TokenFromContext(r.Context()), ID: id,
		Sidebar: formSidebar(h.appName, middleware.TokenFromContext(r.Context()), u, middleware.PermissionsFromContext(r.Context())),
		Values:  educationToView(e),
		Levels:  h.cfg.Education.Levels, Institutions: h.cfg.Education.Institutions, Subjects: h.cfg.Education.Subjects,
	}).Render(r.Context(), w)
}

// Update handles PUT /dashboard/profile/educations/{id}.
func (h *ProfileEducation) Update(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := parseEducationInput(r)
	if err := h.svc.Update(r.Context(), u.ID, id, in); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.renderFormError(w, r, id, in, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#education", "status", "education-updated")
}

// Destroy handles DELETE /dashboard/profile/educations/{id}.
func (h *ProfileEducation) Destroy(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), u.ID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#education", "status", "education-deleted")
}

func (h *ProfileEducation) renderFormError(w http.ResponseWriter, r *http.Request, id int64, in education.Input, err error) {
	var fieldErrs education.FieldErrors
	if !errors.As(err, &fieldErrs) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	u, _ := middleware.UserFromContext(r.Context())
	views.EducationFormPage(views.EducationForm{
		AppName: h.appName, CSRFToken: middleware.TokenFromContext(r.Context()), ID: id,
		Sidebar: formSidebar(h.appName, middleware.TokenFromContext(r.Context()), u, middleware.PermissionsFromContext(r.Context())),
		Values:  inputToEducationView(in),
		Levels:  h.cfg.Education.Levels, Institutions: h.cfg.Education.Institutions, Subjects: h.cfg.Education.Subjects,
		ErrMsg: fieldErrs.Error(),
	}).Render(r.Context(), w)
}

func parseEducationInput(r *http.Request) education.Input {
	return education.Input{
		Level:       strings.TrimSpace(r.FormValue("level")),
		Institution: strings.TrimSpace(r.FormValue("institution")),
		StudentID:   strings.TrimSpace(r.FormValue("student_id")),
		Subject:     strings.TrimSpace(r.FormValue("subject")),
		StartYear:   strings.TrimSpace(r.FormValue("start_year")),
		StartMonth:  strings.TrimSpace(r.FormValue("start_month")),
		IsCurrent:   r.FormValue("is_current") == "1",
		EndYear:     strings.TrimSpace(r.FormValue("end_year")),
		EndMonth:    strings.TrimSpace(r.FormValue("end_month")),
	}
}

func educationToView(e education.Education) views.EducationView {
	return views.EducationView{
		ID: e.ID, Level: e.Level, Institution: e.Institution, Subject: e.Subject,
		StudentID: deref(e.StudentID), IsCurrent: e.IsCurrent,
		StartYear: e.StartYear, StartMonth: derefI16(e.StartMonth),
		EndYear: derefI32(e.EndYear), EndMonth: derefI16(e.EndMonth),
	}
}

func inputToEducationView(in education.Input) views.EducationView {
	return views.EducationView{
		Level: in.Level, Institution: in.Institution, Subject: in.Subject,
		StudentID: in.StudentID, IsCurrent: in.IsCurrent,
		StartYear: atoi32(in.StartYear), StartMonth: atoi16(in.StartMonth),
		EndYear: atoi32(in.EndYear), EndMonth: atoi16(in.EndMonth),
	}
}

// parseID reads the {id} URL param as int64; a bad value 404s.
func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return 0, false
	}
	return id, true
}

func atoi32(s string) int32 {
	n, _ := strconv.Atoi(s)
	return int32(n)
}

func atoi16(s string) int16 {
	n, _ := strconv.Atoi(s)
	return int16(n)
}
