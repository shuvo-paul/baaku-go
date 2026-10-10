// Package memberdirectory — dashboard member directory and membership-state
// management (reference UserRoleController@index/show + UserStateController
// @update over the spatie activity log and Laravel notifications).
package memberdirectory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shuvo-paul/baaku/internal/mailer"
	"github.com/shuvo-paul/baaku/internal/service/activitylog"
	"github.com/shuvo-paul/baaku/internal/service/career"
	"github.com/shuvo-paul/baaku/internal/service/education"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// MemberCard is one grid card: the user plus the summary lines the reference
// eager-loads roles/profile.educations/profile.careers for.
type MemberCard struct {
	ID                   int64
	Name                 string
	Email                string
	State                user.UserState
	EmailVerified        bool
	CreatedAt            time.Time
	PhotoPath            *string
	Roles                []string
	EducationLevel       string
	EducationInstitution string
	CareerJobTitle       string
	CareerCompany        string
}

// Member is the show-page profile: MemberCard data plus the profile details
// the identity rail renders.
type Member struct {
	MemberCard
	Phone            *string
	DateOfBirth      string // formatted YYYY-MM-DD, "" = none (reference format('Y-m-d'))
	Gender           *string
	BloodGroup       *string
	PresentAddress   *string
	PermanentAddress *string
	SocialLinks      map[string]string
	Website          *string
	EmergencyContact map[string]string
	// Educations and Careers are the show-page narrative lists (the reference
	// eager-loads profile.educations / profile.careers); the service composes
	// them from the education/career listers, so the repo leaves them nil.
	Educations []education.Education
	Careers    []career.Career
}

// Page is one directory page of members plus the paginator total.
type Page struct {
	Members []MemberCard
	Total   int64
}

// ListOptions mirrors the reference index(): filter state ("" = all), trimmed
// search, activeOnly for the non-admin branch, 24 per page.
type ListOptions struct {
	State      string
	Search     string
	ActiveOnly bool
	PerPage    int64
	Offset     int64
}

// Store is the member-directory persistence; *repository/memberdirectory
// .Repo satisfies it.
type Store interface {
	List(ctx context.Context, opts ListOptions) (Page, error)
	CountsByState(ctx context.Context) (map[string]int64, error)
	Get(ctx context.Context, id int64, activeOnly bool) (Member, error)
	// SetState persists the membership state (reference $user->update(['state'
	// => ...])).
	SetState(ctx context.Context, id int64, state user.UserState) error
}

// ActivityLogger writes one spatie-shaped entry; *activitylog.Logger and test
// fakes satisfy it.
type ActivityLogger interface {
	Log(ctx context.Context, e activitylog.Entry) error
}

// Notifier delivers one member email; wiring binds it to the mailer-backed
// send func (activated/suspended/rejected notifications).
type Notifier func(to, subject, htmlBody string) error

// EducationLister and CareerLister supply the show-page narrative lists;
// *service/education.Service and *service/career.Service satisfy them.
type EducationLister interface {
	List(ctx context.Context, userID int64) ([]education.Education, error)
}

type CareerLister interface {
	List(ctx context.Context, userID int64) ([]career.Career, error)
}

// PerPage is Laravel's paginator default for the directory (paginate(24)).
const PerPage = 24

// Service is the member directory + state transitions.
type Service struct {
	store      Store
	educations EducationLister
	careers    CareerLister
	logger     ActivityLogger
	notify     Notifier
	appURL     string
}

// New wires the service. store, logger and notify are the real repo, the
// spatie activity logger and the mailer send func; tests pass fakes.
func New(store Store, educations EducationLister, careers CareerLister, logger ActivityLogger, notify Notifier, appURL string) *Service {
	return &Service{store: store, educations: educations, careers: careers, logger: logger, notify: notify, appURL: appURL}
}

// ErrNotFound is a missing (or invisible-to-viewer) member; handlers 404.
var ErrNotFound = errors.New("member not found")

// ErrUnverified is the reference unverified_user_no_transition guard.
var ErrUnverified = errors.New("email must be verified before membership actions")

// ErrSelfChange is the reference cannot_change_own_state self-lockout guard.
var ErrSelfChange = errors.New("you cannot change your own membership state")

// ErrInvalidTransition is the reference invalid_state_transition guard.
var ErrInvalidTransition = errors.New("invalid state transition")

