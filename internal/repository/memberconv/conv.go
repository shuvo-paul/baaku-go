// Package memberconv holds the pgx ↔ domain conversions shared by the
// membership repositories: numeric(10,2) money as a canonical string, the
// plan's features JSON, and nullable timestamps.
package memberconv

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// NumString renders a pgtype.Numeric as a plain decimal string (the domain
// keeps money as the canonical "1500.00" text). Invalid → "0".
func NumString(n pgtype.Numeric) string {
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

// NumValue parses a decimal string into a pgtype.Numeric (invalid when the
// string does not parse).
func NumValue(s string) pgtype.Numeric {
	var n pgtype.Numeric
	if err := n.Scan(strings.TrimSpace(s)); err != nil {
		return pgtype.Numeric{}
	}
	return n
}

// FeaturesMap decodes the plan's features JSON column ({"members":"1"}) into a
// map. Empty / non-object → nil.
func FeaturesMap(raw []byte) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil || len(m) == 0 {
		return nil
	}
	return m
}

// FeaturesJSON encodes a features map into the JSON column value (nil map →
// nil bytes, stored as SQL NULL).
func FeaturesJSON(m map[string]string) []byte {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

// TSPtr converts a pgtype.Timestamp to a *time.Time (nil when invalid).
func TSPtr(t pgtype.Timestamp) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}

// TS converts a *time.Time to a pgtype.Timestamp (invalid when nil).
func TS(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}

// Date converts a *time.Time to a pgtype.Date (invalid when nil).
func Date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// DatePtr converts a pgtype.Date to a *time.Time (nil when invalid).
func DatePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	return &d.Time
}

// DateValue converts a pgtype.Date to a time.Time (zero when invalid).
func DateValue(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return d.Time
}
