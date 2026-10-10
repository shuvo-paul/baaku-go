package membershiprepo

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// tsPtr converts a pgtype.Timestamp to a *time.Time (nil when invalid).
func tsPtr(t pgtype.Timestamp) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// ts converts a *time.Time to a pgtype.Timestamp (invalid when nil).
func ts(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

// date converts a *time.Time to a pgtype.Date (invalid when nil).
func date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// datePtr converts a pgtype.Date to a *time.Time (nil when invalid).
func datePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

// numString renders a pgtype.Numeric as a plain decimal string (the domain
// keeps money as the canonical "1500.00" text). Invalid → "0".
func numString(n pgtype.Numeric) string {
	if !n.Valid {
		return "0"
	}
	var v interface{}
	if err := n.Scan(&v); err != nil {
		return "0"
	}
	switch t := v.(type) {
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'f', 2, 64)
	default:
		return "0"
	}
}

// numValue parses a decimal string into a pgtype.Numeric (invalid when the
// string does not parse).
func numValue(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(strings.TrimSpace(s)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

// featuresMap decodes the plan's features JSON column ({"members":"1"}) into a
// map. Empty / non-object → nil.
func featuresMap(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}

// featuresJSON encodes a features map into the JSON column value (nil map →
// nil bytes, stored as SQL NULL via the query's COALESCE).
func featuresJSON(m map[string]string) []byte {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// int32Ptr maps the optional OFFSET to its generated *int32 form (nil when 0).
func int32Ptr(v int64) *int32 {
	if v == 0 {
		return nil
	}
	i := int32(v)
	return &i
}