// FieldErrors re-renders the form (same shape as the role service).
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Directory returns one page of member cards plus the header counts. isAdmin
// mirrors the reference branches: admins see every state with counts/search/
// filters, everyone else sees only active members.
func (s *Service) Directory(ctx context.Context, isAdmin bool, filter, search string, page int64) (Page, map[string]int64, error) {
	opts := ListOptions{Search: search, PerPage: PerPage, Offset: (page - 1) * PerPage}
	if isAdmin {
		if !validFilter(filter) {
			filter = "all"
		}
		if filter != "all" {
			opts.State = filter
		}
	} else {
		isAdmin = false
		opts.ActiveOnly = true
	}
	if page < 1 {
		opts.Offset = 0
	}
	pg, err := s.store.List(ctx, opts)
	if err != nil {
		return Page{}, nil, err
	}
	counts := map[string]int64{}
	if isAdmin {
		counts, err = s.store.CountsByState(ctx)
		if err != nil {
			return Page{}, nil, err
		}
	}
	return pg, counts, nil
}

// Get returns one member for the show page; non-admins only see active users
// (reference show() findOrFail scope).
func (s *Service) Get(ctx context.Context, isAdmin bool, id int64) (Member, error) {
	m, err := s.store.Get(ctx, id, !isAdmin)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, ErrNotFound
		}
		return Member{}, err
	}
	m.Educations, err = s.educations.List(ctx, id)
	if err != nil {
		return Member{}, err
	}
	m.Careers, err = s.careers.List(ctx, id)
	if err != nil {
		return Member{}, err
	}
	return m, nil
}

// ChangeState applies a membership-state transition (reference
// UserStateController@update): validates the input, guards unverified/self/
// invalid transitions, persists, writes the member_management/state_changed
// activity entry and notifies the member by email.
func (s *Service) ChangeState(ctx context.Context, actorID, targetID int64, stateVal, reason string) error {
	target, err := s.store.Get(ctx, targetID, false)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	errs := FieldErrors{}
	newState := user.UserState(strings.TrimSpace(stateVal))
	if !validState(newState) {
		errs["state"] = "The selected state is invalid."
	}
	reason = strings.TrimSpace(reason)
	if (newState == user.StateRejected || newState == user.StateSuspended) && reason == "" {
		errs["reason"] = "The reason field is required when rejecting or suspending."
	}
	if len(reason) > 2000 {
		errs["reason"] = "The reason must not be greater than 2000 characters."
	}
	if len(errs) > 0 {
		return errs
	}
	// Reference guards, same order: unverified target, then self-lockout, then
	// the state machine.
	if !target.EmailVerified {
		return ErrUnverified
	}
	if actorID == targetID {
		return ErrSelfChange
	}
	if !target.State.CanTransitionTo(newState) {
		return ErrInvalidTransition
	}
	if err := s.store.SetState(ctx, targetID, newState); err != nil {
		return err
	}
	if err := s.logger.Log(ctx, activitylog.Entry{
		LogName:     "member_management",
		Event:       "state_changed",
		Description: "member state changed",
		Subject:     &activitylog.Subject{Kind: activitylog.KindUser, ID: targetID},
		Properties: map[string]any{
			"old_state": string(target.State),
			"new_state": string(newState),
			"reason":    reason,
		},
		Causer: &activitylog.Causer{ID: actorID},
	}); err != nil {
		return err
	}
	return s.notifyState(ctx, target, newState, reason)
}

// notifyState sends the reference's user notifications: activated (no extra
// lines), suspended and rejected (reason line + reapply line).
func (s *Service) notifyState(ctx context.Context, target Member, state user.UserState, reason string) error {
	if s.notify == nil {
		return nil
	}
	var subject, body string
	var err error
	switch state {
	case user.StateRejected:
		subject, body, err = mailer.UserRejected(mailer.UserRejectedData{Reason: reason})
	case user.StateSuspended:
		subject, body, err = mailer.UserSuspended(mailer.UserSuspendedData{Reason: reason})
	case user.StateActive:
		subject, body, err = mailer.UserActivated(mailer.UserActivatedData{URL: s.appURL + "/dashboard"})
	default:
		return nil
	}
	if err != nil {
		return fmt.Errorf("memberdirectory: render %s notification: %w", state, err)
	}
	if err := s.notify(target.Email, subject, body); err != nil {
		return fmt.Errorf("memberdirectory: notify %s: %w", target.Email, err)
	}
	return nil
}

// validFilter mirrors the reference $allowed list.
func validFilter(f string) bool {
	switch f {
	case "pending", "unverified", "rejected", "suspended", "active", "all":
		return true
	}
	return false
}

// validState mirrors UserState::cases() for the required+in: validation.
func validState(s user.UserState) bool {
	switch s {
	case user.StateUnverified, user.StatePending, user.StateActive, user.StateSuspended, user.StateRejected:
		return true
	}
	return false
}
