// The gaps 稽核 01 found in the four skill contexts, each pinned through the real
// route table: a licensing hold a fork could shake off, two endpoints that kept
// answering about a deleted skill, an operator action that would have
// republished a generated skill, a second fork that was a 409, a workspace
// search with no input rules, and a manifest field whose value the contract did
// not allow.
package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// 02:SEC-011's added section says the held state travels with a Fork, in as many
// words, and names what it costs if it does not: 「Fork 一次就解除」. The propagation
// was written; nothing held it down. Deleting `AccessRestriction: src.AccessRestriction`
// from registry.Fork left the whole suite green, and the two places a hold is
// enforced — the files endpoint and the packaging gate — would both have opened
// for anybody who pressed Fork once.
func TestAForkCarriesTheLicensingHoldItWasForkedFrom(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "hold-fork-curator")
	markCatalog(t, pool, curator.workspaceID)
	held := importPackage(t, pool, a.packages, curator, "hold-travels-writer", true)
	restrict(t, pool, held)

	// A different user, so this is the real path: catalogue in, personal
	// workspace out. A fork inside the curator's own workspace would prove less.
	forker := a.login(t, "hold-fork-forker")
	code, body := forker.doJSON(t, http.MethodPost, "/skills/"+held+"/fork", "{}")
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, body)
	}
	forkID, _ := body["skill_id"].(string)
	if forkID == "" {
		t.Fatalf("fork response carried no skill_id: %v", body)
	}

	// The response says so, which is the half a user can see.
	if body["access_restriction"] == nil {
		t.Errorf("the fork reports no hold: %v", body["access_restriction"])
	}
	// And the column says so, which is the half every enforcement point reads.
	var restriction *string
	if err := pool.QueryRow(context.Background(),
		"SELECT access_restriction FROM skills WHERE id = $1", mustUUID(t, forkID),
	).Scan(&restriction); err != nil {
		t.Fatal(err)
	}
	if restriction == nil || *restriction == "" {
		t.Fatal("the fork's access_restriction is NULL: forking is now the way around a licensing hold")
	}

	// The consequence, stated the way the specification states it: the endpoint
	// that reproduces the package's own bytes is closed on the copy too.
	code, files := forker.doJSON(t, http.MethodGet, "/api/skills/"+forkID+"/files", "")
	if code != http.StatusForbidden {
		t.Fatalf("GET /files on the fork of a held skill answered %d, want 403", code)
	}
	if msg, _ := files["error"].(string); msg == "" {
		t.Error("the refusal carried no reason")
	}
}

// WS-005 and SEC-006: a deleted skill stops appearing in the ordinary access
// surfaces. The delete confirmation the server writes says the skill leaves the
// reader's 「lists」 and search — and a version history is a list, while the diff
// is the other reader of the same query. Both kept answering, because
// ListSkillVersions and GetSkillVersion filtered on workspace and skill alone
// and neither handler read the skills row first.
func TestADeletedSkillStopsAnsweringAboutItsVersions(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "deleted-versions-owner")

	skillID := seedSkill(t, pool, owner.workspaceID, "deleted-versions-skill")
	v1 := seedVersion(t, pool, owner.workspaceID, skillID, "hash-del-1")
	v2 := seedVersion(t, pool, owner.workspaceID, skillID, "hash-del-2")

	if got := owner.listVersions(t, skillID); len(got) != 2 {
		t.Fatalf("before the delete the history has %d rows, want 2", len(got))
	}
	diffPath := "/skills/" + skillID + "/diff?from=" + uuidText(v1.ID) + "&to=" + uuidText(v2.ID)

	code := owner.status(t, http.MethodDelete, "/skills/"+skillID)
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("DELETE /skills/%s: got %d", skillID, code)
	}

	// Empty rather than 404, which is this route's documented answer for a skill
	// it cannot see: the scope comes from the session, so a caller cannot tell
	// 「not yours」 from 「not there」 anyway, and the two screens that read this
	// treat both as 「nothing to pick」.
	if got := owner.listVersions(t, skillID); len(got) != 0 {
		t.Errorf("a deleted skill still lists %d versions: %+v", len(got), got)
	}
	// The diff names two versions, so it has something to be not-found about.
	if code := owner.status(t, http.MethodGet, diffPath); code != http.StatusNotFound {
		t.Errorf("GET diff on a deleted skill answered %d, want 404", code)
	}
}

