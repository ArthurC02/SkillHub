package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func postSource(t *testing.T, c *client, data []byte) (int, map[string]any) {
	t.Helper()
	resp, err := c.Post(c.base+"/skills/import/upload", "application/zip", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out := map[string]any{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decoding the %d body: %v", resp.StatusCode, err)
	}
	return resp.StatusCode, out
}

func conformingPlugin(name string) string {
	return `{"$schema":"` + skillpkg.PluginSchemaID + `","name":"` + name +
		`","version":"1.4.0","repository":"https://example.invalid/` + name + `"}`
}

func skillNamed(name string) string {
	return "---\nname: " + name + "\ndescription: Turn one thing into another thing.\nlicense: MIT\n---\n# " +
		name + "\n\nDo the thing.\n"
}

func skillsOf(t *testing.T, body map[string]any) []map[string]any {
	t.Helper()
	raw, ok := body["skills"].([]any)
	if !ok {
		t.Fatalf("no skills array in the reply: %v", body)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, entry := range raw {
		one, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a skills entry is not an object: %v", entry)
		}
		out = append(out, one)
	}
	return out
}

func pathsOf(t *testing.T, body map[string]any, key string) []string {
	t.Helper()
	raw, ok := body[key].([]any)
	if !ok {
		t.Fatalf("no %s array in the reply: %v", key, body)
	}
	var out []string
	for _, entry := range raw {
		one, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("a %s entry is not an object: %v", key, entry)
		}
		out = append(out, fmt.Sprint(one["path"]))
	}
	return out
}

func TestImportingAPluginAnswersWithEverySkillItHeldAndOneStoredPackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-import-reports-each")

	code, body := postSource(t, owner, zipOf(t, map[string]string{
		"plugin.json":                conformingPlugin("desk-tools"),
		"mcp.json":                   `{"mcpServers":{"files":{"command":"node"}}}`,
		"com.example.client/hint.md": "# a client's own directory\n",
		"skills/tidy-notes/SKILL.md": skillNamed("tidy-notes"),
		"skills/split-csv/SKILL.md":  skillNamed("split-csv"),
	}))

	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %v", code, body)
	}
	if body["shape"] != "plugin" {
		t.Errorf("shape = %v, want plugin; a conforming root manifest named these skills", body["shape"])
	}
	plugin, ok := body["plugin"].(map[string]any)
	if !ok || plugin["name"] != "desk-tools" || plugin["version"] != "1.4.0" {
		t.Errorf("plugin = %v, want the manifest's own facts", body["plugin"])
	}

	skills := skillsOf(t, body)
	if len(skills) != 2 {
		t.Fatalf("the reply carries %d skills, want 2; a client reading only the first would drop half the plugin: %v",
			len(skills), body)
	}
	seenPath, seenHash, keys := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, one := range skills {
		path := fmt.Sprint(one["path"])
		if path != "skills/tidy-notes" && path != "skills/split-csv" {
			t.Errorf("path = %q, want the directory the skill sat in", path)
		}
		if seenPath[path] {
			t.Errorf("path %q appears twice", path)
		}
		seenPath[path] = true

		hash := fmt.Sprint(one["content_hash"])
		if hash == "" || seenHash[hash] {
			t.Errorf("%s has content hash %q; two skills of one plugin must not share one", path, hash)
		}
		seenHash[hash] = true

		versionID := fmt.Sprint(one["version_id"])
		if versionID == "" || fmt.Sprint(one["skill_id"]) == "" {
			t.Fatalf("%s came back without ids: %v", path, one)
		}
		var key string
		if err := pool.QueryRow(context.Background(),
			`SELECT package_object_key FROM skill_versions WHERE id = $1`, versionID).Scan(&key); err != nil {
			t.Fatalf("reading %s back: %v", path, err)
		}
		keys[key] = true
	}
	if len(keys) != 1 {
		t.Errorf("the plugin was stored as %d objects, want 1: %v", len(keys), keys)
	}

	excluded := pathsOf(t, body, "excluded_components")
	for _, want := range []string{"mcp.json", "com.example.client/"} {
		if !contains(excluded, want) {
			t.Errorf("excluded_components = %v, want it to disclose %q", excluded, want)
		}
	}
}

func TestASourceWhoseEverySkillFailedIsRefusedWithEachOneNamed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-import-all-bad")

	code, body := postSource(t, owner, zipOf(t, map[string]string{
		"plugin.json":         conformingPlugin("all-bad"),
		"skills/one/SKILL.md": "no frontmatter at all\n",
		"skills/two/SKILL.md": "also no frontmatter\n",
	}))

	if code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %v", code, body)
	}
	if skills := skillsOf(t, body); len(skills) != 0 {
		t.Errorf("skills = %v, want empty: the empty list is the refusal", skills)
	}
	refused := pathsOf(t, body, "refused")
	for _, want := range []string{"skills/one", "skills/two"} {
		if !contains(refused, want) {
			t.Errorf("refused = %v, want %q named; a plugin whose every skill failed must still say which", refused, want)
		}
	}
}

