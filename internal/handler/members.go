// Package handler — Members serves the dashboard member directory (reference
// UserRoleController@index/show) and membership-state changes
// (reference UserStateController@update). The index/show routes are reachable
// by every verified member; the handler branches on the "manage members"
// permission exactly like the reference (admins see every state, everyone
// else sees only active). The state-update route is additionally gated by
// "manage members" at the route layer.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/memberdirectory"
	"github.com/shuvo-paul/baaku/internal/service/membership"
	"github.com/shuvo-paul/baaku/internal/service/membershippayment"
	"github.com/shuvo-paul/baaku/internal/service/user"
	"github.com/shuvo-paul/baaku/internal/views"
)

// MemberDirectory is the member directory + state-change surface;
// *memberdirectory.Service satisfies it.
type MemberDirectory interface {
	Directory(ctx context.Context, isAdmin bool, filter, search string, page int64) (memberdirectory.Page, map[string]int64, error)
	Get(ctx context.Context, isAdmin bool, id int64) (memberdirectory.Member, error)
	ChangeState(ctx context.Context, actorID, targetID int64, state, reason string) error
}

// MemberMemberships supplies the show-page membership summary (reference
// $user->latestMembership()); *service/membership.Service satisfies it.
type MemberMemberships interface {
	LatestForUser(ctx context.Context, userID int64) (membership.Membership, error)
}

// MemberPaymentLister supplies the show-page payment list; the handler slices
// the first page down to the latest 5 (reference ->latest('id')->limit(5)).
type MemberPaymentLister interface {
	ListForUser(ctx context.Context, userID, page int64) (membershippayment.Page, error)
}

// Members renders the directory index + member show pages and handles state
// transitions.
type Members struct {
	svc                MemberDirectory
	memberships        MemberMemberships
	payments           MemberPaymentLister
	membershipsEnabled bool
	appName            string
}

func NewMembers(svc MemberDirectory, memberships MemberMemberships, payments MemberPaymentLister, membershipsEnabled bool, appName string) *Members {
	return &Members{svc: svc, memberships: memberships, payments: payments, membershipsEnabled: membershipsEnabled, appName: appName}
}

// can reports whether the viewer holds the named permission (the same check
// the reference's layout `can()` and controller `$request->user()->can()` do).
func can(r *http.Request, permission string) bool {
	for _, p := range middleware.PermissionsFromContext(r.Context()) {
		if p == permission {
			return true
		}
	}
	return false
}

// Index renders the member directory (reference UserRoleController@index).
// ?filter=&search=&page= mirror the query string the reference paginates with.
func (h *Members) Index(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	isAdmin := can(r, "manage members")
	filter := r.URL.Query().Get("filter")
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pg, counts, err := h.svc.Directory(r.Context(), isAdmin, filter, search, int64(page))
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// The service normalizes filter, but the view needs the value it settled
	// on ("all" for the default) — recompute the same way the reference does.
	if !isAdmin || filter == "" {
		filter = "all"
	} else if !validFilterArg(filter) {
		filter = "all"
	}
	flash := middleware.FlashFromContext(r.Context())
	// The reference's card footer reads "Review & approve" while the pending
	// filter is on (users/partials/grid.blade.php).
	actionLabel := "View profile"
	if filter == "pending" {
		actionLabel = "Review & approve"
	}
	data := views.MembersPageData{
		Sidebar: membersSidebar(h.appName, r, u),
		IsAdmin: isAdmin,
		Members: memberCards(pg.Members, actionLabel),
		Filter:  filter,
		Search:  search,
		Counts:  counts,
		Total:   pg.Total,
		Page:    int64(page),
		HasPrev: page > 1,
		HasNext: int64(page)*memberdirectory.PerPage < pg.Total,
		Flash:   flash["status"],
		Err:     flash["error"],
		CSRF:    middleware.TokenFromContext(r.Context()),
	}
	// The reference answers the debounced live search with just the grid and
	// pagination fragments (users/index.blade.php ajax() branch).
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		var grid, pagination bytes.Buffer
		if err := views.MemberCards(data).Render(r.Context(), &grid); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if err := views.MemberPaginationNav(data).Render(r.Context(), &pagination); err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"grid": grid.String(), "pagination": pagination.String()})
		return
	}
	views.MembersPage(data).Render(r.Context(), w)
}

