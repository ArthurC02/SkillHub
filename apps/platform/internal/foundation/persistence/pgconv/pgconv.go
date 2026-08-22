// Package pgconv converts between pgtype values and Go basic types.
// Generic: no domain rules live here. Callers decide what an absent value
// means; these helpers only agree on how it is spelled.
package pgconv

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// UUIDString renders a UUID in canonical 8-4-4-4-12 form, or "" when the
// value is NULL. pgtype.UUID.Value never returns an error, so an invalid
// value is indistinguishable from the empty string here by design.
func UUIDString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}

// RFC3339 renders a timestamp as UTC RFC 3339, or "" when the value is NULL.
// Callers that must always emit a timestamp (a manifest field with no null
// spelling) handle the zero case themselves rather than sharing one here.
func RFC3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

// Timestamptz wraps a Go time as a non-NULL pgtype value.
func Timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
