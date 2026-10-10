// Package activitylog is a thin wrapper over the sqlc-generated activity_log
// queries (spatie/laravel-activitylog's table). Morph columns store the
// spatie class names ('App\Models\User', 'App\Models\Role').
package activitylog

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shuvo-paul/baaku/internal/database/queries/generated"
)

// Row is one activity entry resolved for display: causer and subject names
// are joined in, like the reference's ->with(['causer','subject']).
type Row struct {
	ID          int64
	LogName     string
	Description string
	Event       string
	Properties  []byte
	CreatedAt   time.Time
	// Causer user fields; empty when the row has no causer (renders as System).
	CauserName  string
	CauserEmail string
	// Subject* describe the affected model: name/email for user subjects,
	// role name for role subjects; all empty when there is no subject.
	SubjectID        int64
	SubjectUserName  string
	SubjectUserEmail string
	SubjectRoleName  string
}

// Repo is a thin wrapper over the sqlc-generated activity queries.
type Repo struct {
	q *generated.Queries
}

func NewRepo(q *generated.Queries) *Repo { return &Repo{q: q} }

// Insert writes one activity row (spatie activity()->log()).
func (r *Repo) Insert(ctx context.Context, logName, description, subjectType string, subjectID *int64, causerID *int64, event string, properties []byte) error {
	var causerType, morphType *string
	if causerID != nil {
		t := userType
		causerType = &t
	}
	if subjectID != nil {
		morphType = &subjectType
	}
	if properties == nil {
		properties = []byte("[]")
	}
	return r.q.InsertActivity(ctx, generated.InsertActivityParams{
		LogName:     &logName,
		Description: description,
		SubjectType: morphType,
		SubjectID:   subjectID,
		CauserType:  causerType,
		CauserID:    causerID,
		Event:       &event,
		Properties:  properties,
	})
}

// userType is the spatie morph class stored in causer_type/subject_type.
const userType = "App\\Models\\User"

// ListPage returns one page of the admin activity feed plus whether a newer
// page exists. Page is 1-based; simplePaginate semantics (reference
// ActivityLogController@index) — fetch pageSize+1 rows to detect the extra.
func (r *Repo) ListPage(ctx context.Context, page, pageSize int) ([]Row, bool, error) {
	if page < 1 {
		page = 1
	}
	rows, err := r.q.ListDashboardActivity(ctx, generated.ListDashboardActivityParams{
		PageSize:   int32(pageSize + 1),
		PageOffset: int32((page - 1) * pageSize),
	})
	if err != nil {
		return nil, false, err
	}
	hasMore := len(rows) > pageSize
	if hasMore {
		rows = rows[:pageSize]
	}
	out := make([]Row, 0, len(rows))
	for _, a := range rows {
		out = append(out, Row{
			ID:               a.ID,
			LogName:          derefStr(a.LogName),
			Description:      a.Description,
			Event:            derefStr(a.Event),
			Properties:       a.Properties,
			CreatedAt:        timestampTime(a.CreatedAt),
			CauserName:       derefStr(a.CauserName),
			CauserEmail:      derefStr(a.CauserEmail),
			SubjectID:        derefInt64(a.SubjectID),
			SubjectUserName:  derefStr(a.SubjectUserName),
			SubjectUserEmail: derefStr(a.SubjectUserEmail),
			SubjectRoleName:  derefStr(a.SubjectRoleName),
		})
	}
	return out, hasMore, nil
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func timestampTime(ts pgtype.Timestamp) time.Time {
	if ts.Valid {
		return ts.Time
	}
	return time.Time{}
}
