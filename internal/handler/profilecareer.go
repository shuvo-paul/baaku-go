package handler

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"strconv"
	"strings"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/career"
	"github.com/shuvo-paul/baaku/internal/views"
)

// CareerService is the career CRUD surface; *career.Service satisfies it.
type CareerService interface {
	List(ctx context.Context, userID int64) ([]career.Career, error)
	Get(ctx context.Context, userID, id int64) (career.Career, error)
	Create(ctx context.Context, userID int64, in career.Input) error
	Update(ctx context.Context, userID, id int64, in career.Input) error
	Delete(ctx context.Context, userID, id int64) error
}

// ProfileCareer serves the user-owned career routes (reference
// ProfileCareerController).
type ProfileCareer struct {
	svc     CareerService
	cfg     *config.Config
	appName string
}

func NewProfileCareer(svc CareerService, cfg *config.Config, appName string) *ProfileCareer {
	return &ProfileCareer{svc: svc, cfg: cfg, appName: appName}
}

// Create renders the add-career form.
func (h *ProfileCareer) Create(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.CareerFormPage(h.form(r, 0, career.Input{}, "")).Render(r.Context(), w)
}

// Store handles POST /dashboard/profile/careers.
func (h *ProfileCareer) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := careerInputFrom(r)
	if err := h.svc.Create(r.Context(), u.ID, in); err != nil {
		h.renderFormError(w, r, 0, in, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#career", "status", "career-created")
}

// Edit renders the edit-career form (ownership-scoped fetch).
func (h *ProfileCareer) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	c, err := h.svc.Get(r.Context(), u.ID, id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.CareerFormPage(h.form(r, id, careerToInput(c), deref(c.Description))).Render(r.Context(), w)
}

// Update handles PUT /dashboard/profile/careers/{id}.
func (h *ProfileCareer) Update(w http.ResponseWriter, r *http.Request) {
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
	in := careerInputFrom(r)
	if err := h.svc.Update(r.Context(), u.ID, id, in); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		h.renderFormError(w, r, id, in, err)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#career", "status", "career-updated")
}

// Destroy handles DELETE /dashboard/profile/careers/{id}.
func (h *ProfileCareer) Destroy(w http.ResponseWriter, r *http.Request) {
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
	middleware.RedirectWithFlash(w, r, "/dashboard/profile#career", "status", "career-deleted")
}

func (h *ProfileCareer) renderFormError(w http.ResponseWriter, r *http.Request, id int64, in career.Input, err error) {
	var fieldErrs career.FieldErrors
	if !errors.As(err, &fieldErrs) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.CareerFormPage(h.form(r, id, in, "")).Render(r.Context(), w)
}

// form builds the CareerForm view model from the input values.
// form builds the CareerForm view model from the input values.
func (h *ProfileCareer) form(r *http.Request, id int64, in career.Input, description string) views.CareerForm {
	if description == "" {
		description = in.Description
	}
	u, _ := middleware.UserFromContext(r.Context())
	return views.CareerForm{
		AppName:         h.appName,
		CSRFToken:       middleware.TokenFromContext(r.Context()),
		Sidebar:         formSidebar(h.appName, middleware.TokenFromContext(r.Context()), u, middleware.PermissionsFromContext(r.Context())),
		ID:              id,
		EmploymentTypes: toViewOptions(h.cfg.Career.EmploymentTypes),
		Values: views.CareerView{
			EmploymentType: in.EmploymentType, JobTitle: in.JobTitle, Company: in.Company,
			Industry: in.Industry, Location: in.Location,
			StartYear: atoi32(in.StartYear), StartMonth: atoi16(in.StartMonth),
			IsCurrent: in.IsCurrent, EndYear: atoi32(in.EndYear), EndMonth: atoi16(in.EndMonth),
			Description: description,
		},
	}
}

func careerInputFrom(r *http.Request) career.Input {
	return career.Input{
		EmploymentType: strings.TrimSpace(r.FormValue("employment_type")),
		JobTitle:       strings.TrimSpace(r.FormValue("job_title")),
		Company:        strings.TrimSpace(r.FormValue("company")),
		Industry:       strings.TrimSpace(r.FormValue("industry")),
		Location:       strings.TrimSpace(r.FormValue("location")),
		StartYear:      strings.TrimSpace(r.FormValue("start_year")),
		StartMonth:     strings.TrimSpace(r.FormValue("start_month")),
		IsCurrent:      r.FormValue("is_current") == "1",
		EndYear:        strings.TrimSpace(r.FormValue("end_year")),
		EndMonth:       strings.TrimSpace(r.FormValue("end_month")),
		Description:    strings.TrimSpace(r.FormValue("description")),
	}
}

func careerToInput(c career.Career) career.Input {
	return career.Input{
		EmploymentType: c.EmploymentType, JobTitle: c.JobTitle, Company: c.Company,
		Industry: deref(c.Industry), Location: deref(c.Location),
		StartYear: itoa32(c.StartYear), StartMonth: itoa16(derefI16(c.StartMonth)),
		IsCurrent: c.IsCurrent, EndYear: itoa32(derefI32(c.EndYear)), EndMonth: itoa16(derefI16(c.EndMonth)),
		Description: deref(c.Description),
	}
}

func itoa32(v int32) string {
	return strconv.FormatInt(int64(v), 10)
}

func itoa16(v int16) string {
	return strconv.FormatInt(int64(v), 10)
}
