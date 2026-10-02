package pgconv

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const uniqueViolationCode = "23505"

func UniqueViolation(err error) (constraint string, violated bool) {
	pgErr, ok := errors.AsType[*pgconn.PgError](err)
	if !ok || pgErr.Code != uniqueViolationCode {
		return "", false
	}
	return pgErr.ConstraintName, true
}

func IsUniqueViolation(err error) bool {
	_, violated := UniqueViolation(err)
	return violated
}

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
