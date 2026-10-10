// Package committee ports reference App\Services\Committee, App\Committee and
// the CommitteeController + PositionController (dashboard CRUD, drag-reorder,
// member search) plus the public committee listings.
package committee

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/jackc/pgx/v5"
)

// Member is the domain representation of a committee_members row with its
// position name and registered-user profile joined on (reference
// CommitteeMember with position/user eager-loaded).
type Member struct {
	ID            int64
	PositionID    *int64
	PositionName  *string
	UserID        *int64
	UserName      *string // registered user's name (user relation)
	UserPhotoPath *string // registered user's profile photo
	UserEmail     *string // registered user's email (member-picker label)
	Name          *string // free-text name for non-registered members
	PhotoPath     *string
	SortOrder     int32
}

// Position is one committee seat (reference App\Models\Position).
type Position struct {
	ID          int64
	Name        string
	MemberCount int64
}

// SearchUser is one member-picker result (reference UserSearch results rows).
type SearchUser struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// DisplayName prefers the registered user's name, then the free-text name,
// then an em dash (reference CommitteeMember@displayName).
func (m Member) DisplayName() string {
	if m.UserName != nil && *m.UserName != "" {
		return *m.UserName
	}
	if m.Name != nil && *m.Name != "" {
		return *m.Name
	}
	return "—"
}

// PhotoURL maps the stored photo_path to its streamed media URL, falling back
// to the registered user's profile photo (reference
// CommitteeMember@photoUrl).
func (m Member) PhotoURL() string {
	if m.PhotoPath != nil && *m.PhotoPath != "" {
		return "/media/committee-photos/" + filepath.Base(*m.PhotoPath)
	}
	if m.UserPhotoPath != nil && *m.UserPhotoPath != "" {
		return "/media/profile-photos/" + filepath.Base(*m.UserPhotoPath)
	}
	return ""
}

// Vacant marks a seat with neither a registered user nor a name (reference
// Committee@all 'vacant').
func (m Member) Vacant() bool {
	return m.UserID == nil && m.Name == nil
}

// Initial returns the first rune of the display name for the monogram
// placeholders (reference mb_substr($name, 0, 1)).
func (m Member) Initial() string {
	name := m.DisplayName()
	for _, r := range name {
		return string(r)
	}
	return ""
}

// ErrNotFound is a missing member or position (findOrFail → 404).
var ErrNotFound = errors.New("committee member not found")

// ErrPositionNotFound is a missing position.
var ErrPositionNotFound = errors.New("position not found")

// FieldErrors mirrors Laravel validator messages keyed by field.
type FieldErrors map[string]string

func (e FieldErrors) Error() string {
	for _, v := range e {
		return v
	}
	return ""
}

// Store is the persistence port; *repository/committee Repo satisfies it.
type Store interface {
	ListMembers(ctx context.Context) ([]Member, error)
	GetMember(ctx context.Context, id int64) (Member, error)
	MaxMemberSortOrder(ctx context.Context) (int32, error)
	CreateMember(ctx context.Context, m Member) (int64, error)
	UpdateMember(ctx context.Context, m Member) error
	DeleteMember(ctx context.Context, id int64) error
	ReorderMember(ctx context.Context, id int64, sort int32) error
	ListPositions(ctx context.Context) ([]Position, error)
	GetPosition(ctx context.Context, id int64) (Position, error)
	CreatePosition(ctx context.Context, name string) (int64, error)
	UpdatePosition(ctx context.Context, id int64, name string) error
	DeletePosition(ctx context.Context, id int64) error
	PositionNameExists(ctx context.Context, name string, excludeID int64) (bool, error)
	PositionIDExists(ctx context.Context, id int64) (bool, error)
	UserIDExists(ctx context.Context, id int64) (bool, error)
	SearchUsers(ctx context.Context, q string) ([]SearchUser, error)
}

// Service ports the committee + positions dashboards and the shared member
// queries used by the public site.
type Service struct {
	store Store
}

func New(store Store) *Service { return &Service{store: store} }

// Members returns every committee member sorted by dashboard order, with
// position and user joined (reference CommitteeService::members).
func (s *Service) Members(ctx context.Context) ([]Member, error) {
	return s.store.ListMembers(ctx)
}

