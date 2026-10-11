package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

var backlogEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

func backlogDay(n int) time.Time { return backlogEpoch.AddDate(0, 0, n) }

func assertOldest(t *testing.T, what string, got pgtype.Timestamptz, err error, want time.Time) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	if !got.Valid || !got.Time.Equal(want) {
		t.Fatalf("%s: oldest = %+v, want %s", what, got, want)
	}
}

func TestTheOrphanObjectBacklogIsAsOldAsItsOldestQueuedKey(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	keys := []string{"packages/backlog-oldest.zip", "packages/backlog-newer.zip"}
	for i, key := range keys {
		if _, err := pool.Exec(ctx, `INSERT INTO object_collection_queue (object_key, enqueued_at) VALUES ($1, $2)`,
			key, backlogDay(i+1)); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM object_collection_queue WHERE object_key = ANY($1)`, keys)
	})

	got, err := (&registry.Service{Pool: pool}).OldestCollectableObject(ctx)
	assertOldest(t, "orphan objects", got, err, backlogDay(1))
}

func TestTheSourceCheckBacklogCountsOnlyUrlSourcesFromTheirLastCheckOrCreation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ctx := context.Background()
	ws := mustUUID(t, a.login(t, uniqueWorklistLabel("backlog-sources")).workspaceID)

	var ids []pgtype.UUID
	insert := func(sourceType string, url *string, created time.Time, checked *time.Time) {
		var id pgtype.UUID
		if err := pool.QueryRow(ctx, `INSERT INTO skill_sources
			(workspace_id, source_type, source_url, content_hash, fetched_at, created_at, last_checked_at, counts_toward_generate_quota)
			VALUES ($1, $2, $3, 'sha256:backlog', $4, $4, $5, false) RETURNING id`,
			ws, sourceType, url, created, checked).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	url := "https://github.com/example/backlog"
	checkedLater := backlogDay(4)
	insert(string(ingest.SourceUpload), nil, backlogDay(1), nil)
	insert(string(ingest.SourceGit), &url, backlogDay(0), &checkedLater)
	insert(string(ingest.SourceGit), &url, backlogDay(3), nil)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM skill_sources WHERE id = ANY($1)`, ids)
	})

	got, err := (&ingest.Service{Pool: pool}).OldestSourceCheck(ctx)
	assertOldest(t, "source checks", got, err, backlogDay(3))
}

func setAsidePendingEnrichments(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, `UPDATE search_documents SET enrichment_status = $2
		WHERE enrichment_status = $1 RETURNING skill_id`,
		string(catalog.EnrichmentPending), string(catalog.EnrichmentEnriched))
	if err != nil {
		t.Fatal(err)
	}
	var setAside []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		setAside = append(setAside, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `UPDATE search_documents SET enrichment_status = $2
			WHERE skill_id = ANY($1)`, setAside, string(catalog.EnrichmentPending)); err != nil {
			t.Errorf("restore pending enrichments: %v", err)
		}
	})
}

func TestTheEnrichmentBacklogCountsOnlyPendingDocumentsThatHaveAPackage(t *testing.T) {
	pool := requireDB(t)
	setAsidePendingEnrichments(t, pool)
	a := newAPI(t, pool)
	ctx := context.Background()
	c := a.login(t, uniqueWorklistLabel("backlog-enrichment"))

	var skills []pgtype.UUID
	seed := func(name, status string, packaged bool, updated time.Time) {
		skillID := seedSkill(t, pool, c.workspaceID, uniqueWorklistLabel(name))
		skill := mustUUID(t, skillID)
		if _, err := pool.Exec(ctx, `UPDATE search_documents SET enrichment_status = $2, updated_at = $3 WHERE skill_id = $1`,
			skill, status, updated); err != nil {
			t.Fatal(err)
		}
		if packaged {
			version := mustUUID(t, seedSkillVersion(t, pool, c.workspaceID, skillID))
			if _, err := pool.Exec(ctx, `UPDATE search_documents sd
				SET latest_version_id = v.id, latest_package_object_key = v.package_object_key, latest_source_path = v.source_path
				FROM skill_versions v WHERE v.id = $2 AND sd.skill_id = $1`, skill, version); err != nil {
				t.Fatal(err)
			}
		}
		skills = append(skills, skill)
	}
	seed("backlog-no-package", string(catalog.EnrichmentPending), false, backlogDay(1))
	seed("backlog-enriched", string(catalog.EnrichmentEnriched), true, backlogDay(2))
	seed("backlog-waiting", string(catalog.EnrichmentPending), true, backlogDay(5))
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `UPDATE search_documents SET enrichment_status = $2, updated_at = now()
			WHERE skill_id = ANY($1)`, skills, string(catalog.EnrichmentEnriched))
	})

	got, err := (&catalog.Service{Pool: pool}).OldestPendingEnrichment(ctx)
	assertOldest(t, "enrichment", got, err, backlogDay(5))
}
