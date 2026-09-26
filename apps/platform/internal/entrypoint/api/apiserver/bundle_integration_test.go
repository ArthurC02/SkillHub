package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

func newestVersionID(t *testing.T, c *client, skillID string) string {
	t.Helper()
	versions := c.listVersions(t, skillID)
	if len(versions) == 0 {
		t.Fatalf("skill %s has no versions", skillID)
	}
	return versions[0].VersionID
}

func createBundle(t *testing.T, c *client, name, version string, memberVersionIDs ...string) (int, map[string]any) {
	t.Helper()
	ids, err := json.Marshal(memberVersionIDs)
	if err != nil {
		t.Fatal(err)
	}
	return postJSON(t, c, "/me/bundles",
		`{"name":"`+name+`","version":"`+version+`","description":"Tools that work on one desk.","member_version_ids":`+string(ids)+`}`)
}

func bundleOfTwo(t *testing.T, c *client, prefix string) (bundle string, skillIDs, skillNames []string) {
	t.Helper()
	bundle = freshName(prefix)
	var versionIDs []string
	for _, part := range []string{"tidy", "split"} {
		name := freshName(prefix + "-" + part)
		id := uploadedSkill(t, c, name, "Do the "+part+" carefully.")
		skillIDs, skillNames = append(skillIDs, id), append(skillNames, name)
		versionIDs = append(versionIDs, newestVersionID(t, c, id))
	}
	if code, body := createBundle(t, c, bundle, "1.0.0", versionIDs...); code != http.StatusCreated {
		t.Fatalf("creating bundle %s: %d %v", bundle, code, body)
	}
	return bundle, skillIDs, skillNames
}

func rawDownloads(t *testing.T, c *client) []map[string]any {
	t.Helper()
	var out struct {
		Downloads []map[string]any `json:"downloads"`
	}
	if code := getJSON(t, c.Client, c.base+"/downloads", &out); code != http.StatusOK {
		t.Fatalf("GET /downloads: got %d", code)
	}
	return out.Downloads
}

func namesOf(entries []map[string]any, key string) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name, _ := e[key].(string)
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sorted(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}

func TestABundleExportsAsAPluginThatImportsBackAsTheSameSkills(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("bundle-alice"))
	bob := a.login(t, freshName("bundle-bob"))
	bundle, _, skillNames := bundleOfTwo(t, alice, "bundle")

	code, body := postJSON(t, alice, "/me/bundles/"+bundle+"/export", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("exporting %s: %d %v", bundle, code, body)
	}
	artifactID, _ := body["artifact_id"].(string)
	if body["file_name"] != bundle+"-1.0.0.zip" || body["duplicate"] != false {
		t.Errorf("export = %v, want a new %s-1.0.0.zip", body, bundle)
	}

	downloads := rawDownloads(t, alice)
	if len(downloads) != 1 {
		t.Fatalf("alice's downloads = %v, want the one plugin", downloads)
	}
	plugin, _ := downloads[0]["plugin"].(map[string]any)
	if plugin == nil || plugin["name"] != bundle || plugin["version"] != "1.0.0" || downloads[0]["skill_id"] != nil {
		t.Errorf("the download row = %v, want a plugin %s 1.0.0 and no single skill", downloads[0], bundle)
	}
	if members := objects(t, plugin["members"]); strings.Join(namesOf(members, "name"), ",") != strings.Join(sorted(skillNames), ",") {
		t.Errorf("plugin members = %v, want %v", members, skillNames)
	}

	resp, data := alice.fetchContent(t, artifactID)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetching the plugin: %d", resp.StatusCode)
	}
	code, imported := postSource(t, bob, data)
	if code != http.StatusCreated || imported["shape"] != "plugin" {
		t.Fatalf("importing the exported plugin: %d shape %v (%v)", code, imported["shape"], imported)
	}
	var want []string
	for _, name := range sorted(skillNames) {
		want = append(want, "skills/"+name)
	}
	if got := namesOf(skillsOf(t, imported), "path"); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("the re-imported plugin carries %v, want the bundle's %v", got, skillNames)
	}

	if code, again := postJSON(t, alice, "/me/bundles/"+bundle+"/export", `{}`); code != http.StatusCreated || again["artifact_id"] != artifactID || again["duplicate"] != true {
		t.Errorf("exporting the same version again: %d %v, want the kept artifact %s", code, again, artifactID)
	}
}