// Get returns one member (404 via ErrNotFound).
func (s *Service) Get(ctx context.Context, id int64) (Member, error) {
	m, err := s.store.GetMember(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	return m, err
}

// StoreInput is the validated member payload (reference
// Store/UpdateCommitteeMemberRequest).
type StoreInput struct {
	PositionID *int64
	UserID     *int64
	Name       *string
	// PhotoPath is set by the handler when a photo upload was stored.
	PhotoPath *string
	// KeepPhoto preserves the existing photo on update when no new upload
	// arrived (the reference never clears a photo without replacing it).
	KeepPhoto bool
}

// Store creates a member. New members append to the end (max sort_order + 1,
// reference CommitteeController@store).
func (s *Service) Store(ctx context.Context, in StoreInput) (int64, error) {
	if errs := s.validate(ctx, in, 0); len(errs) > 0 {
		return 0, errs
	}
	max, err := s.store.MaxMemberSortOrder(ctx)
	if err != nil {
		return 0, err
	}
	return s.store.CreateMember(ctx, Member{
		PositionID: in.PositionID,
		UserID:     in.UserID,
		Name:       in.Name,
		PhotoPath:  in.PhotoPath,
		SortOrder:  max + 1,
	})
}

// Update edits a member (reference CommitteeController@update). The photo is
// replaced only when a new one was stored; otherwise the kept path wins.
func (s *Service) Update(ctx context.Context, id int64, in StoreInput) error {
	if errs := s.validate(ctx, in, id); len(errs) > 0 {
		return errs
	}
	existing, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	m := Member{
		ID:         id,
		PositionID: in.PositionID,
		UserID:     in.UserID,
		Name:       in.Name,
		PhotoPath:  in.PhotoPath,
		SortOrder:  existing.SortOrder,
	}
	if in.PhotoPath == nil && in.KeepPhoto {
		m.PhotoPath = existing.PhotoPath
	}
	return s.store.UpdateMember(ctx, m)
}

// Destroy deletes a member (reference CommitteeController@destroy — the
// handler removes the stored photo file first).
func (s *Service) Destroy(ctx context.Context, id int64) error {
	if _, err := s.Get(ctx, id); err != nil {
		return err
	}
	return s.store.DeleteMember(ctx, id)
}

// Reorder persists a new position for one member (reference
// CommitteeController@reorder).
func (s *Service) Reorder(ctx context.Context, id int64, sort int32) error {
	return s.store.ReorderMember(ctx, id, sort)
}

// Positions lists every position with its member count (reference
// PositionController@index).
func (s *Service) Positions(ctx context.Context) ([]Position, error) {
	return s.store.ListPositions(ctx)
}

// GetPosition returns one position (404 via ErrPositionNotFound).
func (s *Service) GetPosition(ctx context.Context, id int64) (Position, error) {
	p, err := s.store.GetPosition(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Position{}, ErrPositionNotFound
	}
	return p, err
}

// StorePosition creates a position (reference PositionController@store).
func (s *Service) StorePosition(ctx context.Context, name string) (int64, error) {
	if errs := s.validatePosition(ctx, name, 0); len(errs) > 0 {
		return 0, errs
	}
	return s.store.CreatePosition(ctx, name)
}

// UpdatePosition renames a position (reference PositionController@update).
func (s *Service) UpdatePosition(ctx context.Context, id int64, name string) error {
	if errs := s.validatePosition(ctx, name, id); len(errs) > 0 {
		return errs
	}
	if _, err := s.GetPosition(ctx, id); err != nil {
		return err
	}
	return s.store.UpdatePosition(ctx, id, name)
}

// DestroyPosition deletes a position (reference PositionController@destroy —
// members keep existing with a cleared position via FK nullOnDelete).
func (s *Service) DestroyPosition(ctx context.Context, id int64) error {
	if _, err := s.GetPosition(ctx, id); err != nil {
		return err
	}
	return s.store.DeletePosition(ctx, id)
}

// SearchUsers returns active users matching name/email — the member picker's
// live search (reference Livewire UserSearch: min 2 chars, limit 8).
func (s *Service) SearchUsers(ctx context.Context, q string) ([]SearchUser, error) {
	if len(q) < 2 {
		return []SearchUser{}, nil
	}
	return s.store.SearchUsers(ctx, q)
}

// validate enforces the Store/UpdateCommitteeMemberRequest rules.
func (s *Service) validate(ctx context.Context, in StoreInput, id int64) FieldErrors {
	errs := FieldErrors{}
	if in.PositionID == nil || *in.PositionID == 0 {
		errs["position_id"] = "The position id field is required."
	} else {
		ok, err := s.store.PositionIDExists(ctx, *in.PositionID)
		if err != nil || !ok {
			errs["position_id"] = "The selected position id is invalid."
		}
	}
	if in.UserID != nil && *in.UserID != 0 {
		ok, err := s.store.UserIDExists(ctx, *in.UserID)
		if err != nil || !ok {
			errs["user_id"] = "The selected user id is invalid."
		}
	}
	if in.Name != nil && len(*in.Name) > 255 {
		errs["name"] = "The name must not be greater than 255 characters."
	}
	// withValidator after(): a registered member or a name is required.
	if (in.UserID == nil || *in.UserID == 0) && (in.Name == nil || *in.Name == "") {
		if _, taken := errs["name"]; !taken {
			errs["name"] = "Either a registered member or a name must be provided."
		}
	}
	return errs
}

// validatePosition enforces the Store/UpdatePositionRequest rules.
func (s *Service) validatePosition(ctx context.Context, name string, id int64) FieldErrors {
	errs := FieldErrors{}
	if name == "" {
		errs["name"] = "The name field is required."
		return errs
	}
	if len(name) > 255 {
		errs["name"] = "The name must not be greater than 255 characters."
		return errs
	}
	taken, err := s.store.PositionNameExists(ctx, name, id)
	if err == nil && taken {
		errs["name"] = "The name has already been taken."
	}
	return errs
}
