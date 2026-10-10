package handler

import (
	"errors"
	"net/http"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// MethodAdmin is the admin payment-methods CRUD surface (reference
// MembershipPaymentMethodController), behind "manage membership plans" at the
// route layer.
type MethodAdmin struct {
	svc     *membershippaymentmethod.Service
	appName string
}

func NewMethodAdmin(svc *membershippaymentmethod.Service, appName string) *MethodAdmin {
	return &MethodAdmin{svc: svc, appName: appName}
}

// Index renders the payment-methods table (reference
// MembershipPaymentMethodController@index).
func (h *MethodAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	methods, err := h.svc.AdminList(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.AdminMethodRow, 0, len(methods))
	for _, m := range methods {
		rows = append(rows, views.AdminMethodRow{
			ID:       m.ID,
			Type:     string(m.Type),
			Label:    m.Type.Label(),
			IsActive: m.IsActive,
		})
	}
	flash := middleware.FlashFromContext(r.Context())
	views.AdminMethodsPage(views.AdminMethodsData{
		Sidebar: membershipSidebar(h.appName, r, u, "/dashboard/payment-methods"),
		Rows:    rows,
		Flash:   flash["status"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the create-method form (reference
// MembershipPaymentMethodController@create).
func (h *MethodAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.AdminMethodFormPage(h.formData(r, u, views.AdminMethodForm{
		Title:       "New Payment Method",
		TypeOptions: methodTypeOptions(),
		IsActive:    true,
	})).Render(r.Context(), w)
}

// Store handles POST /dashboard/payment-methods (reference
// MembershipPaymentMethodController@store).
func (h *MethodAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	in := membershippaymentmethod.StoreInput{
		Type:         r.FormValue("type"),
		Instructions: optString(r.FormValue("instructions")),
		IsActive:     boolFromForm(r, "is_active"),
	}
	if _, err := h.svc.Store(r.Context(), in); err != nil {
		var fieldErrs membershippaymentmethod.FieldErrors
		if errors.As(err, &fieldErrs) {
			form := h.formFrom(r, in)
			form.Errors = fieldErrs
			views.AdminMethodFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payment-methods", "status", "method-created")
}

// Edit renders the edit-method form (reference
// MembershipPaymentMethodController@edit). The type is locked on edit.
func (h *MethodAdmin) Edit(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	m, err := h.svc.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, membershippaymentmethod.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.AdminMethodFormPage(h.formData(r, u, views.AdminMethodForm{
		Title:        "Edit Payment Method",
		ID:           m.ID,
		Type:         string(m.Type),
		LockedType:   true,
		Instructions: deref(m.Instructions),
		IsActive:     m.IsActive,
	})).Render(r.Context(), w)
}

// Update handles PUT /dashboard/payment-methods/{id} (reference
// MembershipPaymentMethodController@update).
func (h *MethodAdmin) Update(w http.ResponseWriter, r *http.Request) {
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
	in := membershippaymentmethod.StoreInput{
		Type:         r.FormValue("type"),
		Instructions: optString(r.FormValue("instructions")),
		IsActive:     boolFromForm(r, "is_active"),
	}
	if err := h.svc.Update(r.Context(), id, in); err != nil {
		if errors.Is(err, membershippaymentmethod.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs membershippaymentmethod.FieldErrors
		if errors.As(err, &fieldErrs) {
			form := h.formFrom(r, in)
			form.ID = id
			form.LockedType = true
			form.Errors = fieldErrs
			views.AdminMethodFormPage(h.formData(r, u, form)).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payment-methods", "status", "method-updated")
}

// Destroy handles DELETE /dashboard/payment-methods/{id} (reference
// MembershipPaymentMethodController@destroy).
func (h *MethodAdmin) Destroy(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, membershippaymentmethod.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payment-methods", "status", "method-deleted")
}

// formFrom maps the submitted input to the form view model.
func (h *MethodAdmin) formFrom(r *http.Request, in membershippaymentmethod.StoreInput) views.AdminMethodForm {
	return views.AdminMethodForm{
		Title:        "New Payment Method",
		Type:         in.Type,
		Instructions: deref(in.Instructions),
		IsActive:     r.FormValue("is_active") == "1",
		TypeOptions:  methodTypeOptions(),
		Errors:       map[string]string{},
	}
}

// formData builds the method form view model.
func (h *MethodAdmin) formData(r *http.Request, u user.User, form views.AdminMethodForm) views.AdminMethodForm {
	form.Sidebar = membershipSidebar(h.appName, r, u, "/dashboard/payment-methods")
	if form.TypeOptions == nil {
		form.TypeOptions = methodTypeOptions()
	}
	if form.Errors == nil {
		form.Errors = map[string]string{}
	}
	form.CSRF = middleware.TokenFromContext(r.Context())
	return form
}
