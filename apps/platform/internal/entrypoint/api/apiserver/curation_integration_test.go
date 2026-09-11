package apiserver_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func curate(t *testing.T, pool *pgxpool.Pool, skillID, versionID string) {
	t.Helper()
	var sk, ver pgtype.UUID
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	if err := ver.Scan(versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET curation_tier = 'curated', curated_version_id = $2 WHERE id = $1",
		sk, ver,
	); err != nil {
		t.Fatal(err)
	}
	refreshListing(t, pool, skillID)
}

func newestVersion(t *testing.T, pool *pgxpool.Pool, skillID string) string {
	t.Helper()
	var sk pgtype.UUID
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(),
		"SELECT id FROM skill_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1", sk,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	v, _ := id.Value()
	s, _ := v.(string)
	return s
}

func TestACuratedSkillSaysSoUntilANewVersionArrives(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-tier")
	markCatalog(t, pool, curator.workspaceID)
	skillID := importPackage(t, pool, a.packages, curator, "tulgey-ledger-tidy", false)

	anon := &client{Client: http.DefaultClient, base: a.URL}

	if got := anon.search(t, "/api/skills/search?q=tulgey").Results[0].Tier.Value; got != "indexed" {
		t.Fatalf("an imported skill claimed tier %q before any review was recorded", got)
	}

	reviewed := newestVersion(t, pool, skillID)
	curate(t, pool, skillID, reviewed)

	row := anon.search(t, "/api/skills/search?q=tulgey").Results[0]
	if row.Tier.Value != "curated" || row.Tier.Label == "" {
		t.Fatalf("tier = %+v, want curated with its copy", row.Tier)
	}

	seedSkillVersion(t, pool, curator.workspaceID, skillID)
	if newestVersion(t, pool, skillID) == reviewed {
		t.Fatal("the second version did not become the newest one; the rest of this test proves nothing")
	}

	after := anon.search(t, "/api/skills/search?q=tulgey").Results[0]
	if after.Tier.Value != "indexed" {
		t.Fatalf("tier = %q after a new version; a review of bytes nobody is looking at is still being shown as 精選", after.Tier.Value)
	}
}

func TestTheTierFilterSeparatesTheReviewedFromTheRest(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-tier-filter")
	markCatalog(t, pool, curator.workspaceID)
	reviewedSkill := importPackage(t, pool, a.packages, curator, "tumtum-toves-reviewed", false)
	importPackage(t, pool, a.packages, curator, "tumtum-toves-plain", false)
	curate(t, pool, reviewedSkill, newestVersion(t, pool, reviewedSkill))

	anon := &client{Client: http.DefaultClient, base: a.URL}

	if got := anon.search(t, "/api/skills/search?q=tumtum").Results; len(got) != 2 {
		t.Fatalf("unfiltered search returned %d rows, want both", len(got))
	}

	only := anon.search(t, "/api/skills/search?q=tumtum&tier=curated").Results
	if len(only) != 1 || only[0].SkillID != reviewedSkill {
		t.Fatalf("tier=curated returned %d rows: %v", len(only), only)
	}
	rest := anon.search(t, "/api/skills/search?q=tumtum&tier=indexed").Results
	if len(rest) != 1 || rest[0].SkillID == reviewedSkill {
		t.Fatalf("tier=indexed returned %d rows including the reviewed one: %v", len(rest), rest)
	}

	resp, err := http.Get(a.URL + "/api/skills/search?q=tumtum&tier=external")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("tier=external got %d, want 400", resp.StatusCode)
	}
}