func TestOneUnusableSkillDoesNotRefuseThePluginsGoodOnes(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-import-partial")

	code, body := postSource(t, owner, zipOf(t, map[string]string{
		"plugin.json":            conformingPlugin("half-good"),
		"skills/works/SKILL.md":  skillNamed("works"),
		"skills/broken/SKILL.md": "no frontmatter at all\n",
	}))

	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; one broken skill must not refuse the others: %v", code, body)
	}
	if got := pathsOf(t, body, "skills"); len(got) != 1 || got[0] != "skills/works" {
		t.Errorf("skills = %v, want only skills/works", got)
	}
	if got := pathsOf(t, body, "refused"); len(got) != 1 || got[0] != "skills/broken" {
		t.Errorf("refused = %v, want only skills/broken", got)
	}
}

func TestASourceOverTheSkillCeilingIsRefusedAndSaysTheCeiling(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-import-ceiling")

	files := map[string]string{"plugin.json": conformingPlugin("too-many")}
	for i := 0; i <= ingest.MaxSkillsPerImport; i++ {
		name := fmt.Sprintf("skill-%03d", i)
		files["skills/"+name+"/SKILL.md"] = skillNamed(name)
	}

	code, body := postSource(t, owner, zipOf(t, files))
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body = %v", code, body)
	}
	if got := fmt.Sprint(body["error"]); !strings.Contains(got, fmt.Sprint(ingest.MaxSkillsPerImport)) {
		t.Errorf("the refusal does not say the ceiling %d: %q", ingest.MaxSkillsPerImport, got)
	}
}

func TestEachSkillOfAnImportedPluginCanBePackagedOnItsOwn(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-packager")

	res := importSource(t, a, pool, owner, map[string]string{
		"plugin.json":                        conformingPlugin("packagable"),
		"mcp.json":                           `{"mcpServers":{"files":{"command":"node"}}}`,
		"skills/first-skill/SKILL.md":        packagingSKILLMD("first-skill"),
		"skills/second-skill/SKILL.md":       packagingSKILLMD("second-skill"),
		"skills/first-skill/scripts/run.py":  "print('first')\n",
		"skills/second-skill/scripts/run.py": "print('second')\n",
	}, nil)
	if len(res.Imported) != 2 {
		t.Fatalf("the plugin fixture imported %d skills, want 2", len(res.Imported))
	}

	for _, imported := range res.Imported {
		skillID, versionID := uuidText(imported.Skill.ID), uuidText(imported.Version.ID)
		name := imported.Skill.Name
		t.Run(name, func(t *testing.T) {
			allowRedistribution(t, pool, skillID)
			code, body := postJSON(t, owner, packagingPath(skillID, versionID), `{"target":"standard"}`)
			if code != http.StatusCreated {
				t.Fatalf("POST packaging for %s: got %d, body %v", name, code, body)
			}
			hash, _ := body["content_hash"].(string)
			entries := zipEntries(t, a, hash)

			if _, ok := entries["SKILL.md"]; !ok {
				t.Errorf("the package for %s has no SKILL.md at its root; entries = %v", name, sortedKeys(entries))
			}
			for entry := range entries {
				if strings.HasPrefix(entry, "skills/") || entry == "plugin.json" || entry == "mcp.json" {
					t.Errorf("the package for %s carries %q — a download of one skill must not hand over the whole plugin", name, entry)
				}
			}
		})
	}
}

func sortedKeys(entries map[string][]byte) []string {
	out := make([]string, 0, len(entries))
	for name := range entries {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func TestEverySkillOfAPluginKeepsThePluginsOwnFacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "plugin-facts")

	res := importSource(t, a, pool, owner, map[string]string{
		"plugin.json":         conformingPlugin("desk-tools"),
		"skills/one/SKILL.md": skillNamed("facts-one"),
		"skills/two/SKILL.md": skillNamed("facts-two"),
	}, nil)
	if len(res.Imported) != 2 {
		t.Fatalf("imported %d skills, want 2", len(res.Imported))
	}

	for _, imported := range res.Imported {
		var name, version, repository *string
		if err := pool.QueryRow(context.Background(),
			`SELECT s.plugin_name, s.plugin_version, s.plugin_repository
			   FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
			  WHERE v.id = $1`, uuidText(imported.Version.ID)).Scan(&name, &version, &repository); err != nil {
			t.Fatalf("reading the source row of %s: %v", imported.Path, err)
		}
		if name == nil || *name != "desk-tools" {
			t.Errorf("%s: plugin_name = %v, want desk-tools; without it nothing can say which Plugin this Skill came from",
				imported.Path, name)
		}
		if version == nil || *version != "1.4.0" {
			t.Errorf("%s: plugin_version = %v, want 1.4.0", imported.Path, version)
		}
		if repository == nil || *repository != "https://example.invalid/desk-tools" {
			t.Errorf("%s: plugin_repository = %v, want the manifest's own url", imported.Path, repository)
		}
	}
}

func TestASkillThatIsNotFromAPluginRecordsNoPluginFacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "no-plugin-facts")

	res := importSource(t, a, pool, owner, map[string]string{"SKILL.md": skillNamed("lonely")}, nil)
	imported := onlyImported(t, res)

	var name, version, repository *string
	if err := pool.QueryRow(context.Background(),
		`SELECT s.plugin_name, s.plugin_version, s.plugin_repository
		   FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
		  WHERE v.id = $1`, uuidText(imported.Version.ID)).Scan(&name, &version, &repository); err != nil {
		t.Fatal(err)
	}
	if name != nil || version != nil || repository != nil {
		t.Errorf("plugin facts = %v/%v/%v, want all unset; there was no Plugin", name, version, repository)
	}
}
