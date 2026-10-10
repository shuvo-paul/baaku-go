package views

import (
	"net/url"
	"strconv"
)

// Initials is the exported form of the unexported initials helper (the
// member-directory handler builds card view models with it).
func Initials(name string) string { return initials(name) }

// MembersPageData is the member directory index (reference
// resources/views/users/index.blade.php inside layouts/dashboard).
type MembersPageData struct {
	Sidebar SidebarData
	IsAdmin bool
	Members []MemberCardView
	Filter  string // pending/unverified/active/rejected/suspended/all
	Search  string
	Counts  map[string]int64 // state → count (admin header); nil for members
	Total   int64
	Page    int64
	HasPrev bool
	HasNext bool
	Flash   string // user-state-updated
	Err     string // invalid-state-transition / cannot-change-own-state / unverified-user-no-transition
	CSRF    string
}

// MemberCardView is one grid card in the directory.
type MemberCardView struct {
	ID          int64
	Name        string
	Email       string
	State       string
	PhotoURL    string
	Initials    string
	Roles       string // comma-joined role names
	Education   string // "level · institution" or ""
	Career      string // "job_title · company" or ""
	ActionLabel string // "Review & approve" when filter=pending, else "View profile"
}

// filterOptions is the admin status filter list (reference index.blade.php).
var filterOptions = []struct{ Value, Label string }{
	{"pending", "Pending"},
	{"unverified", "Unverified"},
	{"active", "Active"},
	{"rejected", "Rejected"},
	{"suspended", "Suspended"},
	{"all", "All"},
}

// FilterLabel renders the currently selected filter's label.
func (d MembersPageData) FilterLabel() string {
	for _, f := range filterOptions {
		if f.Value == d.Filter {
			return f.Label
		}
	}
	return "All"
}

// FilterOptions exposes the filter list to the template.
func (d MembersPageData) FilterOptions() []struct{ Value, Label string } {
	return filterOptions
}

// Summary mirrors the reference summary line: filtered > state > all.
func (d MembersPageData) Summary() string {
	if d.Search != "" {
		return strconv.FormatInt(d.Total, 10) + " members found"
	}
	if d.Filter == "all" || d.Filter == "" {
		return strconv.FormatInt(arraySum(d.Counts), 10) + " members total"
	}
	n := d.Counts[d.Filter]
	return strconv.FormatInt(n, 10) + " " + lower(d.FilterLabel()) + " members"
}

// EmptyMessage mirrors the reference empty-state copy.
func (d MembersPageData) EmptyMessage() string {
	switch d.Filter {
	case "pending":
		return "No users awaiting approval."
	case "unverified":
		return "No users awaiting email verification."
	default:
		return "No users found."
	}
}

// pageHref builds a directory URL carrying filter, search and page.
// pageHref builds a directory URL carrying filter, search and page.
func (d MembersPageData) pageHref(page int64) string {
	out := "/dashboard/users?"
	if d.Filter != "" && d.Filter != "all" {
		out += "filter=" + d.Filter + "&"
	}
	if d.Search != "" {
		out += "search=" + url.QueryEscape(d.Search) + "&"
	}
	return out + "page=" + strconv.FormatInt(page, 10)
}

// PrevHref is the previous-page URL ("" when on the first page).
func (d MembersPageData) PrevHref() string {
	if !d.HasPrev {
		return ""
	}
	return d.pageHref(d.Page - 1)
}

// NextHref is the next-page URL ("" when on the last page).
func (d MembersPageData) NextHref() string {
	if !d.HasNext {
		return ""
	}
	return d.pageHref(d.Page + 1)
}

func arraySum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}

// MemberShowPageData is one member's profile page (reference
// resources/views/users/show.blade.php inside layouts/dashboard).
type MemberShowPageData struct {
	Sidebar      SidebarData
	Member       MemberProfileView
	IsAdmin      bool
	ShowControls bool // admin + verified + not-self (the reference gate)
	Transitions  []TransitionView
	// ShowMembership gates the reference membership summary box (viewer can
	// "manage memberships" && features.memberships).
	ShowMembership bool
	Membership     *MemberMembershipView // nil = no membership yet
	Payments       []MemberPaymentView   // latest 5
	// ShowAssignRoles renders the assign-roles link (admin + target active +
	// not self; reference users/show.blade.php).
	ShowAssignRoles bool
	Err             string
	CSRF            string
}