// GEN-007's search exclusion has no key of its own: it reads
// `skills.redistribution <> 'generated'`, on the search query and on the
// enrichment worklist alike. So an operator pressing 「不可再散布」 on a generated
// skill would not have tightened anything — it would have erased the only record
// that the skill is generated, put it back into its workspace's search results,
// and put it back into a queue that spends money on it.
func TestAnOperatorCannotUnhideAGeneratedSkillBySettingItBlocked(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, "generated-unhide-operator")
	a.auth.Operators = map[string]bool{operator.userID: true}

	skillID := seedSkill(t, pool, operator.workspaceID, "generated-unhide-skill")
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET redistribution = 'generated' WHERE id = $1", mustUUID(t, skillID),
	); err != nil {
		t.Fatal(err)
	}

	code, body := operatorCall(t, operator, http.MethodPut,
		"/admin/skills/"+skillID+"/redistribution",
		`{"value":"blocked","note":"a reviewer decided this must stop being handed out"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("PUT redistribution=blocked on a generated skill: got %d, body %v", code, body)
	}
	// A refusal that does not name the mechanism which does work is a dead end.
	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "access_restriction") {
		t.Errorf("the refusal must point at the hold that does work, got %q", msg)
	}

	// Nothing landed: not the column, and not an audit event about a change that
	// did not happen.
	var value string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id = $1", mustUUID(t, skillID)).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "generated" {
		t.Errorf("redistribution is now %q; the generated record was overwritten", value)
	}
	var events int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_events WHERE action = 'skill.redistribution' AND resource_id = $1",
		mustUUID(t, skillID)).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Errorf("%d audit events for a refused change; the transaction did not unwind", events)
	}
}

// WS-001 lets a signed-in user fork a skill into their workspace. It does not
// say once, and the fixed `-fork` suffix meant the second one hit the unique
// index and came back a 409 with no suggested next step — for what is the
// ordinary shape of the work: 試跑、改、再試一份.
func TestASecondForkGetsItsOwnNameInsteadOfAConflict(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "fork-naming-curator")
	markCatalog(t, pool, curator.workspaceID)
	source := importPackage(t, pool, a.packages, curator, "twice-forked", true)

	forker := a.login(t, "fork-naming-forker")
	names := make([]string, 0, 3)
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		code, body := forker.doJSON(t, http.MethodPost, "/skills/"+source+"/fork", "{}")
		if code != http.StatusCreated {
			t.Fatalf("fork %d: got %d, body %v", i+1, code, body)
		}
		name, _ := body["name"].(string)
		id, _ := body["skill_id"].(string)
		names = append(names, name)
		ids = append(ids, id)
	}
	for i, want := range []string{"twice-forked-fork", "twice-forked-fork-2", "twice-forked-fork-3"} {
		if names[i] != want {
			t.Errorf("fork %d is named %q, want %q", i+1, names[i], want)
		}
	}

	// A fork of a fork keeps the series flat rather than growing a word per
	// generation: `x-fork-fork` is a name nobody chose and nobody can read.
	code, body := forker.doJSON(t, http.MethodPost, "/skills/"+ids[0]+"/fork", "{}")
	if code != http.StatusCreated {
		t.Fatalf("forking a fork: got %d, body %v", code, body)
	}
	if name, _ := body["name"].(string); name != "twice-forked-fork-4" {
		t.Errorf("a fork of a fork is named %q, want twice-forked-fork-4", name)
	}
}

// DISC-001's rule — 空白或無法理解的查詢不得建立搜尋 — is about searching, not about
// one route. The public endpoint had both checks and the workspace one had
// neither, so a 2 MB `q` went straight into websearch_to_tsquery.
func TestTheWorkspaceSearchAppliesTheSameQueryRulesAsThePublicOne(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	me := a.login(t, "workspace-search-rules")

	long := strings.Repeat("abcdefghij", 210) // 2100 runes
	for name, query := range map[string]string{
		"blank":                     "",
		"over the 2000-rune cap":    long,
		"nothing a search is about": "!!",
		"one rune":                  "a",
	} {
		path := "/skills/search?q=" + url.QueryEscape(query)
		if code := me.status(t, http.MethodGet, path); code != http.StatusBadRequest {
			t.Errorf("%s: GET /skills/search answered %d, want 400", name, code)
		}
	}
	// And a usable query is still answered, so the rules did not close the route.
	if code := me.status(t, http.MethodGet, "/skills/search?q=csv"); code != http.StatusOK {
		t.Errorf("a usable query answered %d, want 200", code)
	}
}

// A generated skill's manifest writes `source_type: "generated"` straight out of
// skill_sources, and until 2026-08-29 the packaging contract's two source_type
// enums listed only git and upload — so the one field a downloader reads to know
// where a package came from carried a value the schema refused.
//
// The permitted set is read from the shipped schema rather than restated here:
// an expectation written in this file drifts the same way the thing it checks
// drifts, and then agrees with it.
func TestAGeneratedSkillsManifestNamesASourceTypeTheContractAllows(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "generated-manifest-owner")
	skillID, versionID := packagedSkill(t, a, pool, c, "generated-manifest-skill")

	// What the generation path writes. 0037's one-way CHECK requires all three
	// generator columns whenever source_type is 'generated', so this is the whole
	// row shape and not a partial one.
	if _, err := pool.Exec(context.Background(), `
		UPDATE skill_sources SET source_type = 'generated', source_url = NULL, source_ref = NULL,
			task_description = 'tidy a csv', generator_model = 'test-model',
			generator_prompt_version = 'generate-skill/v1'
		WHERE id = (SELECT source_id FROM skill_versions WHERE id = $1)`,
		mustUUID(t, versionID)); err != nil {
		t.Fatal(err)
	}

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	raw, ok := zipEntries(t, a, hash)["skillhub-manifest.json"]
	if !ok {
		t.Fatal("the package carries no manifest")
	}
	var manifest struct {
		SchemaVersion string `json:"schema_version"`
		Source        struct {
			Origin struct {
				Kind       string `json:"kind"`
				SourceType string `json:"source_type"`
			} `json:"origin"`
		} `json:"source"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Source.Origin.SourceType != "generated" {
		t.Fatalf("source.origin.source_type = %q, want generated: %s", manifest.Source.Origin.SourceType, raw)
	}
	allowed := manifestSourceTypeEnum(t)
	if !contains(allowed, manifest.Source.Origin.SourceType) {
		t.Errorf("the manifest writes source_type %q, which the contract's enum %v does not permit",
			manifest.Source.Origin.SourceType, allowed)
	}
	// A producer declares the contract version it writes to, and `generated` is a
	// 1.1 value. Declaring 1.0 while writing one would make the version string a
	// decoration rather than a statement.
	if manifest.SchemaVersion == "1.0" {
		t.Errorf("schema_version is %q while the manifest writes a value 1.0 does not define",
			manifest.SchemaVersion)
	}
}

