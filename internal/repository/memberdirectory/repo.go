// Package memberdirectory — repository for the dashboard member directory
// (reference UserRoleController): paginated user cards, per-state counts and
// the show-page fetch. Thin wrappers over the sqlc-generated queries.
package memberdirectory

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
	"github.com/shuvo-paul/baaku/internal/service/memberdirectory"
	"github.com/shuvo-paul/baaku/internal/service/user"
)

// Repo is the member-directory store; *Repo satisfies memberdirectory.Store.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// List returns one page of member cards. % and _ are escaped the way the
// reference does it (str_replace on the LIKE pattern).
func (r *Repo) List(ctx context.Context, opts memberdirectory.ListOptions) (memberdirectory.Page, error) {
	var state *string
	if opts.State != "" {
		state = &opts.State
	}
	var search *string
	if s := escapeLike(opts.Search); s != "" {
		search = &s
	}
	rows, err := r.q.ListDirectoryUsers(ctx, generated.ListDirectoryUsersParams{
		ActiveOnly: opts.ActiveOnly,
		State:      state,
		Search:     search,
		PageLimit:  int32(opts.PerPage),
		PageOffset: int32(opts.Offset),
	})
	if err != nil {
		return memberdirectory.Page{}, err
	}
	members := make([]memberdirectory.MemberCard, 0, len(rows))
	for _, row := range rows {
		members = append(members, cardFromRow(row.ID, row.Name, row.Email, row.State, row.CreatedAt, row.EmailVerifiedAt, row.PhotoPath, row.RoleNames, row.EducationLevel, row.EducationInstitution, row.CareerJobTitle, row.CareerCompany))
	}
	// The reference paginator total is the sum across grouped states.
	var total int64
	counts, err := r.q.CountDirectoryUsers(ctx, generated.CountDirectoryUsersParams{
		ActiveOnly: opts.ActiveOnly,
		State:      state,
		Search:     search,
	})
	if err != nil {
		return memberdirectory.Page{}, err
	}
	for _, c := range counts {
		total += c.Total
	}
	return memberdirectory.Page{Members: members, Total: total}, nil
}

// CountsByState returns the state → count map the admin header renders.
func (r *Repo) CountsByState(ctx context.Context) (map[string]int64, error) {
	rows, err := r.q.CountUsersByState(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.State] = row.Total
	}
	return out, nil
}

// Get returns one member for the show page. activeOnly mirrors the non-admin
// branch: non-active users surface as pgx.ErrNoRows → 404.
func (r *Repo) Get(ctx context.Context, id int64, activeOnly bool) (memberdirectory.Member, error) {
	row, err := r.q.GetDirectoryUserByID(ctx, generated.GetDirectoryUserByIDParams{ID: id, ActiveOnly: activeOnly})
	if err != nil {
		return memberdirectory.Member{}, err
	}
	return memberdirectory.Member{
		MemberCard:       cardFromRow(row.ID, row.Name, row.Email, row.State, row.CreatedAt, row.EmailVerifiedAt, row.PhotoPath, row.RoleNames, "", "", "", ""),
		Phone:            row.Phone,
		DateOfBirth:      formatDate(row.DateOfBirth),
		Gender:           row.Gender,
		BloodGroup:       row.BloodGroup,
		PresentAddress:   row.PresentAddress,
		PermanentAddress: row.PermanentAddress,
		SocialLinks:      unmarshalMap(row.SocialLinks),
		Website:          row.Website,
		EmergencyContact: unmarshalMap(row.EmergencyContact),
	}, nil
}

// SetState persists the membership state (reference UserStateController
// @update's $user->update(['state' => ...])).
func (r *Repo) SetState(ctx context.Context, id int64, state user.UserState) error {
	return r.q.UpdateUserState(ctx, generated.UpdateUserStateParams{ID: id, State: string(state)})
}

func cardFromRow(id int64, name, email, state string, createdAt, verified pgtype.Timestamp, photoPath *string, roleNames interface{}, eduLevel, eduInst, jobTitle, company string) memberdirectory.MemberCard {
	return memberdirectory.MemberCard{
		ID:                   id,
		Name:                 name,
		Email:                email,
		State:                user.UserState(state),
		CreatedAt:            createdAt.Time,
		EmailVerified:        verified.Valid,
		PhotoPath:            photoPath,
		Roles:                textSlice(roleNames),
		EducationLevel:       eduLevel,
		EducationInstitution: eduInst,
		CareerJobTitle:       jobTitle,
		CareerCompany:        company,
	}
}

// escapeLike escapes the LIKE metacharacters (reference: str_replace(['%',
// '_'], ['\\%', '\\_'], $search)); the query adds the ESCAPE clause.
func escapeLike(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	return strings.ReplaceAll(s, `_`, `\_`)
}

// formatDate renders the pgtype.Date the way the reference show page does
// (format('Y-m-d')); invalid dates render empty.
func formatDate(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// textSlice coerces the sqlc array_agg payload (pgx returns interface{} for
// the text[] column) into []string.
func textSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// unmarshalMap decodes the JSON object columns (social_links,
// emergency_contact); nil/invalid JSON is an empty map.
func unmarshalMap(b []byte) map[string]string {
	out := map[string]string{}
	if len(b) == 0 {
		return out
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]string{}
	}
	return out
}
