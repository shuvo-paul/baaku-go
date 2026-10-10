package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// PaymentAdmin is the admin payment queue/review/record surface (reference
// PaymentController), behind "manage memberships" at the route layer.
type PaymentAdmin struct {
	svc     *membershippayment.Service
	plans   *membershipplan.Service
	methods *membershippaymentmethod.Service
	cfg     *config.Config
	appName string
}

func NewPaymentAdmin(svc *membershippayment.Service, plans *membershipplan.Service, methods *membershippaymentmethod.Service, cfg *config.Config, appName string) *PaymentAdmin {
	return &PaymentAdmin{svc: svc, plans: plans, methods: methods, cfg: cfg, appName: appName}
}

// Index renders the payment queue (reference PaymentController@index).
func (h *PaymentAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "pending"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pg, err := h.svc.List(r.Context(), status, int64(page))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.AdminPaymentsPage(views.AdminPaymentsData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/payments"),
		Rows:     h.adminRows(r, pg.Payments),
		Status:   status,
		Total:    pg.Total,
		Page:     page,
		HasPrev:  page > 1,
		HasNext:  int64(page)*20 < pg.Total,
		Currency: h.cfg.Membership.Currency,
		CSRF:     middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Show renders one payment for review (reference PaymentController@show).
func (h *PaymentAdmin) Show(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, membershippayment.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.AdminPaymentShowPage(views.AdminPaymentShowData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/payments"),
		Payment:  h.adminRows(r, []membershippayment.Payment{p})[0],
		Currency: h.cfg.Membership.Currency,
		CSRF:     middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Create renders the record-payment form (reference PaymentController@create).
func (h *PaymentAdmin) Create(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	views.AdminPaymentRecordPage(h.recordData(r, u)).Render(r.Context(), w)
}

// Store handles POST /dashboard/payments (reference PaymentController@store):
// records a pending payment for another member, optionally activating
// immediately.
func (h *PaymentAdmin) Store(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	userID := atoi64(r.FormValue("user_id"))
	planID := atoi64(r.FormValue("membership_plan_id"))
	amount := r.FormValue("amount")
	method := r.FormValue("method")
	reference := optString(r.FormValue("reference"))
	paidAt := parseTime(r.FormValue("paid_at"))
	notes := optString(r.FormValue("notes"))
	activate := r.FormValue("activate") == "1"
	var proofPath *string
	if path, okPath := storeProof(r, h.cfg); okPath {
		proofPath = &path
	}
	id, _, err := h.svc.Submit(r.Context(), membershippayment.SubmitInput{
		UserID:    userID,
		PlanID:    planID,
		Amount:    amount,
		Method:    method,
		Reference: reference,
		PaidAt:    paidAt,
		Notes:     notes,
		ProofPath: proofPath,
		Activate:  activate,
		CreatedBy: u.ID,
	})
	if err != nil {
		var fieldErrs membershippayment.FieldErrors
		if errors.As(err, &fieldErrs) {
			d := h.recordData(r, u)
			d.Errors = fieldErrs
			views.AdminPaymentRecordPage(d).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payments/"+strconv.FormatInt(id, 10), "status", "payment-recorded")
}

// Approve handles POST /dashboard/payments/{id}/approve (reference
// PaymentController@approve).
func (h *PaymentAdmin) Approve(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if _, err := h.svc.Approve(r.Context(), id, u.ID); err != nil {
		if errors.Is(err, membershippayment.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, membershippayment.ErrNotPending) {
			middleware.RedirectWithFlash(w, r, "/dashboard/payments/"+strconv.FormatInt(id, 10), "error", "payment-not-pending")
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payments", "status", "payment-approved")
}

// Reject handles POST /dashboard/payments/{id}/reject (reference
// PaymentController@reject).
func (h *PaymentAdmin) Reject(w http.ResponseWriter, r *http.Request) {
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
	reviewNotes := r.FormValue("review_notes")
	if err := h.svc.Reject(r.Context(), id, u.ID, reviewNotes); err != nil {
		if errors.Is(err, membershippayment.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, membershippayment.ErrNotPending) {
			middleware.RedirectWithFlash(w, r, "/dashboard/payments/"+strconv.FormatInt(id, 10), "error", "payment-not-pending")
			return
		}
		var fieldErrs membershippayment.FieldErrors
		if errors.As(err, &fieldErrs) {
			middleware.RedirectWithFlash(w, r, "/dashboard/payments/"+strconv.FormatInt(id, 10), "error", fieldErrs.Error())
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/payments", "status", "payment-rejected")
}

// adminRows maps payments to the admin row view model.
func (h *PaymentAdmin) adminRows(_ *http.Request, payments []membershippayment.Payment) []views.AdminPaymentRow {
	out := make([]views.AdminPaymentRow, 0, len(payments))
	for _, p := range payments {
		out = append(out, views.AdminPaymentRow{
			ID:          p.ID,
			UserID:      p.UserID,
			UserName:    p.UserName,
			PlanName:    p.PlanName,
			Amount:      p.Amount,
			Method:      p.Method,
			Reference:   deref(p.Reference),
			Notes:       deref(p.Notes),
			ReviewNotes: deref(p.ReviewNotes),
			PaidAt:      p.PaidAt,
			Status:      string(p.Status),
			ReviewedAt:  p.ReviewedAt,
			ProofURL:    proofURL(p.ProofPath),
		})
	}
	return out
}

// recordData builds the record-payment form view model.
func (h *PaymentAdmin) recordData(r *http.Request, u user.User) views.AdminPaymentRecordData {
	plans, _ := h.plans.ActiveList(r.Context())
	methods, _ := h.methods.ActiveList(r.Context())
	return views.AdminPaymentRecordData{
		Sidebar: membershipSidebar(h.appName, r, u, "/dashboard/payments"),
		Plans:   planOptions(plans),
		Methods: methodOptions(methods),
		CSRF:    middleware.TokenFromContext(r.Context()),
		Errors:  map[string]string{},
	}
}
