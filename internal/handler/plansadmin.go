package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// PlanAdmin is the admin membership-plans CRUD surface (reference
// MembershipPlanController), behind "manage membership plans" at the route
// layer.
type PlanAdmin struct {
	svc     *membershipplan.Service
	cfg     *config.Config
	appName string
}

func NewPlanAdmin(svc *membershipplan.Service, cfg *config.Config, appName string) *PlanAdmin {
	return &PlanAdmin{svc: svc, cfg: cfg, appName: appName}
}

// Index renders the plans table (reference MembershipPlanController@index).
func (h *PlanAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	plans, err := h.svc.AdminList(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.AdminPlanRow, 0, len(plans))
	for _, p := range plans {
		rows = append(rows, views.AdminPlanRow{
			ID:        p.ID,
			Name:      p.Name,
			TermLabel: p.TermLabel(),
			Price:     p.Price,
			IsActive:  p.IsActive,
			SortOrder: p.SortOrder,
		})
	}
	flash := middleware.FlashFromContext(r.Context())
	views.AdminPlansPage(views.AdminPlansData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/plans"),
		Rows:     rows,
		Currency: h.cfg.Membership.Currency,
		Flash:    flash["status"],
		Err:      flash["error"],
		CSRF:     middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the create-plan form (reference MembershipPlanController@
// create).
func (h *PlanAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.AdminPlanFormPage(h.formData(r, u, views.AdminPlanForm{TermType: "days", IsActive: true})).Render(r.Context(), w)
}

// Store handles POST /dashboard/plans (reference MembershipPlanController@
// store).
func (h *PlanAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in, form := h.inputFrom(r)
	if _, err := h.svc.Store(r.Context(), in); err != nil {
		var fieldErrs membershipplan.FieldErrors
		if errors.As(err, &fieldErrs) {
			form.Errors = fieldErrs
			views.AdminPlanFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/plans", "status", "plan-created")
}

// Edit renders the edit-plan form (reference MembershipPlanController@edit).
func (h *PlanAdmin) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, membershipplan.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	form := views.AdminPlanForm{
		ID:          p.ID,
		Title:       "Edit Plan",
		Name:        p.Name,
		Description: deref(p.Description),
		Price:       p.Price,
		IsActive:    p.IsActive,
		Selected:    featureSelected(p.Features),
		TermType:    termTypeOf(p),
		TermDays:    termDaysOf(p),
		TermMonths:  termMonthsOf(p),
	}
	views.AdminPlanFormPage(h.formData(r, u, form)).Render(r.Context(), w)
}

// Update handles PUT /dashboard/plans/{id} (reference
// MembershipPlanController@update).
func (h *PlanAdmin) Update(w http.ResponseWriter, r *http.Request) {
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
	in, form := h.inputFrom(r)
	if err := h.svc.Update(r.Context(), id, in); err != nil {
		if errors.Is(err, membershipplan.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs membershipplan.FieldErrors
		if errors.As(err, &fieldErrs) {
			form.ID = id
			form.Errors = fieldErrs
			views.AdminPlanFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/plans", "status", "plan-updated")
}

// Destroy handles DELETE /dashboard/plans/{id} (reference
// MembershipPlanController@destroy): a plan in use redirects back with the
// plan_in_use error flash.
func (h *PlanAdmin) Destroy(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Destroy(r.Context(), id); err != nil {
		if errors.Is(err, membershipplan.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, membershipplan.ErrInUse) {
			middleware.RedirectWithFlash(w, r, "/dashboard/plans", "error", "plan-in-use")
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/plans", "status", "plan-deleted")
}

// Reorder handles POST /dashboard/plans/reorder (reference
// MembershipPlanController@reorder): assigns each id its list index as the new
// sort order.
func (h *PlanAdmin) Reorder(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.UserFromContext(r.Context()); !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	ids, err := decodeReorderIDs(r)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	for i, id := range ids {
		if err := h.svc.Reorder(r.Context(), id, int32(i)); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w)
}

// inputFrom parses the plan form into the service input and the view form.
func (h *PlanAdmin) inputFrom(r *http.Request) (membershipplan.StoreInput, views.AdminPlanForm) {
	termType := r.FormValue("term_type")
	isLifetime := termType == "lifetime"
	var days, months *int32
	if termType == "days" {
		if d := atoi64(r.FormValue("term_days")); d > 0 {
			v := int32(d)
			days = &v
		}
	}
	if termType == "months" {
		if m := atoi64(r.FormValue("term_months")); m > 0 {
			v := int32(m * 30)
			months = &v
		}
	}
	// The reference normalizes the authored term into a single day count.
	var duration *int32
	if isLifetime {
		duration = nil
	} else if days != nil {
		duration = days
	} else if months != nil {
		duration = months
	}
	in := membershipplan.StoreInput{
		Name:         r.FormValue("name"),
		Description:  optString(r.FormValue("description")),
		Price:        r.FormValue("price"),
		DurationDays: duration,
		IsLifetime:   isLifetime,
		Features:     planFeatureMap(r, featureKeys(h.cfg)),
		IsActive:     boolFromForm(r, "is_active"),
	}
	form := views.AdminPlanForm{
		Title:       "New Plan",
		Name:        in.Name,
		Description: deref(in.Description),
		Price:       in.Price,
		TermType:    termType,
		TermDays:    r.FormValue("term_days"),
		TermMonths:  r.FormValue("term_months"),
		IsLifetime:  isLifetime,
		Features:    featureKeys(h.cfg),
		Selected:    featureSelected(in.Features),
		IsActive:    r.FormValue("is_active") == "1",
		Errors:      map[string]string{},
	}
	return in, form
}

// formData builds the plan form view model.
func (h *PlanAdmin) formData(r *http.Request, u user.User, form views.AdminPlanForm) views.AdminPlanForm {
	form.Sidebar = membershipSidebar(h.appName, r, u, "/dashboard/plans")
	if form.Title == "" {
		form.Title = "New Plan"
	}
	if form.Features == nil {
		form.Features = featureKeys(h.cfg)
	}
	if form.Selected == nil {
		form.Selected = map[string]bool{}
	}
	if form.Errors == nil {
		form.Errors = map[string]string{}
	}
	if form.TermType == "" {
		form.TermType = "days"
	}
	form.CSRF = middleware.TokenFromContext(r.Context())
	return form
}

// termTypeOf derives the form's term radio from a stored plan.
func termTypeOf(p membershipplan.Plan) string {
	if p.IsLifetime {
		return "lifetime"
	}
	if p.DurationDays != nil && *p.DurationDays >= 60 && *p.DurationDays%30 == 0 {
		return "months"
	}
	return "days"
}

// termDaysOf returns the days value for the edit form.
func termDaysOf(p membershipplan.Plan) string {
	if p.IsLifetime || p.DurationDays == nil || termTypeOf(p) != "days" {
		return ""
	}
	return strconv.Itoa(int(*p.DurationDays))
}

// termMonthsOf returns the months value for the edit form.
func termMonthsOf(p membershipplan.Plan) string {
	if p.IsLifetime || p.DurationDays == nil || termTypeOf(p) != "months" {
		return ""
	}
	return strconv.Itoa(int(*p.DurationDays) / 30)
}

// decodeReorderIDs reads the {"ids":[...]} reorder payload.
func decodeReorderIDs(r *http.Request) ([]int64, error) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(body.IDs))
	for _, s := range body.IDs {
		out = append(out, atoi64(s))
	}
	return out, nil
}

// writeJSON writes a minimal {"ok":true} response for the reorder endpoints.
func writeJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
