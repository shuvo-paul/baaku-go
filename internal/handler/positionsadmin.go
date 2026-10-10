package handler

import (
	"errors"
	"net/http"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/committee"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// PositionsAdmin is the committee-positions CRUD surface (reference
// PositionController), behind "manage committee" at the route layer.
type PositionsAdmin struct {
	svc     *committee.Service
	appName string
}

func NewPositionsAdmin(svc *committee.Service, appName string) *PositionsAdmin {
	return &PositionsAdmin{svc: svc, appName: appName}
}

// Index renders the positions table (reference PositionController@index).
func (h *PositionsAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	positions, err := h.svc.Positions(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.PositionRow, 0, len(positions))
	for _, p := range positions {
		rows = append(rows, views.PositionRow{ID: p.ID, Name: p.Name, MemberCount: p.MemberCount})
	}
	flash := middleware.FlashFromContext(r.Context())
	views.PositionsIndexPage(views.PositionsIndexData{
		Sidebar: committeeSidebar(h.appName, r, u),
		Rows:    rows,
		Flash:   flash["status"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the create-position form (reference PositionController@create).
func (h *PositionsAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.PositionFormPage(h.formData(r, u, views.PositionForm{Title: "New Position"})).Render(r.Context(), w)
}

// Store handles POST /dashboard/positions (reference PositionController@store).
func (h *PositionsAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	form := views.PositionForm{Title: "New Position", Name: name}
	if _, err := h.svc.StorePosition(r.Context(), name); err != nil {
		var fieldErrs committee.FieldErrors
		if errors.As(err, &fieldErrs) {
			form.Errors = fieldErrs
			views.PositionFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/positions", "status", "position-created")
}

// Edit renders the edit-position form (reference PositionController@edit).
func (h *PositionsAdmin) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GetPosition(r.Context(), id)
	if err != nil {
		if errors.Is(err, committee.ErrPositionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.PositionFormPage(h.formData(r, u, views.PositionForm{Title: "Edit Position", ID: p.ID, Name: p.Name})).Render(r.Context(), w)
}

// Update handles PUT /dashboard/positions/{id} (reference
// PositionController@update).
func (h *PositionsAdmin) Update(w http.ResponseWriter, r *http.Request) {
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
	name := r.FormValue("name")
	if err := h.svc.UpdatePosition(r.Context(), id, name); err != nil {
		if errors.Is(err, committee.ErrPositionNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs committee.FieldErrors
		if errors.As(err, &fieldErrs) {
			views.PositionFormPage(h.formData(r, u, views.PositionForm{Title: "Edit Position", ID: id, Name: name, Errors: fieldErrs})).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/positions", "status", "position-updated")
}

// Destroy handles DELETE /dashboard/positions/{id} (reference
// PositionController@destroy): members keep existing with a cleared position
// (FK nullOnDelete).
func (h *PositionsAdmin) Destroy(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.DestroyPosition(r.Context(), id); err != nil {
		if errors.Is(err, committee.ErrPositionNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/positions", "status", "position-deleted")
}

// formData builds the position form view model.
func (h *PositionsAdmin) formData(r *http.Request, u user.User, form views.PositionForm) views.PositionForm {
	form.Sidebar = committeeSidebar(h.appName, r, u)
	if form.Title == "" {
		form.Title = "New Position"
	}
	if form.Errors == nil {
		form.Errors = map[string]string{}
	}
	form.CSRF = middleware.TokenFromContext(r.Context())
	return form
}
