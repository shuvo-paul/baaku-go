package handler

import (
	"net/http"

	"github.com/shuvo-paul/baaku/internal/service/committee"
	"github.com/shuvo-paul/baaku/internal/views"
)

// Homepage serves GET / (reference routes/web.php view('homepage')): hero,
// about, committee teaser and the blogs grid.
type Homepage struct {
	svc *committee.Service
}

func NewHomepage(svc *committee.Service) *Homepage {
	return &Homepage{svc: svc}
}

func (h *Homepage) Show(w http.ResponseWriter, r *http.Request) {
	members, err := h.svc.Members(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.HomePage(views.HomePageData{
		Title:   "বাংলা অ্যালামনাই অ্যাসোসিয়েশন",
		Members: publicMembers(members),
		Posts:   views.Posts(),
	}).Render(r.Context(), w)
}

// CommitteeFull serves GET /committee (reference routes/web.php
// view('committee-full')).
type CommitteeFull struct {
	svc *committee.Service
}

func NewCommitteeFull(svc *committee.Service) *CommitteeFull {
	return &CommitteeFull{svc: svc}
}

func (h *CommitteeFull) Show(w http.ResponseWriter, r *http.Request) {
	members, err := h.svc.Members(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	views.CommitteeFullPage(views.CommitteeFullData{
		Title:   "কার্যনির্বাহী কমিটি — বাকু",
		Members: publicMembers(members),
	}).Render(r.Context(), w)
}

// publicMembers maps committee members to the public-facing view models
// (reference App\Committee@all).
func publicMembers(members []committee.Member) []views.PublicMember {
	out := make([]views.PublicMember, 0, len(members))
	for _, m := range members {
		out = append(out, views.PublicMember{
			Role:    deref(m.PositionName),
			Name:    m.DisplayName(),
			Image:   m.PhotoURL(),
			Vacant:  m.Vacant(),
			Initial: m.Initial(),
		})
	}
	return out
}
