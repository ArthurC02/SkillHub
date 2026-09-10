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

func TestAForkCarriesTheLicensingHoldItWasForkedFrom(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "hold-fork-curator")
	markCatalog(t, pool, curator.workspaceID)
	held := importPackage(t, pool, a.packages, curator, "hold-travels-writer", true)
	restrict(t, pool, held)

	forker := a.login(t, "hold-fork-forker")
	code, body := forker.doJSON(t, http.MethodPost, "/skills/"+held+"/fork", "{}")
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, body)
	}
	forkID, _ := body["skill_id"].(string)
	if forkID == "" {
		t.Fatalf("fork response carried no skill_id: %v", body)
	}

	if body["access_restriction"] == nil {
		t.Errorf("the fork reports no hold: %v", body["access_restriction"])
	}

	var restriction *string
	if err := pool.QueryRow(context.Background(),
		"SELECT access_restriction FROM skills WHERE id = $1", mustUUID(t, forkID),
	).Scan(&restriction); err != nil {
		t.Fatal(err)
	}
	if restriction == nil || *restriction == "" {
		t.Fatal("the fork's access_restriction is NULL: forking is now the way around a licensing hold")
	}

	code, files := forker.doJSON(t, http.MethodGet, "/api/skills/"+forkID+"/files", "")
	if code != http.StatusForbidden {
		t.Fatalf("GET /files on the fork of a held skill answered %d, want 403", code)
	}
	if msg, _ := files["error"].(string); msg == "" {
		t.Error("the refusal carried no reason")
	}
}

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

	if got := owner.listVersions(t, skillID); len(got) != 0 {
		t.Errorf("a deleted skill still lists %d versions: %+v", len(got), got)
	}

	if code := owner.status(t, http.MethodGet, diffPath); code != http.StatusNotFound {
		t.Errorf("GET diff on a deleted skill answered %d, want 404", code)
	}
}

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

	msg, _ := body["error"].(string)
	if !strings.Contains(msg, "access_restriction") {
		t.Errorf("the refusal must point at the hold that does work, got %q", msg)
	}

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

	code, body := forker.doJSON(t, http.MethodPost, "/skills/"+ids[0]+"/fork", "{}")
	if code != http.StatusCreated {
		t.Fatalf("forking a fork: got %d, body %v", code, body)
	}
	if name, _ := body["name"].(string); name != "twice-forked-fork-4" {
		t.Errorf("a fork of a fork is named %q, want twice-forked-fork-4", name)
	}
}

func TestTheWorkspaceSearchAppliesTheSameQueryRulesAsThePublicOne(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	me := a.login(t, "workspace-search-rules")

	long := strings.Repeat("abcdefghij", 210)
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

	if code := me.status(t, http.MethodGet, "/skills/search?q=csv"); code != http.StatusOK {
		t.Errorf("a usable query answered %d, want 200", code)
	}
}

func TestAGeneratedSkillsManifestNamesASourceTypeTheContractAllows(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "generated-manifest-owner")
	skillID, versionID := packagedSkill(t, a, pool, c, "generated-manifest-skill")

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

	if manifest.SchemaVersion == "1.0" {
		t.Errorf("schema_version is %q while the manifest writes a value 1.0 does not define",
			manifest.SchemaVersion)
	}
}

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

	if _, found, err := registry.VersionByContent(context.Background(), tx,
		mustUUID(t, stranger.workspaceID), mustUUID(t, skillID), "sha256:dup-scope"); err != nil || found {
		t.Fatalf("a foreign workspace read somebody else's version: found=%v err=%v", found, err)
	}
}