// manifestSourceTypeEnum reads the shipped schema and returns the source_type
// values its import origin branch permits.
func manifestSourceTypeEnum(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "..",
		"contracts", "packaging", "download-manifest.schema.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Defs struct {
			Origin struct {
				OneOf []struct {
					Title      string `json:"title"`
					Properties struct {
						SourceType struct {
							Enum []string `json:"enum"`
						} `json:"source_type"`
					} `json:"properties"`
				} `json:"oneOf"`
			} `json:"origin"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	for _, branch := range schema.Defs.Origin.OneOf {
		if branch.Title == "import" {
			if len(branch.Properties.SourceType.Enum) == 0 {
				t.Fatal("the import origin branch declares no source_type enum")
			}
			return branch.Properties.SourceType.Enum
		}
	}
	t.Fatal("the manifest schema has no import origin branch")
	return nil
}

// GetVersionBySkillAndHash sat in a query file whose header says every read there
// is workspace scoped, without one. It was safe because of who called it, which
// is precisely what skills.sql warns about in as many words: an unscoped read in
// a query file is a cross-tenant read waiting for its second caller. Now the
// scope is in the query, and this is what it refuses.
func TestDuplicateDetectionCannotReachAnotherWorkspacesVersion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "dup-scope-owner")
	stranger := a.login(t, "dup-scope-stranger")

	skillID := seedSkill(t, pool, owner.workspaceID, "dup-scope-skill")
	seedVersion(t, pool, owner.workspaceID, skillID, "sha256:dup-scope")

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, found, err := registry.VersionByContent(context.Background(), tx,
		mustUUID(t, owner.workspaceID), mustUUID(t, skillID), "sha256:dup-scope"); err != nil || !found {
		t.Fatalf("the owning workspace must find its own version: found=%v err=%v", found, err)
	}
	// A different workspace holding the same skill id — which it should never
	// have, and that is the point: the read refuses on its own rather than on the
	// caller having been careful.
	if _, found, err := registry.VersionByContent(context.Background(), tx,
		mustUUID(t, stranger.workspaceID), mustUUID(t, skillID), "sha256:dup-scope"); err != nil || found {
		t.Fatalf("a foreign workspace read somebody else's version: found=%v err=%v", found, err)
	}
}