func TestBundleRulesRefuseWhatAPluginCouldNotCarry(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("rules-alice"))
	bob := a.login(t, freshName("rules-bob"))
	name := freshName("rules-skill")
	skillID := uploadedSkill(t, alice, name, "First way.")
	first := newestVersionID(t, alice, skillID)
	uploadedSkill(t, alice, name, "Second way.")
	second := newestVersionID(t, alice, skillID)
	bobsVersion := newestVersionID(t, bob, uploadedSkill(t, bob, freshName("rules-bob-skill"), "Theirs."))

	for _, tc := range []struct {
		name, version, description string
		members                    []string
		wantCode                   int
		wantReason                 string
	}{
		{"a name the name rule refuses", "1.0.0", "Desk tools.", []string{first}, http.StatusUnprocessableEntity, "name_shape"},
		{"two parts to the version", "1.0", "Desk tools.", []string{first}, http.StatusUnprocessableEntity, "version_shape"},
		{"a leading zero", "1.01.0", "Desk tools.", []string{first}, http.StatusUnprocessableEntity, "version_shape"},
		{"no description", "1.0.0", "  ", []string{first}, http.StatusUnprocessableEntity, "description_missing"},
		{"no members", "1.0.0", "Desk tools.", []string{}, http.StatusUnprocessableEntity, "no_members"},
		{"two versions of one skill", "1.0.0", "Desk tools.", []string{first, second}, http.StatusUnprocessableEntity, "duplicate_skill"},
		{"someone else's version", "1.0.0", "Desk tools.", []string{bobsVersion}, http.StatusNotFound, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids, _ := json.Marshal(tc.members)
			bundleName := freshName("rules")
			if tc.wantReason == "name_shape" {
				bundleName = "Desk_Tools"
			}
			code, body := postJSON(t, alice, "/me/bundles",
				`{"name":"`+bundleName+`","version":"`+tc.version+`","description":"`+tc.description+`","member_version_ids":`+string(ids)+`}`)
			if code != tc.wantCode || (tc.wantReason != "" && body["reason"] != tc.wantReason) {
				t.Errorf("got %d %v, want %d %s", code, body, tc.wantCode, tc.wantReason)
			}
		})
	}

	bundle := freshName("rules")
	if code, body := createBundle(t, alice, bundle, "1.0.0-beta.1", second); code != http.StatusCreated {
		t.Fatalf("a pre-release version: %d %v, want 201", code, body)
	}
	if code, body := createBundle(t, alice, bundle, "1.0.0-beta.1", first); code != http.StatusConflict || body["reason"] != "version_exists" {
		t.Errorf("the same version again: %d %v, want 409 version_exists", code, body)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM bundle_versions bv JOIN bundles b ON b.id = bv.bundle_id WHERE b.workspace_id = $1`,
		mustUUID(t, alice.workspaceID)); n != 1 {
		t.Errorf("refused requests left %d bundle versions, want only the one accepted", n)
	}
}

func TestAPluginTakesItsWorstMembersTrustAndNamesIt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("worst-alice"))
	bundle, skillIDs, skillNames := bundleOfTwo(t, alice, "worst")
	setSkill(t, pool, skillIDs[1], "access_restriction = 'license-review'")

	code, body := postJSON(t, alice, "/me/bundles/"+bundle+"/export", `{}`)
	message, _ := body["error"].(string)
	if code != http.StatusUnprocessableEntity || body["reason"] != "license_hold" || !strings.HasPrefix(message, "成員 "+skillNames[1]+"：") {
		t.Errorf("exporting with one held member: %d %v, want 422 license_hold naming %s", code, body, skillNames[1])
	}
	if downloads := rawDownloads(t, alice); len(downloads) != 0 {
		t.Errorf("a refused export left %d downloads", len(downloads))
	}
}

func TestAPublishedBundleShowsItsMembersAndWhatChangedSinceTheLastRelease(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("pubbundle-alice"))
	bob := a.login(t, freshName("pubbundle-bob"))
	registerPublisher(t, alice, freshName("pubbundle"))
	bundle, skillIDs, skillNames := bundleOfTwo(t, alice, "pubbundle")

	code, own := postJSON(t, alice, "/me/bundles/"+bundle+"/publication", `{"rights_attested":true}`)
	if code != http.StatusOK || own["kind"] != "bundle" {
		t.Fatalf("publishing %s: %d %v", bundle, code, own)
	}
	address, _ := own["address"].(string)
	_, public := publicRead(t, a, address)
	if acquisition, _ := public["acquisition"].(map[string]any); !strings.Contains(fmt.Sprint(acquisition["note"]), "不含 MCP 設定或宿主專屬元件") {
		t.Errorf("the address offers the plugin without saying it carries only Agent Skills: %v", acquisition)
	}
	if code, body := publish(t, alice, skillIDs[1], `{"name":"`+bundle+`","rights_attested":true}`); code != http.StatusConflict || body["reason"] != "name_taken" {
		t.Errorf("publishing a Skill under the bundle's name: %d %v, want 409 name_taken; Skills and Bundles share one namespace", code, body)
	}
	if releases := objects(t, own["releases"]); len(releases) != 1 || releases[0]["bundle_version"] != "1.0.0" {
		t.Errorf("own releases = %v, want 1.0.0", releases)
	}

	uploadedSkill(t, alice, skillNames[0], "A better way.")
	third := freshName("pubbundle-third")
	thirdID := uploadedSkill(t, alice, third, "Something new.")
	if code, body := createBundle(t, alice, bundle, "1.1.0", newestVersionID(t, alice, skillIDs[0]), newestVersionID(t, alice, thirdID)); code != http.StatusCreated {
		t.Fatalf("bundle 1.1.0: %d %v", code, body)
	}
	if code, body := postJSON(t, alice, "/me/bundles/"+bundle+"/publication", `{"version":"1.1.0","rights_attested":true}`); code != http.StatusOK {
		t.Fatalf("publishing 1.1.0: %d %v", code, body)
	}

	code, public = publicRead(t, a, address)
	if code != http.StatusOK || public["kind"] != "bundle" {
		t.Fatalf("anonymous GET %s: %d %v", address, code, public)
	}
	current, _ := public["bundle"].(map[string]any)
	if current == nil || current["version"] != "1.1.0" || len(objects(t, current["members"])) != 2 {
		t.Errorf("the public bundle = %v, want 1.1.0 with two members", current)
	}
	releases := objects(t, public["releases"])
	if len(releases) != 2 || releases[1]["changes"] != nil {
		t.Fatalf("public releases = %v, want two, the first with no changes", releases)
	}
	changes := map[string]map[string]any{}
	for _, c := range objects(t, releases[0]["changes"]) {
		changes[c["name"].(string)] = c
	}
	if len(changes) != 3 ||
		changes[skillNames[0]]["change"] != "changed" || changes[skillNames[0]]["from"] != float64(1) || changes[skillNames[0]]["to"] != float64(2) ||
		changes[third]["change"] != "added" || changes[skillNames[1]]["change"] != "removed" {
		t.Errorf("1.1.0 changes = %v, want %s changed 1→2, %s added, %s removed", changes, skillNames[0], third, skillNames[1])
	}

	code, acquired := acquire(t, bob, address)
	if code != http.StatusCreated || acquired["file_name"] != bundle+"-1.1.0.zip" {
		t.Fatalf("bob acquiring the bundle: %d %v", code, acquired)
	}
	if downloads := rawDownloads(t, bob); len(downloads) != 1 || downloads[0]["plugin"] == nil {
		t.Errorf("bob's downloads = %v, want the plugin", downloads)
	}

	setSkill(t, pool, thirdID, "takedown_at = now(), takedown_reason = 'fixture'")
	_, public = publicRead(t, a, address)
	availability, _ := public["availability"].(map[string]any)
	note, _ := availability["note"].(string)
	if availability["value"] != "taken_down" || !strings.HasPrefix(note, "成員 "+third+"：") || public["bundle"] != nil {
		t.Errorf("with a member taken down the address says %v and carries %v, want taken_down naming %s and no content", availability, public["bundle"], third)
	}
	if code, body := acquire(t, bob, address); code != http.StatusConflict || body["reason"] != "taken_down" || body["error"] != note {
		t.Errorf("acquiring with a member taken down: %d %v, want 409 saying what the address says", code, body)
	}
	if code, body := postJSON(t, alice, "/me/bundles/"+bundle+"/publication", `{"version":"1.1.0","rights_attested":true}`); code != http.StatusUnprocessableEntity || body["reason"] != "member_withdrawn" {
		t.Errorf("republishing with a member taken down: %d %v, want 422 member_withdrawn", code, body)
	}
}

func TestABundleVersionHoldsItsMembersAgainstThePurgeOfADeletedSkill(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, freshName("hold-alice"))
	_, skillIDs, _ := bundleOfTwo(t, alice, "hold")
	versionID := newestVersionID(t, alice, skillIDs[0])

	if code, body := deleteJSON(t, alice, "/skills/"+skillIDs[0]); code != http.StatusOK {
		t.Fatalf("deleting the member skill: %d %v", code, body)
	}
	if _, err := pool.Exec(context.Background(), "UPDATE skills SET deleted_at = now() - interval '40 days' WHERE id = $1", mustUUID(t, skillIDs[0])); err != nil {
		t.Fatal(err)
	}
	if _, err := registryPurger(pool).PurgeDeletedSkills(context.Background(), 30*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM skill_versions WHERE id = $1`, mustUUID(t, versionID)); n != 1 {
		t.Errorf("the purge removed a version a bundle pins (%d rows)", n)
	}
}
