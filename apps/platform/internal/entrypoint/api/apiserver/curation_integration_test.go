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

func TestAHybridSearchRowCarriesTheCuratedTier(t *testing.T) {
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, stubLLM(t, 23, "it fits"))
	curator := a.login(t, uniqueWorklistLabel("curator-hybrid-tier"))
	markCatalog(t, pool, curator.workspaceID)
	skillID := importPackage(t, pool, a.packages, curator, "vorpal-hybrid-tier", false)
	seedEmbedding(t, pool, skillID, 23)
	curate(t, pool, skillID, newestVersion(t, pool, skillID))

	anon := &client{Client: http.DefaultClient, base: a.URL}
	body := anon.search(t, "/api/skills/search?q=vorpal")
	if body.Degraded || len(body.Results) == 0 || body.Results[0].SkillID != skillID {
		t.Fatalf("hybrid search did not rank the curated skill first: degraded=%v %v", body.Degraded, body.ids())
	}
	if got := body.Results[0].Tier.Value; got != "curated" {
		t.Fatalf("hybrid row tier = %q, want curated", got)
	}
}

func TestAnOperatorCuratesTheNewestCatalogueVersionAndCanWithdrawIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-tier-operator")
	markCatalog(t, pool, curator.workspaceID)
	skillID := importPackage(t, pool, a.packages, curator, "slithy-tier-operator", false)
	member := a.login(t, "member-tier-operator")
	userSkill := importPackage(t, pool, a.packages, member, "slithy-tier-imported", false)
	operator := a.login(t, "platform-operator-tier")
	a.auth.Operators = map[string]bool{operator.userID: true}
	anon := &client{Client: http.DefaultClient, base: a.URL}

	stored := func(id string) (tier string, version *string) {
		t.Helper()
		if err := pool.QueryRow(context.Background(),
			"SELECT curation_tier, curated_version_id::text FROM skills WHERE id = $1", mustUUID(t, id),
		).Scan(&tier, &version); err != nil {
			t.Fatal(err)
		}
		return tier, version
	}
	shown := func() string {
		t.Helper()
		return anon.search(t, "/api/skills/search?q=slithy-tier-operator").Results[0].Tier.Value
	}
	put := func(id, body string) (int, map[string]any) {
		t.Helper()
		return operatorCall(t, operator, http.MethodPut, "/admin/skills/"+id+"/tier", body)
	}

	if code, body := put(userSkill, `{"value":"curated","note":"looks fine"}`); code != http.StatusBadRequest {
		t.Fatalf("curating a user's own import: got %d (%v), want 400", code, body)
	}
	if tier, _ := stored(userSkill); tier != "indexed" {
		t.Fatalf("a refused curation still wrote tier %q", tier)
	}
	if code, _ := put(skillID, `{"value":"curated","note":"  "}`); code != http.StatusBadRequest {
		t.Fatalf("curating without a note: got %d, want 400", code)
	}
	if code, _ := put("00000000-0000-0000-0000-000000000001", `{"value":"indexed","note":"n"}`); code != http.StatusNotFound {
		t.Fatalf("curating a missing skill: got %d, want 404", code)
	}

	reviewed := newestVersion(t, pool, skillID)
	code, body := put(skillID, `{"value":"curated","note":"passed the nine curation checks"}`)
	if code != http.StatusOK || body["tier"] != "curated" || body["curated_version_id"] != reviewed || body["previous_tier"] != "indexed" {
		t.Fatalf("operator curation: got %d %v, want 200 curated on %s", code, body, reviewed)
	}
	if tier, version := stored(skillID); tier != "curated" || version == nil || *version != reviewed {
		t.Fatalf("stored tier = %q/%v, want curated/%s", tier, version, reviewed)
	}
	if got := shown(); got != "curated" {
		t.Fatalf("catalogue shows tier %q right after curation, want curated", got)
	}
	before, after, note, count := auditNote(t, operator, "skill.curation_set", skillID)
	if count != 1 || deref(before) != "indexed" || deref(after) != "curated" || deref(note) != "passed the nine curation checks" {
		t.Fatalf("audit = %d events, %s -> %s (%q)", count, deref(before), deref(after), deref(note))
	}
	var events int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = 'skill.curation_set'",
		mustUUID(t, skillID)).Scan(&events); err != nil || events != 1 {
		t.Fatalf("curation events = %d (%v), want 1", events, err)
	}

	if code, body := put(skillID, `{"value":"indexed","note":"source rewritten in place"}`); code != http.StatusOK || body["curated_version_id"] != nil {
		t.Fatalf("withdrawal: got %d %v, want 200 with no reviewed version", code, body)
	}
	if tier, version := stored(skillID); tier != "indexed" || version != nil {
		t.Fatalf("stored tier after withdrawal = %q/%v, want indexed/null", tier, version)
	}
	if got := shown(); got != "indexed" {
		t.Fatalf("catalogue shows tier %q after withdrawal, want indexed", got)
	}
}
