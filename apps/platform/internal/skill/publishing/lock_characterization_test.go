package publishing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var publishingPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintln(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset; this run would have skipped every database test and still reported success")
			os.Exit(1)
		}
		os.Exit(m.Run())
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	schemaLock, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := schemaLock.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	publishingPool = pool
	code := m.Run()
	_, _ = schemaLock.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
	schemaLock.Release()
	pool.Close()
	os.Exit(code)
}

func rolledBackPublishingTx(t *testing.T) pgx.Tx {
	t.Helper()
	if publishingPool == nil {
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the publishing lock database test")
	}
	ctx := context.Background()
	tx, err := publishingPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

func seedPublishingWorkspace(t *testing.T, tx pgx.Tx, name string) identity.Workspace {
	t.Helper()
	var ws identity.Workspace
	if err := tx.QueryRow(context.Background(),
		`WITH u AS (INSERT INTO users (email, display_name) VALUES ($1, $1) RETURNING id)
		 INSERT INTO workspaces (owner_user_id, name) SELECT id, $2 FROM u RETURNING id, owner_user_id`,
		name+"@example.test", name).Scan(&ws.ID, &ws.OwnerUserID); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestReleasingRequiresTheWorkspaceToHaveRegisteredAPublisher(t *testing.T) {
	ctx := context.Background()
	t.Run("no publisher", func(t *testing.T) {
		tx := rolledBackPublishingTx(t)
		ws := seedPublishingWorkspace(t, tx, "lock-no-publisher")
		if err := lockPublisher(ctx, gen.New(tx), ws); !errors.Is(err, ErrNoPublisher) {
			t.Fatalf("err = %v, want ErrNoPublisher", err)
		}
	})
	t.Run("registered publisher", func(t *testing.T) {
		tx := rolledBackPublishingTx(t)
		ws := seedPublishingWorkspace(t, tx, "lock-publisher")
		if _, err := tx.Exec(ctx, `INSERT INTO publishers (workspace_id, name) VALUES ($1, 'lock-publisher-name')`, ws.ID); err != nil {
			t.Fatal(err)
		}
		if err := lockPublisher(ctx, gen.New(tx), ws); err != nil {
			t.Fatalf("err = %v, want the publisher locked", err)
		}
	})
}

func TestEveryUnavailablePublicationReadsAsNoLongerOffered(t *testing.T) {
	for _, availability := range []Availability{
		AvailabilityWithdrawn, AvailabilityTakenDown, AvailabilityHeld, AvailabilityNotRedistributed,
	} {
		view := (&Handler{}).publicView(PublicPublication{Availability: availability})
		if view.Availability.Label != "已不提供" || view.Acquisition.Available {
			t.Errorf("%s: label %q, acquirable %v; want 已不提供 and nothing to acquire",
				availability, view.Availability.Label, view.Acquisition.Available)
		}
	}
}

func TestAnExposureReviewNeedsAPublicationWithARelease(t *testing.T) {
	ctx := context.Background()
	t.Run("no publication at that address", func(t *testing.T) {
		tx := rolledBackPublishingTx(t)
		id, _, err := lockCurrentExposure(ctx, gen.New(tx), "nobody-here", "nothing")
		if !errors.Is(err, ErrNotFound) || id.Valid {
			t.Fatalf("lockCurrentExposure = %v, %v; want ErrNotFound", id, err)
		}
	})
	t.Run("publication that was never released", func(t *testing.T) {
		tx := rolledBackPublishingTx(t)
		ws := seedPublishingWorkspace(t, tx, "lock-unreleased")
		var skillID pgtype.UUID
		if err := tx.QueryRow(ctx,
			`WITH pb AS (INSERT INTO publishers (workspace_id, name) VALUES ($1, 'lock-unreleased-pub') RETURNING id),
			      sk AS (INSERT INTO skills (workspace_id, name) VALUES ($1, 'unreleased') RETURNING id)
			 INSERT INTO publications (publisher_id, name, skill_id, status)
			 SELECT pb.id, 'unreleased', sk.id, 'published' FROM pb, sk RETURNING skill_id`, ws.ID).Scan(&skillID); err != nil {
			t.Fatal(err)
		}
		id, _, err := lockCurrentExposure(ctx, gen.New(tx), "lock-unreleased-pub", "unreleased")
		if !errors.Is(err, ErrNotFound) || id.Valid {
			t.Fatalf("lockCurrentExposure = %v, %v; want ErrNotFound", id, err)
		}
	})
}
