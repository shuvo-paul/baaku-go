package handler

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/shuvo-paul/baaku/internal/config"
	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/membership"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/views"
)

// MembershipAdmin is the admin memberships surface (reference
// MembershipController), behind "manage memberships" at the route layer.
type MembershipAdmin struct {
	svc      *membership.Service
	payments *membershippayment.Service
	cfg      *config.Config
	appName  string
}

func NewMembershipAdmin(svc *membership.Service, payments *membershippayment.Service, cfg *config.Config, appName string) *MembershipAdmin {
	return &MembershipAdmin{svc: svc, payments: payments, cfg: cfg, appName: appName}
}

// Index renders the memberships list (reference MembershipController@index).
func (h *MembershipAdmin) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pg, err := h.svc.AdminList(r.Context(), status, int64(page))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	rows := make([]views.AdminMembershipRow, 0, len(pg.Items))
	for _, it := range pg.Items {
		rows = append(rows, views.AdminMembershipRow{
			ID:         it.ID,
			UserID:     it.UserID,
			UserName:   it.UserName,
			PlanName:   it.PlanName,
			Status:     string(it.EffectiveStatus()),
			StartsAt:   it.StartsAt,
			EndsAt:     it.EndsAt,
			IsLifetime: it.Plan.IsLifetime,
			Notes:      deref(it.Notes),
		})
	}
	if status == "" {
		status = "all"
	}
	flash := middleware.FlashFromContext(r.Context())
	views.AdminMembershipsPage(views.AdminMembershipsData{
		Sidebar: membershipSidebar(h.appName, r, u, "/dashboard/memberships"),
		Rows:    rows,
		Status:  status,
		Total:   pg.Total,
		Page:    page,
		HasPrev: page > 1,
		HasNext: int64(page)*20 < pg.Total,
		Flash:   flash["status"],
		Err:     flash["error"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Show renders one membership for management (reference
// MembershipController@show).
func (h *MembershipAdmin) Show(w http.ResponseWriter, r *http.Request) {
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
		if errors.Is(err, membership.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	row := views.AdminMembershipRow{
		ID:         m.ID,
		UserID:     m.UserID,
		UserName:   "", // Get does not join the member name; show page shows plan context
		PlanName:   m.Plan.Name,
		Status:     string(m.EffectiveStatus()),
		StartsAt:   m.StartsAt,
		EndsAt:     m.EndsAt,
		IsLifetime: m.Plan.IsLifetime,
		Notes:      deref(m.Notes),
	}
	// Payments for this membership.
	if pays, err := h.payments.ListForMembership(r.Context(), id); err == nil {
		for _, p := range pays {
			row.Payments = append(row.Payments, views.AdminPaymentRow{
				ID:       p.ID,
				Amount:   p.Amount,
				Method:   p.Method,
				PaidAt:   p.PaidAt,
				Status:   string(p.Status),
				PlanName: p.PlanName,
			})
		}
	}
	flash := middleware.FlashFromContext(r.Context())
	views.AdminMembershipShowPage(views.AdminMembershipShowData{
		Sidebar:    membershipSidebar(h.appName, r, u, "/dashboard/memberships"),
		Membership: row,
		CanCancel:  m.Status == membership.StatusActive,
		Err:        flash["error"],
		CSRF:       middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// Update handles PUT /dashboard/memberships/{id} (reference
// MembershipController@update): staff adjusts the end date + notes.
func (h *MembershipAdmin) Update(w http.ResponseWriter, r *http.Request) {
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
	var endsAt *time.Time
	if s := r.FormValue("ends_at"); s != "" {
		t := parseTime(s)
		endsAt = &t
	}
	notes := optString(r.FormValue("notes"))
	if err := h.svc.Update(r.Context(), id, endsAt, notes, &u.ID); err != nil {
		if errors.Is(err, membership.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs membership.FieldErrors
		if errors.As(err, &fieldErrs) {
			middleware.RedirectWithFlash(w, r, "/dashboard/memberships/"+strconv.FormatInt(id, 10), "error", "ends-after-start")
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/memberships/"+strconv.FormatInt(id, 10), "status", "membership-updated")
}

// Cancel handles POST /dashboard/memberships/{id}/cancel (reference
// MembershipController@cancel).
func (h *MembershipAdmin) Cancel(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Cancel(r.Context(), id, &u.ID); err != nil {
		if errors.Is(err, membership.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		var fieldErrs membership.FieldErrors
		if errors.As(err, &fieldErrs) {
			middleware.RedirectWithFlash(w, r, "/dashboard/memberships/"+strconv.FormatInt(id, 10), "error", "membership-not-active")
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	middleware.RedirectWithFlash(w, r, "/dashboard/memberships/"+strconv.FormatInt(id, 10), "status", "membership-cancelled")
}
