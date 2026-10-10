package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membership"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/service/membershippaymentmethod"
	"github.com/shuvo-paul/baaku/internal/service/membershipplan"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// MyMembership is the member-facing membership surface (reference
// MyMembershipController).
type MyMembership struct {
	members  *membership.Service
	payments *membershippayment.Service
	plans    *membershipplan.Service
	methods  *membershippaymentmethod.Service
	cfg      *config.Config
	appName  string
}

func NewMyMembership(members *membership.Service, payments *membershippayment.Service, plans *membershipplan.Service, methods *membershippaymentmethod.Service, cfg *config.Config, appName string) *MyMembership {
	return &MyMembership{members: members, payments: payments, plans: plans, methods: methods, cfg: cfg, appName: appName}
}

// Show renders the member's membership page (reference
// MyMembershipController@show).
func (h *MyMembership) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	d := views.MyMembershipData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/membership"),
		Currency: h.cfg.Membership.Currency,
		CSRF:     middleware.TokenFromContext(r.Context()),
	}
	// Active membership (plan + features) when present.
	if m, err := h.members.ActiveForUser(r.Context(), u.ID); err == nil {
		d.HasActive = true
		d.PlanName = m.Plan.Name
		d.IsLifetime = m.Plan.IsLifetime
		d.StartsAt, d.EndsAt = m.StartsAt, m.EndsAt
		for key := range m.Plan.Features {
			d.Features = append(d.Features, key)
		}
	}
	// Pending payment banner.
	if p, err := h.payments.PendingForUser(r.Context(), u.ID); err == nil {
		d.PendingPayID = p.ID
	}
	// Payment history page.
	if pg, err := h.payments.ListForUser(r.Context(), u.ID, 1); err == nil {
		for _, p := range pg.Payments {
			d.Payments = append(d.Payments, views.MyPaymentRow{
				ID:       p.ID,
				PlanName: p.PlanName,
				Amount:   p.Amount,
				Method:   p.Method,
				PaidAt:   p.PaidAt,
				Status:   string(p.Status),
			})
		}
	}
	flash := middleware.FlashFromContext(r.Context())
	d.Status, d.Err = flash["status"], flash["error"]
	views.MyMembershipPage(d).Render(r.Context(), w)
}

// Plans renders the member-facing plans page (reference
// MyMembershipController@plans).
func (h *MyMembership) Plans(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	plans, err := h.plans.ActiveList(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	cards := make([]views.MemberPlanCard, 0, len(plans))
	for _, p := range plans {
		cards = append(cards, views.MemberPlanCard{
			ID:          p.ID,
			Name:        p.Name,
			Description: deref(p.Description),
			Price:       p.Price,
			TermLabel:   p.TermLabel(),
			IsLifetime:  p.IsLifetime,
			Features:    featureKeyList(p.Features),
		})
	}
	views.MemberPlansPage(views.MemberPlansData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/membership/plans"),
		Plans:    cards,
		Currency: h.cfg.Membership.Currency,
		CSRF:     middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// CreatePayment renders the payment submit form (reference
// MyMembershipController@createPayment). ?plan= pre-selects a plan.
func (h *MyMembership) CreatePayment(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	d := h.paymentFormData(r, u)
	d.PlanID = atoi64(r.URL.Query().Get("plan"))
	if d.PlanID != 0 {
		if p, err := h.plans.Get(r.Context(), d.PlanID); err == nil {
			d.Amount = p.Price
		}
	}
	views.MemberPaymentCreatePage(d).Render(r.Context(), w)
}

// StorePayment handles POST /dashboard/membership/payments (reference
// MyMembershipController@storePayment): stores the proof and records a pending
// payment, then redirects to the payment page.
func (h *MyMembership) StorePayment(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	planID := atoi64(r.FormValue("membership_plan_id"))
	amount := r.FormValue("amount")
	method := r.FormValue("method")
	reference := optString(r.FormValue("reference"))
	paidAt := parseTime(r.FormValue("paid_at"))
	notes := optString(r.FormValue("notes"))
	var proofPath *string
	if path, okPath := storeProof(r, h.cfg); okPath {
		proofPath = &path
	}
	id, _, err := h.payments.Submit(r.Context(), membershippayment.SubmitInput{
		UserID:    u.ID,
		PlanID:    planID,
		Amount:    amount,
		Method:    method,
		Reference: reference,
		PaidAt:    paidAt,
		Notes:     notes,
		ProofPath: proofPath,
		CreatedBy: u.ID,
	})
	if err != nil {
		var fieldErrs membershippayment.FieldErrors
		if errors.As(err, &fieldErrs) {
			d := h.paymentFormData(r, u)
			d.PlanID = planID
			d.Amount = amount
			d.Errors = fieldErrs
			views.MemberPaymentCreatePage(d).Render(r.Context(), w)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/membership/payments/"+strconv.FormatInt(id, 10), "status", "payment-submitted")
}

// ShowPayment renders one owned payment (reference
// MyMembershipController@showPayment).
func (h *MyMembership) ShowPayment(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.payments.GetOwned(r.Context(), u.ID, id)
	if err != nil {
		if errors.Is(err, membershippayment.ErrNotOwner) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if errors.Is(err, membershippayment.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.MemberPaymentShowPage(views.MemberPaymentShowData{
		Sidebar:     membershipSidebar(h.appName, r, u, "/dashboard/membership"),
		Payment:     myPaymentRow(p),
		Reference:   deref(p.Reference),
		Notes:       deref(p.Notes),
		ReviewNotes: deref(p.ReviewNotes),
		MethodName:  methodTypeLabel(p.Method),
		ProofURL:    proofURL(p.ProofPath),
		Currency:    h.cfg.Membership.Currency,
		CSRF:        middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// paymentFormData builds the payment submit form view model.
func (h *MyMembership) paymentFormData(r *http.Request, u user.User) views.MemberPaymentCreateData {
	plans, _ := h.plans.ActiveList(r.Context())
	methods, _ := h.methods.ActiveList(r.Context())
	return views.MemberPaymentCreateData{
		Sidebar:  membershipSidebar(h.appName, r, u, "/dashboard/membership"),
		Plans:    planOptions(plans),
		Methods:  methodOptions(methods),
		Currency: h.cfg.Membership.Currency,
		CSRF:     middleware.TokenFromContext(r.Context()),
		Errors:   map[string]string{},
	}
}

// myPaymentRow maps a payment to the member-facing row view model.
func myPaymentRow(p membershippayment.Payment) views.MyPaymentRow {
	return views.MyPaymentRow{
		ID:       p.ID,
		PlanName: p.PlanName,
		Amount:   p.Amount,
		Method:   p.Method,
		PaidAt:   p.PaidAt,
		Status:   string(p.Status),
	}
}

// featureKeyList returns a plan's granted feature keys in a stable order.
func featureKeyList(features map[string]string) []string {
	keys := make([]string, 0, len(features))
	for k := range features {
		keys = append(keys, k)
	}
	return keys
}

// methodTypeLabel maps a payment method string to its human label.
func methodTypeLabel(t string) string {
	return membershippaymentmethod.MethodType(t).Label()
}
