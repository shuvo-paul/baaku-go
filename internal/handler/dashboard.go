package handler

import (
	"net/http"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/views"
)

// Dashboard is the post-login landing (reference DashboardController rendered
// inside layouts/dashboard).
type Dashboard struct {
	appName string
}

func NewDashboard(appName string) *Dashboard {
	return &Dashboard{appName: appName}
}

// Show renders GET /dashboard (auth + verified guards applied on the route).
func (h *Dashboard) Show(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.Redirect(w, r, middleware.LoginPath)
		return
	}
	csrf := middleware.TokenFromContext(r.Context())
	views.DashboardPage(views.DashboardPageData{
		Sidebar: views.NewSidebarData(h.appName, csrf, u.Name, u.Email, "/dashboard", string(u.State), middleware.PermissionsFromContext(r.Context())),
		Name:    u.Name,
		State:   string(u.State),
		Status:  middleware.FlashFromContext(r.Context())["status"],
	}).Render(r.Context(), w)
}
