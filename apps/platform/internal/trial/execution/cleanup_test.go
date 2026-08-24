package run

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

// unreachablePool is a pool that will never connect, and needs no database to
// build one: pgxpool.New does not dial. The absence of a database is the
// condition under test, so a test that required one could not express it.
func unreachablePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody@127.0.0.1:1/nothing?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// RUN-007: a sweep that cannot reach the database has established nothing, and
// what it does next is irreversible.
//
// isOrphan used to fold every error from GetRunAttemptForReconcile into "no
// such attempt" and hand it to orphanByAge, whose only remaining guard is the
// five-minute grace window. So a pool timeout during a busy period destroyed
// every live sandbox older than five minutes — precisely the long Runs the
// grace window was written to protect (M2 audit, 2026-08-24). No existing test
// caught it because no existing test ever made the database fail.
func TestOrphanJudgementIsWithheldWhenTheDatabaseCannotAnswer(t *testing.T) {
	s := &Service{Pool: unreachablePool(t)}

	created := time.Now().Add(-time.Hour) // far past orphanGrace
	entry := ProviderRun{
		RunID:         "9d3a4d0e-3f8c-4a1e-9c2b-1f0a7c5b2e11",
		RunAttemptID:  "0f2c7b91-5a6d-4e3f-8b0c-2d9e6a4f1c30",
		Provider:      "test",
		ProviderRunID: "sbx-live",
		CreatedAt:     &created,
	}

	orphan, why := s.isOrphan(context.Background(), entry, time.Now())
	if orphan {
		t.Fatalf("an unanswerable lookup was read as a verdict to destroy: %q", why)
	}
}

// The same sandbox, once the platform can actually say it has no such attempt,
// is still an orphan. Without this the fix above could be "return false,
// always" and the test above would not notice.
func TestAnUnrecognisedAndOldSandboxIsStillAnOrphan(t *testing.T) {
	s := &Service{Pool: unreachablePool(t)}

	created := time.Now().Add(-time.Hour)
	entry := ProviderRun{
		RunID:         "9d3a4d0e-3f8c-4a1e-9c2b-1f0a7c5b2e11",
		RunAttemptID:  "", // no attempt id at all: never reaches the database
		Provider:      "test",
		ProviderRunID: "sbx-leaked",
		CreatedAt:     &created,
	}

	orphan, why := s.isOrphan(context.Background(), entry, time.Now())
	if !orphan {
		t.Fatal("a sandbox with no platform attempt id, older than the grace window, was spared")
	}
	if why == "" {
		t.Fatal("destroying a sandbox without recording why")
	}
}

// The status reason reaches runs.status_reason and run_attempts.error_message,
// both text columns in a UTF8 database. Postgres refuses an invalid byte
// sequence outright, and the write that carries the reason is the one carrying
// the terminal state — so a reason cut mid-rune does not produce a mangled
// message, it produces a Run that never finishes.
func TestTruncatedReasonsStayValidUTF8(t *testing.T) {
	// Traditional Chinese is three bytes per rune, and reasonLimit is not a
	// multiple of three for any interesting value, so the cut lands mid-rune.
	long := strings.Repeat("逾時", reasonLimit)

	got := truncate(long)
	if utf8.ValidString(got) != true {
		t.Fatalf("truncate produced invalid UTF-8: %q", got)
	}
	if len(got) > reasonLimit+len("...") {
		t.Fatalf("truncate returned %d bytes, over the %d limit", len(got), reasonLimit)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("a truncated reason must say it was truncated: %q", got)
	}

	// Short reasons are returned whole, ellipsis and all-ASCII cases included.
	if got := truncate("短"); got != "短" {
		t.Fatalf("a reason inside the limit was altered: %q", got)
	}
}
