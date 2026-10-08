package handler

import (
	"net/http"

	"github.com/shuvo-paul/baaku/internal/middleware"
	"github.com/shuvo-paul/baaku/internal/views"
)

// Dashboard is the minimal post-login landing (reference DashboardController
// behind auth + verified; full content is a later wave).
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
	views.DashboardPage(h.appName, middleware.TokenFromContext(r.Context()), u.Name).Render(r.Context(), w)
}
