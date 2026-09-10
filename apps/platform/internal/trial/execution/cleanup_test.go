package run

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
)

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

func TestOrphanJudgementIsWithheldWhenTheDatabaseCannotAnswer(t *testing.T) {
	s := &Service{Pool: unreachablePool(t)}

	created := time.Now().Add(-time.Hour)
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

func TestAnUnrecognisedAndOldSandboxIsStillAnOrphan(t *testing.T) {
	s := &Service{Pool: unreachablePool(t)}

	created := time.Now().Add(-time.Hour)
	entry := ProviderRun{
		RunID:         "9d3a4d0e-3f8c-4a1e-9c2b-1f0a7c5b2e11",
		RunAttemptID:  "",
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

func TestTruncatedReasonsStayValidUTF8(t *testing.T) {

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

	if got := truncate("短"); got != "短" {
		t.Fatalf("a reason inside the limit was altered: %q", got)
	}
}
