package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/views"
)

// ActivityFeed lists one page of the admin activity feed; *activitylog
// .Service satisfies it.
type ActivityFeed interface {
	Recent(ctx context.Context, page int) ([]activitylog.Item, bool, error)
}

// ActivityLog renders GET /dashboard/activity-log (reference
// ActivityLogController@index, behind permission:view activity log).
type ActivityLog struct {
	feed    ActivityFeed
	appName string
}

func NewActivityLog(feed ActivityFeed, appName string) *ActivityLog {
	return &ActivityLog{feed: feed, appName: appName}
}

// Show renders the newest-first activity page. ?page= mirrors Laravel's
// simplePaginate (1-based, invalid values fall back to 1 in the service).
func (h *ActivityLog) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	items, more, err := h.feed.Recent(r.Context(), page)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.ActivitiesPage(views.ActivityPageData{
		Sidebar: views.NewSidebarData(h.appName, middleware.TokenFromContext(r.Context()), u.Name, u.Email, "/dashboard/activity-log", string(u.State), middleware.PermissionsFromContext(r.Context())),
		Items:   toActivityViewItems(items),
		HasMore: more,
		Page:    page,
	}).Render(r.Context(), w)
}

// toActivityViewItems maps service items to the view shape.
func toActivityViewItems(items []activitylog.Item) []views.ActivityItem {
	out := make([]views.ActivityItem, 0, len(items))
	for _, it := range items {
		out = append(out, views.ActivityItem{
			Domain:       activityDomain(it.LogName),
			Verb:         activityVerb(it.LogName, it.Event),
			Actor:        it.Actor,
			SubjectLabel: activitySubjectLabel(it.LogName),
			SubjectName:  it.SubjectName,
			SubjectURL:   activitySubjectURL(it.LogName, it.SubjectKind, it.SubjectID),
			Properties:   activityProps(it.Properties),
			CreatedAt:    it.CreatedAt,
			ISOTime:      it.CreatedAt.Format(time.RFC3339),
		})
	}
	return out
}

// Label maps port reference lang/en/activity_log.php for the three logged
// domains; unknown domains/events fall back to the raw value like the
// reference's `?? $activity->event` / `?? $domain`.
func activityDomain(logName string) string {
	switch logName {
	case "member_management":
		return "Membership"
	case "role_management":
		return "Roles"
	case "profile":
		return "Profiles"
	default:
		return logName
	}
}

func activityVerb(logName, event string) string {
	switch logName + "/" + event {
	case "member_management/state_changed":
		return "Membership state changed"
	case "member_management/roles_synced":
		return "Roles updated"
	case "role_management/created":
		return "Role created"
	case "role_management/updated":
		return "Role updated"
	case "role_management/deleted":
		return "Role deleted"
	case "profile/submitted":
		return "Profile submitted"
	case "profile/resubmitted":
		return "Profile resubmitted for review"
	default:
		return event
	}
}

func activitySubjectLabel(logName string) string {
	switch logName {
	case "role_management":
		return "role"
	case "profile":
		return "profile"
	default:
		return "member"
	}
}

// activitySubjectURL links a subject to its dashboard page, but only once
// those routes exist in the Go port (roles edit is live; users show lands in
// the users wave). Until then it returns "" and the view renders the subject
// as plain text — same "only live routes" rule as the sidebar.
func activitySubjectURL(logName, kind string, id int64) string {
	_ = logName
	switch kind {
	case activitylog.KindRole:
		return "/dashboard/roles/" + strconv.FormatInt(id, 10) + "/edit"
	case activitylog.KindUser:
		return "/dashboard/users/" + strconv.FormatInt(id, 10)
	}
	return ""
}

// activityProps flattens the spatie properties bag into ordered pairs for
// the view (the reference renders arrays comma-joined, empty arrays as None).
func activityProps(props map[string]any) []views.ActivityProp {
	if len(props) == 0 {
		return nil
	}
	out := make([]views.ActivityProp, 0, len(props))
	for k, v := range props {
		out = append(out, views.ActivityProp{Key: k, Value: activityPropValue(v)})
	}
	return out
}

func activityPropValue(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case string:
		return t
	case []any:
		if len(t) == 0 {
			return "None"
		}
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, activityPropValue(e))
		}
		return joinCommas(parts)
	default:
		return strconv.FormatFloat(toFloat64(t), 'f', -1, 64)
	}
}

func toFloat64(v any) float64 {
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func joinCommas(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
