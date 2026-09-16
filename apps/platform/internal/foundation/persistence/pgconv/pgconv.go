package pgconv

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func UUIDString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}

func RFC3339(ts pgtype.Timestamptz) string {
	if !ts.Valid {
		return ""
	}
	return ts.Time.UTC().Format(time.RFC3339)
}

func Timestamptz(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func Interval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