// MemberProfileView is the identity rail + narrative for one member.
type MemberProfileView struct {
	ID          int64
	Name        string
	Email       string
	Phone       string // admin only; "" hides the line
	State       string
	PhotoURL    string
	Initials    string
	Roles       []string
	MemberSince string // "Jan 2006"
	// CurrentJobTitle rides above the name (reference label-caps gold).
	CurrentJobTitle string
	// Profile details.
	DOB               string // Y-m-d or ""
	Gender            string
	BloodGroup        string
	PresentAddress    string // admin only
	PermanentAddress  string // admin only
	Website           string
	WebsiteURL        string
	Socials           []SocialLink
	EmergencyName     string // admin only
	EmergencyRelation string
	EmergencyPhone    string
	// HasProfile reports whether a profiles row exists (reference
	// users/show.blade.php `if (! $profile)`).
	HasProfile bool
	// Narrative lists.
	Educations []MemberEducationView
	Careers    []MemberCareerView
}

// MemberMembershipView is one membership summary row (reference
// users/show.blade.php membership box).
type MemberMembershipView struct {
	PlanName    string // "—" when the plan is missing
	StatusLabel string // Active / Expired / Cancelled (effective status)
	StatusClass string
	Starts      string // "02 Jan 2006" or "—"
	Ends        string // "02 Jan 2006" or "Never"
}

// MemberPaymentView is one payment row in the membership box (latest 5).
type MemberPaymentView struct {
	PlanName    string // "—" when the plan is missing
	PaidAt      string // "02 Jan 2006"
	StatusLabel string // Pending / Approved / Rejected
	StatusClass string
}

// SocialLink is one rendered social icon (linkedin/facebook).

type SocialLink struct {
	Key   string
	URL   string
	Label string
}

// MemberEducationView is one education row on the show narrative.
type MemberEducationView struct {
	Period      string // "2020 — 2024" / "2020 — Present"
	Institution string
	LevelLine   string // "level" or "level · subject"
	StudentID   string
}

// MemberCareerView is one career row on the show narrative.
type MemberCareerView struct {
	Period      string
	Title       string
	TypeLabel   string
	Company     string
	Meta        string // "industry · location"
	Description string
}

// TransitionView is one allowed state transition button.
type TransitionView struct {
	Value       string
	Label       string
	Description string
	NeedsReason bool
	// Kind picks the button style: "gold" (active), "error" (rejected),
	// "" = secondary (reference users/show.blade.php @if chain).
	Kind string
	CSRF string
	// Target is the form action: POST /dashboard/users/{id}/state (spoofed PUT).
	Target string
}

// memberFlashMessage maps the index status-flash slugs to the reference's
// session copy (lang dashboard.php).
func memberFlashMessage(key string) string {
	switch key {
	case "user-state-updated":
		return "User state updated successfully."
	case "user-roles-updated":
		return "User roles updated successfully."
	}
	return key
}

// memberErrMessage maps the users error-flash slugs to the reference copy;
// validation messages already carry their text and pass through.
func memberErrMessage(key string) string {
	switch key {
	case "invalid-state-transition":
		return "Invalid state transition."
	case "cannot-change-own-state":
		return "You cannot change your own membership state."
	case "unverified-user-no-transition":
		return "Email must be verified before membership actions can be taken."
	}
	return key
}

// memberSearchData is the Alpine object powering the reference's debounced
// live search (users/index.blade.php x-data + fetch XHR branch).
func memberSearchData(search string) string {
	return `{
		search: ` + strconv.Quote(search) + `,
		controller: null,
		searchUsers() {
			if (this.controller) this.controller.abort();
			this.controller = new AbortController();
			const filter = document.getElementById('filter').value;
			fetch('/dashboard/users?filter=' + encodeURIComponent(filter) + '&search=' + encodeURIComponent(this.search), {
				headers: { 'X-Requested-With': 'XMLHttpRequest' },
				signal: this.controller.signal
			})
			.then(r => r.json())
			.then(data => {
				document.getElementById('user-grid').innerHTML = data.grid;
				document.getElementById('user-pagination').innerHTML = data.pagination;
			})
			.catch(e => { if (e.name !== 'AbortError') throw e; });
		}
	}`
}

// transitionButtonClass picks the transition button styling (reference
// users/show.blade.php: active gold, rejected error outline, else secondary).
func transitionButtonClass(t TransitionView) string {
	switch t.Kind {
	case "gold":
		return "inline-flex items-center justify-center rounded bg-gold px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-gold/90 focus-visible:ring-2 focus-visible:ring-gold/50"
	case "error":
		return "inline-flex items-center justify-center rounded border border-error px-4 py-2 text-sm font-semibold text-error transition-colors hover:bg-error hover:text-white focus-visible:ring-2 focus-visible:ring-error/50"
	}
	return "btn-secondary"
}
