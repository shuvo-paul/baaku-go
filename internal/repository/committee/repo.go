// Package committee is the persistence adapter for the committee service
// (reference committee_members + positions tables and the UserSearch picker).
package committee

import (
	"context"

	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/committee"
)

// Repo is the committee store; *Repo satisfies committee.Store.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

func (r *Repo) ListMembers(ctx context.Context) ([]committee.Member, error) {
	rows, err := r.q.ListCommitteeMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]committee.Member, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromMemberRow(row))
	}
	return out, nil
}

func (r *Repo) GetMember(ctx context.Context, id int64) (committee.Member, error) {
	row, err := r.q.GetCommitteeMemberByID(ctx, id)
	if err != nil {
		return committee.Member{}, err
	}
	return fromMemberRow(generated.ListCommitteeMembersRow{
		ID:            row.ID,
		PositionID:    row.PositionID,
		UserID:        row.UserID,
		Name:          row.Name,
		PhotoPath:     row.PhotoPath,
		SortOrder:     row.SortOrder,
		PositionName:  row.PositionName,
		UserName:      row.UserName,
		UserEmail:     row.UserEmail,
		UserPhotoPath: row.UserPhotoPath,
	}), nil
}

func (r *Repo) MaxMemberSortOrder(ctx context.Context) (int32, error) {
	v, err := r.q.MaxCommitteeMemberSortOrder(ctx)
	if err != nil {
		return 0, err
	}
	return toInt32(v), nil
}

func (r *Repo) CreateMember(ctx context.Context, m committee.Member) (int64, error) {
	return r.q.CreateCommitteeMember(ctx, generated.CreateCommitteeMemberParams{
		PositionID: m.PositionID,
		UserID:     m.UserID,
		Name:       m.Name,
		PhotoPath:  m.PhotoPath,
		SortOrder:  m.SortOrder,
	})
}

func (r *Repo) UpdateMember(ctx context.Context, m committee.Member) error {
	return r.q.UpdateCommitteeMember(ctx, generated.UpdateCommitteeMemberParams{
		PositionID: m.PositionID,
		UserID:     m.UserID,
		Name:       m.Name,
		PhotoPath:  m.PhotoPath,
		ID:         m.ID,
	})
}

func (r *Repo) DeleteMember(ctx context.Context, id int64) error {
	return r.q.DeleteCommitteeMember(ctx, id)
}

func (r *Repo) ReorderMember(ctx context.Context, id int64, sort int32) error {
	return r.q.ReorderCommitteeMember(ctx, generated.ReorderCommitteeMemberParams{
		SortOrder: sort,
		ID:        id,
	})
}

func (r *Repo) ListPositions(ctx context.Context) ([]committee.Position, error) {
	rows, err := r.q.ListPositions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]committee.Position, 0, len(rows))
	for _, row := range rows {
		out = append(out, committee.Position{
			ID:          row.ID,
			Name:        row.Name,
			MemberCount: row.CommitteeMembersCount,
		})
	}
	return out, nil
}

func (r *Repo) GetPosition(ctx context.Context, id int64) (committee.Position, error) {
	row, err := r.q.GetPositionByID(ctx, id)
	if err != nil {
		return committee.Position{}, err
	}
	return committee.Position{ID: row.ID, Name: row.Name}, nil
}

func (r *Repo) CreatePosition(ctx context.Context, name string) (int64, error) {
	return r.q.CreatePosition(ctx, name)
}

func (r *Repo) UpdatePosition(ctx context.Context, id int64, name string) error {
	return r.q.UpdatePosition(ctx, generated.UpdatePositionParams{Name: name, ID: id})
}

func (r *Repo) DeletePosition(ctx context.Context, id int64) error {
	return r.q.DeletePosition(ctx, id)
}

func (r *Repo) PositionNameExists(ctx context.Context, name string, excludeID int64) (bool, error) {
	return r.q.PositionNameExists(ctx, generated.PositionNameExistsParams{Name: name, ID: excludeID})
}

func (r *Repo) PositionIDExists(ctx context.Context, id int64) (bool, error) {
	return r.q.PositionIDExists(ctx, id)
}

func (r *Repo) UserIDExists(ctx context.Context, id int64) (bool, error) {
	return r.q.UserIDExists(ctx, id)
}

func (r *Repo) SearchUsers(ctx context.Context, q string) ([]committee.SearchUser, error) {
	rows, err := r.q.SearchCommitteeUsers(ctx, &q)
	if err != nil {
		return nil, err
	}
	out := make([]committee.SearchUser, 0, len(rows))
	for _, row := range rows {
		out = append(out, committee.SearchUser{ID: row.ID, Name: row.Name, Email: row.Email})
	}
	return out, nil
}

// fromMemberRow maps the joined listing row to the domain member.
func fromMemberRow(row generated.ListCommitteeMembersRow) committee.Member {
	return committee.Member{
		ID:            row.ID,
		PositionID:    row.PositionID,
		PositionName:  row.PositionName,
		UserID:        row.UserID,
		UserName:      row.UserName,
		UserPhotoPath: row.UserPhotoPath,
		UserEmail:     row.UserEmail,
		Name:          row.Name,
		PhotoPath:     row.PhotoPath,
		SortOrder:     row.SortOrder,
	}
}

// toInt32 coerces the COALESCE(MAX(sort_order), -1) interface{} payload (pgx
// returns int64 for max) to int32.
func toInt32(v interface{}) int32 {
	switch t := v.(type) {
	case int64:
		return int32(t)
	case int32:
		return t
	case int:
		return int32(t)
	default:
		return 0
	}
}
