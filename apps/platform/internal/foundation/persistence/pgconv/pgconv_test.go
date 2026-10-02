package pgconv

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUUIDString(t *testing.T) {
	valid := pgtype.UUID{Bytes: [16]byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}, Valid: true}
	if got, want := UUIDString(valid), "01234567-89ab-cdef-0123-456789abcdef"; got != want {
		t.Errorf("valid UUID = %q, want %q", got, want)
	}
	if got := UUIDString(pgtype.UUID{}); got != "" {
		t.Errorf("NULL UUID = %q, want empty", got)
	}
}

func TestRFC3339(t *testing.T) {
	ts := pgtype.Timestamptz{Time: time.Date(2026, 8, 19, 12, 30, 0, 0, time.FixedZone("x", 3600)), Valid: true}
	if got, want := RFC3339(ts), "2026-08-19T11:30:00Z"; got != want {
		t.Errorf("RFC3339 = %q, want %q (UTC normalised)", got, want)
	}
	if got := RFC3339(pgtype.Timestamptz{}); got != "" {
		t.Errorf("NULL timestamp = %q, want empty", got)
	}
}

func TestTimestamptz(t *testing.T) {
	now := time.Now()
	got := Timestamptz(now)
	if !got.Valid || !got.Time.Equal(now) {
		t.Errorf("Timestamptz(%v) = %+v, want valid and equal", now, got)
	}
}

func TestOnlyAUniqueViolationIsReportedWithItsConstraint(t *testing.T) {
	for _, c := range []struct {
		name           string
		err            error
		wantConstraint string
		wantViolated   bool
	}{
		{"a unique violation", fmt.Errorf("insert: %w", &pgconn.PgError{Code: "23505", ConstraintName: "skills_name_key"}), "skills_name_key", true},
		{"another constraint", &pgconn.PgError{Code: "23503", ConstraintName: "skills_workspace_fk"}, "", false},
		{"not a database error", errors.New("pool exhausted"), "", false},
		{"no error", nil, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			constraint, violated := UniqueViolation(c.err)
			if constraint != c.wantConstraint || violated != c.wantViolated || IsUniqueViolation(c.err) != c.wantViolated {
				t.Errorf("got (%q, %v), want (%q, %v)", constraint, violated, c.wantConstraint, c.wantViolated)
			}
		})
	}
}