// Show renders one member profile (reference UserRoleController@show). The
// service scopes visibility: non-admins only see active members.
func (h *Members) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	isAdmin := can(r, "manage members")
	m, err := h.svc.Get(r.Context(), isAdmin, id)
	if errors.Is(err, memberdirectory.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	// The reference hides the transition controls for the viewer's own profile
	// and for unverified targets; admins additionally get the phone,
	// addresses and emergency contact.
	showTransitions := isAdmin && m.EmailVerified && m.ID != u.ID
	transitions := []views.TransitionView{}
	if showTransitions {
		transitions = transitionViews(id, m.State.Transitions(), middleware.TokenFromContext(r.Context()))
	}
	// The membership summary box renders for viewers who can manage
	// memberships while the memberships feature is on (reference
	// users/show.blade.php: can('manage memberships') && config).
	var memView *views.MemberMembershipView
	var payViews []views.MemberPaymentView
	showMembership := h.membershipsEnabled && can(r, "manage memberships")
	if showMembership {
		mship, merr := h.memberships.LatestForUser(r.Context(), id)
		switch {
		case merr == nil:
			v := membershipView(mship)
			memView = &v
		case errors.Is(merr, membership.ErrNotFound):
			// No membership yet — the view renders the "none" copy.
		default:
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		pg, perr := h.payments.ListForUser(r.Context(), id, 1)
		if perr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		pays := pg.Payments
		if len(pays) > 5 {
			pays = pays[:5]
		}
		payViews = paymentViews(pays)
	}
	flash := middleware.FlashFromContext(r.Context())
	views.MemberShowPage(views.MemberShowPageData{
		Sidebar:         membersSidebar(h.appName, r, u),
		Member:          memberView(m),
		IsAdmin:         isAdmin,
		ShowControls:    showTransitions,
		Transitions:     transitions,
		ShowMembership:  showMembership,
		Membership:      memView,
		Payments:        payViews,
		ShowAssignRoles: isAdmin && m.State == user.StateActive && m.ID != u.ID,
		Err:             showErr(flash),
		CSRF:            middleware.TokenFromContext(r.Context()),
	}).Render(r.Context(), w)
}

// UpdateState handles PUT /dashboard/users/{id}/state (reference
// UserStateController@update). Field errors re-render the show page's form;
// the reference's three guard errors redirect back with a flash.
func (h *Members) UpdateState(w http.ResponseWriter, r *http.Request) {
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
	state := r.FormValue("state")
	reason := r.FormValue("reason")
	err := h.svc.ChangeState(r.Context(), u.ID, id, state, reason)
	if err == nil {
		middleware.RedirectWithFlash(w, r, "/dashboard/users", "status", "user-state-updated")
		return
	}
	switch {
	case errors.Is(err, memberdirectory.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, memberdirectory.ErrUnverified):
		middleware.RedirectWithFlash(w, r, "/dashboard/users/"+strconv.FormatInt(id, 10), "error", "unverified-user-no-transition")
	case errors.Is(err, memberdirectory.ErrSelfChange):
		middleware.RedirectWithFlash(w, r, "/dashboard/users/"+strconv.FormatInt(id, 10), "error", "cannot-change-own-state")
	case errors.Is(err, memberdirectory.ErrInvalidTransition):
		middleware.RedirectWithFlash(w, r, "/dashboard/users", "error", "invalid-state-transition")
	default:
		var fieldErrs memberdirectory.FieldErrors
		if errors.As(err, &fieldErrs) {
			middleware.RedirectWithFlash(w, r, "/dashboard/users/"+strconv.FormatInt(id, 10), "error", fieldErrs.Error())
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// membersSidebar builds the dashboard chrome for the member pages.
func membersSidebar(appName string, r *http.Request, u user.User) views.SidebarData {
	return views.NewSidebarData(appName, middleware.TokenFromContext(r.Context()), u.Name, u.Email, "/dashboard/users", string(u.State), middleware.PermissionsFromContext(r.Context()))
}

// validFilterArg mirrors the reference's $allowed filter list (kept in sync
// with the service's validFilter).
func validFilterArg(f string) bool {
	switch f {
	case "pending", "unverified", "rejected", "suspended", "active", "all":
		return true
	}
	return false
}

// transitionViews maps the state machine's allowed transitions to the view
// buttons (reference users/show.blade.php: description + label per target,
// reason modal only for rejected/suspended).
func transitionViews(targetID int64, states []user.UserState, csrf string) []views.TransitionView {
	out := make([]views.TransitionView, 0, len(states))
	target := "/dashboard/users/" + strconv.FormatInt(targetID, 10) + "/state"
	for _, s := range states {
		v := views.TransitionView{Value: string(s), CSRF: csrf, Target: target}
		switch s {
		case user.StateActive:
			v.Label, v.Description, v.Kind = "Activate", "Approve this member and let them sign in.", "gold"
		case user.StateSuspended:
			v.Label, v.Description, v.NeedsReason = "Suspend", "Block sign-in for now. Their profile stays on file.", true
		case user.StateRejected:
			v.Label, v.Description, v.NeedsReason, v.Kind = "Reject", "Decline the application and block sign-in.", true, "error"
		case user.StatePending:
			v.Label, v.Description = "Move to Pending", "Send the member back to the review queue."
		}
		out = append(out, v)
	}
	return out
}

// showErr picks the show-page error flash (the state guard messages ride the
// "error" key).
func showErr(flash map[string]string) string {
	return flash["error"]
}

// memberCards maps service cards to the directory grid view model.
func memberCards(cards []memberdirectory.MemberCard, actionLabel string) []views.MemberCardView {
	out := make([]views.MemberCardView, 0, len(cards))
	for _, c := range cards {
		v := views.MemberCardView{
			ID:          c.ID,
			Name:        c.Name,
			Email:       c.Email,
			State:       string(c.State),
			PhotoURL:    photoURL(c.PhotoPath),
			Initials:    views.Initials(c.Name),
			Roles:       strings.Join(c.Roles, ", "),
			ActionLabel: actionLabel,
		}
		if c.EducationLevel != "" && c.EducationInstitution != "" {
			v.Education = c.EducationLevel + " · " + c.EducationInstitution
		}
		if c.CareerJobTitle != "" && c.CareerCompany != "" {
			v.Career = c.CareerJobTitle + " · " + c.CareerCompany
		}
		out = append(out, v)
	}
	return out
}

// memberView maps one member to the show-page view model (reference
// users/show.blade.php identity rail + narrative).
func memberView(m memberdirectory.Member) views.MemberProfileView {
	v := views.MemberProfileView{
		ID:                m.ID,
		Name:              m.Name,
		Email:             m.Email,
		State:             string(m.State),
		PhotoURL:          photoURL(m.PhotoPath),
		Initials:          views.Initials(m.Name),
		Roles:             m.Roles,
		MemberSince:       m.CreatedAt.Format("Jan 2006"),
		DOB:               m.DateOfBirth,
		Gender:            deref(m.Gender),
		BloodGroup:        deref(m.BloodGroup),
		PresentAddress:    deref(m.PresentAddress),
		PermanentAddress:  deref(m.PermanentAddress),
		Website:           deref(m.Website),
		Socials:           socialLinks(m.SocialLinks),
		EmergencyName:     m.EmergencyContact["name"],
		EmergencyRelation: m.EmergencyContact["relation"],
		EmergencyPhone:    m.EmergencyContact["phone"],
		HasProfile:        m.HasProfile,
	}
	// Admin-only lines (phone, addresses, emergency contact) are gated in the
	// view by d.IsAdmin, mirroring the reference's $isAdmin checks.
	if v.Website != "" {
		v.WebsiteURL = absoluteURL(v.Website)
	}
	// Reference: firstWhere('is_current') ?? first() over the start_year-desc
	// list — the repo returns careers already sorted that way.
	if len(m.Careers) > 0 {
		v.CurrentJobTitle = m.Careers[0].JobTitle
		for _, c := range m.Careers {
			if c.IsCurrent {
				v.CurrentJobTitle = c.JobTitle
				break
			}
		}
	}
	for _, e := range m.Educations {
		view := views.MemberEducationView{
			Period:      strconv.FormatInt(int64(e.StartYear), 10) + " — " + endYearOrPresent(e.EndYear),
			Institution: e.Institution,
			LevelLine:   e.Level,
			StudentID:   deref(e.StudentID),
		}
		if e.Subject != "" {
			view.LevelLine += " · " + e.Subject
		}
		v.Educations = append(v.Educations, view)
	}
	for _, c := range m.Careers {
		view := views.MemberCareerView{
			Period:      strconv.FormatInt(int64(c.StartYear), 10) + " — " + yearOrPresent(c.EndYear, c.IsCurrent),
			Title:       c.JobTitle,
			TypeLabel:   employmentLabel(c.EmploymentType),
			Company:     c.Company,
			Description: deref(c.Description),
		}
		if c.Industry != nil && c.Location != nil {
			view.Meta = *c.Industry + " · " + *c.Location
		}
		v.Careers = append(v.Careers, view)
	}
	return v
}

// socialLinks orders and labels the social icons the way the reference
// foreach does (linkedin, facebook).
func socialLinks(links map[string]string) []views.SocialLink {
	out := []views.SocialLink{}
	for _, key := range []string{"linkedin", "facebook"} {
		url, ok := links[key]
		if !ok || url == "" {
			continue
		}
		label := "Facebook"
		if key == "linkedin" {
			label = "LinkedIn"
		}
		out = append(out, views.SocialLink{Key: key, URL: absoluteURL(url), Label: label})
	}
	return out
}

// absoluteURL mirrors the reference filter_var($url, FILTER_VALIDATE_URL)
// check: absolute URLs pass, bare domains get https:// prefixed.
func absoluteURL(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	return "https://" + u
}

// yearOrPresent renders an education/career end: "Present" when current,
// the year otherwise, "—" when neither (reference show page).
func yearOrPresent(endYear *int32, current bool) string {
	if current {
		return "Present"
	}
	if endYear != nil {
		return strconv.FormatInt(int64(*endYear), 10)
	}
	return "—"
}

// endYearOrPresent renders an education end the way the reference show page
// does: the year when set, "Present" otherwise (no is_current check).
func endYearOrPresent(endYear *int32) string {
	if endYear != nil {
		return strconv.FormatInt(int64(*endYear), 10)
	}
	return "Present"
}

// membershipView maps a membership to the show-page summary box (reference
// users/show.blade.php: effective-status badge + start → end dates).
func membershipView(m membership.Membership) views.MemberMembershipView {
	planName := m.Plan.Name
	if planName == "" {
		planName = "—"
	}
	status := m.EffectiveStatus()
	v := views.MemberMembershipView{
		PlanName:    planName,
		StatusLabel: membershipStatusLabel(status),
		StatusClass: membershipStatusClass(status),
		Starts:      "—",
		Ends:        "Never",
	}
	if m.StartsAt != nil {
		v.Starts = m.StartsAt.Format("02 Jan 2006")
	}
	if m.EndsAt != nil {
		v.Ends = m.EndsAt.Format("02 Jan 2006")
	}
	return v
}

// membershipStatusLabel maps the membership.status_* lang labels.
func membershipStatusLabel(s membership.Status) string {
	switch s {
	case membership.StatusActive:
		return "Active"
	case membership.StatusExpired:
		return "Expired"
	case membership.StatusCancelled:
		return "Cancelled"
	}
	return string(s)
}

// membershipStatusClass maps the reference membership pill colours.
func membershipStatusClass(s membership.Status) string {
	switch s {
	case membership.StatusActive:
		return "bg-emerald-100 text-emerald-800"
	case membership.StatusExpired:
		return "bg-amber-100 text-amber-800"
	}
	return "bg-gray-100 text-gray-600"
}

// paymentViews maps the latest payments to their summary rows (reference
// users/show.blade.php payments list).
func paymentViews(pays []membershippayment.Payment) []views.MemberPaymentView {
	out := make([]views.MemberPaymentView, 0, len(pays))
	for _, p := range pays {
		planName := p.PlanName
		if planName == "" {
			planName = "—"
		}
		out = append(out, views.MemberPaymentView{
			PlanName:    planName,
			PaidAt:      p.PaidAt.Format("02 Jan 2006"),
			StatusLabel: paymentStatusLabel(p.Status),
			StatusClass: paymentStatusClass(p.Status),
		})
	}
	return out
}

// paymentStatusLabel/Class mirror the reference payment pill (approved
// emerald, pending amber, everything else gray).
func paymentStatusLabel(s membershippayment.Status) string {
	switch s {
	case membershippayment.StatusPending:
		return "Pending"
	case membershippayment.StatusApproved:
		return "Approved"
	}
	return "Rejected"
}

func paymentStatusClass(s membershippayment.Status) string {
	switch s {
	case membershippayment.StatusApproved:
		return "bg-emerald-100 text-emerald-800"
	case membershippayment.StatusPending:
		return "bg-amber-100 text-amber-800"
	}
	return "bg-gray-100 text-gray-600"
}

// employmentLabel maps config('career.employment_types') labels.
func employmentLabel(v string) string {
	switch v {
	case "full_time":
		return "Full-Time"
	case "part_time":
		return "Part-Time"
	case "contract":
		return "Contract"
	case "freelance":
		return "Freelance"
	case "internship":
		return "Internship"
	default:
		return v
	}
}
