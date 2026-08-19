// DISC-006/007/008/010 database-backed tests for the public detail and file
// views. Shared harness (TestMain, migrate, requireDB, login, seedSkill) lives
// in authz_integration_test.go; seedSkillVersion and markCatalog in
// disc_integration_test.go.
package identity_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// packageStore is an in-memory object store. Missing keys return an error,
// which is the same thing a real store does for a package that was never
// written — the case the detail view must answer without claiming a clean scan.
type packageStore map[string][]byte

func (s packageStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s[key]
	if !ok {
		return nil, errors.New("no such object: " + key)
	}
	return data, nil
}

// PresignGet and PresignPut complete run.ObjectStore. They answer for any key,
// including one that does not exist, because that is what S3 pre-signing does:
// signing a URL is a local operation over a key and never a lookup. The
// fail-closed case grantsFor actually guards is a deployment with no object
// store at all (SBX-008).
func (s packageStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/" + key + "?signature=test", nil
}

func (s packageStore) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/" + key + "?signature=test&method=put", nil
}

// demoPackage builds a valid Agent Skills zip carrying a script file, so the
// file tree has something to mark (DISC-003 Script 必須有明確標示).
func demoPackage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"SKILL.md":       "---\nname: demo-skill\ndescription: A demo skill.\nlicense: MIT\n---\n\nUse it like this.\n",
		"scripts/run.py": "print('hello')\n",
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// seedLicensedVersion is seedSkillVersion plus the ADR-021 license pair, so the
// detail response has a provenance tier to show. Written at insert time, because
// a version row is immutable once created (0005 trigger, iron rule 4) — the same
// reason the license and its tier are import-time facts and not editable state.
func seedLicensedVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, expression, source string) {
	t.Helper()
	var ws, sk pgtype.UUID
	if err := ws.Scan(workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := sk.Scan(skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.New(pool).CreateSkillVersion(context.Background(), gen.CreateSkillVersionParams{
		WorkspaceID:       ws,
		SkillID:           sk,
		ContentHash:       "sha256:" + skillID,
		PackageObjectKey:  "packages/" + skillID + ".tar",
		Manifest:          []byte(`{}`),
		LicenseExpression: &expression,
		LicenseSource:     &source,
	}); err != nil {
		t.Fatal(err)
	}
}

// detail is the subset of GET /api/skills/{id} these tests assert on.
type detail struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Tier    struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"tier"`
	Version *struct {
		VersionID   string `json:"version_id"`
		ContentHash string `json:"content_hash"`
	} `json:"version"`
	License struct {
		Expression string `json:"expression"`
		Source     string `json:"source"`
		Status     struct {
			Value string `json:"value"`
		} `json:"status"`
	} `json:"license"`
	Redistribution struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Note  string `json:"note"`
	} `json:"redistribution"`
	Risk struct {
		ScanStatus string `json:"scan_status"`
		HasScripts bool   `json:"has_scripts"`
		Counts     struct {
			Errors, Warnings, Infos int
		} `json:"counts"`
	} `json:"risk"`
	Compatibility struct {
		SpecValidation string `json:"spec_validation"`
		Capability     string `json:"capability"`
		Runtime        string `json:"runtime"`
	} `json:"compatibility"`
	Limitations []struct {
		Text   string `json:"text"`
		Source string `json:"source"`
	} `json:"limitations"`
}

func getJSON(t *testing.T, c *http.Client, url string, out any) int {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

// DISC-010: "公開搜尋與詳情不要求登入". The detail endpoint is the half that did
// not exist, so the acceptance criterion could not be verified at all.
// DISC-006/008 ride along: this asserts the response actually carries the source,
// license-with-provenance, risk, and compatibility blocks.
func TestAnonymousReadsCatalogSkillDetail(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-detail")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-detail-skill")
	seedLicensedVersion(t, pool, curator.workspaceID, skillID, "MIT", "repo-license-file")
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	var got detail
	// http.DefaultClient carries no cookie jar: this request has no session.
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
		t.Fatalf("anonymous GET /api/skills/{id}: want 200, got %d", code)
	}
	if got.SkillID != skillID || got.Scope != "catalog" {
		t.Fatalf("detail = %+v, want the catalog skill", got)
	}
	if got.Version == nil || got.Version.ContentHash == "" {
		t.Error("DISC-003: the version's content hash is missing from the detail")
	}
	// ADR-021: the tier the license was established at must survive to the view,
	// and a repo-level file is still only 已宣告, never 已人工確認.
	if got.License.Expression != "MIT" || got.License.Source != "repo-license-file" {
		t.Errorf("license = %+v, want MIT/repo-license-file", got.License)
	}
	if got.License.Status.Value != "declared" {
		t.Errorf("license status = %q, want declared", got.License.Status.Value)
	}
	// ADR-027 決策 4: every skill carries the verdict and it starts at `unknown`.
	// A declared licence does not move it — the two axes are separate, and
	// 02:CONTENT-002 rules out even a confirmed licence being the release
	// condition. This is the reading the packaging gate makes, on the same column.
	if got.Redistribution.Value != "unknown" {
		t.Errorf("redistribution = %+v; a declared MIT must not release a skill on its own", got.Redistribution)
	}
	// DISC-008: the risk block comes from a real scan of the stored package.
	if got.Risk.ScanStatus != "scanned" || !got.Risk.HasScripts {
		t.Errorf("risk = %+v, want a scan that found scripts/run.py", got.Risk)
	}
	// DISC-008: three axes, and the two that need a sandbox say so.
	if got.Compatibility.SpecValidation != "passed" {
		t.Errorf("spec_validation = %q, want passed", got.Compatibility.SpecValidation)
	}
	if got.Compatibility.Capability != "unverified" || got.Compatibility.Runtime != "unverified" {
		t.Errorf("compatibility = %+v, want capability and runtime unverified before M2", got.Compatibility)
	}
	if got.Tier.Value != "indexed" || got.Tier.Label == "" {
		t.Errorf("tier = %+v, want the indexed badge (curation is not recorded anywhere yet)", got.Tier)
	}
	// DISC-003 一般模式「限制」: the package ships a script, which is a
	// requirement on whoever runs it, so the scan half of the block must produce
	// a line for it even with no enrichment on the row at all. Labelled `scan`,
	// because the author's document never said this.
	if len(got.Limitations) == 0 {
		t.Error("DISC-003: no limitations for a package that ships a script")
	}
	for _, l := range got.Limitations {
		if l.Source != "scan" {
			t.Errorf("limitation %q labelled %q; nothing enriched this row", l.Text, l.Source)
		}
		if l.Text == "" {
			t.Error("limitation with no text")
		}
	}
}

// 02:SEC-007 / ADR-027 決策 4: the detail view is where the redistribution
// verdict becomes visible, and it reads the column the packaging gate reads.
// Three states, each arriving with the platform's own copy — a client mapping
// the value to its own wording would be a second place the three can be
// described differently (NFR-001), and the label is what tells `blocked` from
// `unknown` in front of a reader.
func TestSkillDetailReportsTheRedistributionVerdict(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-redistribution")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-redistribution-skill")
	seedLicensedVersion(t, pool, curator.workspaceID, skillID, "MIT", "manifest")
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	labels := map[string]string{}
	for _, want := range []string{"unknown", "allowed", "blocked"} {
		if _, err := pool.Exec(context.Background(),
			"UPDATE skills SET redistribution = $2 WHERE id = $1", mustUUID(t, skillID), want); err != nil {
			t.Fatal(err)
		}
		var got detail
		if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
			t.Fatalf("GET detail with redistribution=%s: want 200, got %d", want, code)
		}
		if got.Redistribution.Value != want {
			t.Errorf("redistribution value = %q, want %q", got.Redistribution.Value, want)
		}
		if got.Redistribution.Label == "" || got.Redistribution.Note == "" {
			t.Errorf("redistribution %q arrived without its copy: %+v", want, got.Redistribution)
		}
		if prev, dup := labels[got.Redistribution.Label]; dup {
			t.Errorf("%q and %q share the label %q; three states shown as two is one of them going missing",
				prev, want, got.Redistribution.Label)
		}
		labels[got.Redistribution.Label] = want
	}
}

// DISC-010 / WS-006: the public half of the endpoint is catalog-only. Another
// user's private skill answers 404, identical to a skill that does not exist.
func TestAnonymousCannotReadPrivateSkillDetail(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	bob := a.login(t, "bob-private-detail")
	skillID := seedSkill(t, pool, bob.workspaceID, "bob-private-detail-skill")
	seedSkillVersion(t, pool, bob.workspaceID, skillID)
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, nil); code != http.StatusNotFound {
		t.Fatalf("anonymous GET of a private skill: want 404, got %d", code)
	}
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID+"/files", nil); code != http.StatusNotFound {
		t.Fatalf("anonymous GET of a private skill's files: want 404, got %d", code)
	}
}

// DISC-010: "私有操作要求登入" — the owner sees the same information for their own
// private skill through the same route, which is what makes the 404 above a
// scope decision rather than a missing feature.
func TestOwnerReadsOwnPrivateSkillDetail(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	bob := a.login(t, "bob-owns-detail")
	skillID := seedSkill(t, pool, bob.workspaceID, "bob-owns-detail-skill")
	seedSkillVersion(t, pool, bob.workspaceID, skillID)
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	var got detail
	if code := getJSON(t, bob.Client, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
		t.Fatalf("owner GET of their own private skill: want 200, got %d", code)
	}
	if got.Scope != "private" {
		t.Errorf("scope = %q, want private", got.Scope)
	}

	// And a third party still cannot, session or not.
	mallory := a.login(t, "mallory-detail")
	if code := getJSON(t, mallory.Client, a.URL+"/api/skills/"+skillID, nil); code != http.StatusNotFound {
		t.Fatalf("another user's GET of a private skill: want 404, got %d", code)
	}
}

// DISC-007: SKILL.md full text plus the package file tree, scripts marked.
func TestSkillFilesServesSkillMDAndMarksScripts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-files")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-files-skill")
	seedSkillVersion(t, pool, curator.workspaceID, skillID)
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	var got struct {
		SkillID string `json:"skill_id"`
		SkillMD string `json:"skill_md"`
		Tree    []struct {
			Path     string `json:"path"`
			Size     int64  `json:"size"`
			IsScript bool   `json:"is_script"`
		} `json:"tree"`
	}
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID+"/files", &got); code != http.StatusOK {
		t.Fatalf("anonymous GET /api/skills/{id}/files: want 200, got %d", code)
	}
	if !bytes.Contains([]byte(got.SkillMD), []byte("Use it like this.")) {
		t.Errorf("skill_md does not carry the package's SKILL.md body: %q", got.SkillMD)
	}
	if len(got.Tree) != 2 {
		t.Fatalf("tree = %+v, want both package files", got.Tree)
	}
	for _, e := range got.Tree {
		switch e.Path {
		case "scripts/run.py":
			if !e.IsScript || e.Size == 0 {
				t.Errorf("scripts/run.py = %+v, want is_script with a size", e)
			}
		case "SKILL.md":
			if e.IsScript {
				t.Error("SKILL.md is marked as a script")
			}
		default:
			t.Errorf("unexpected tree entry %+v", e)
		}
	}
}

// A skill whose package never made it to the store still answers, with the scan
// reported as unavailable. Reporting "no findings" here would present an
// unreadable package as a clean one (DISC-004 不得自行推定為通過).
func TestUnreadablePackageIsReportedAsUnknownNotClean(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-noobject")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-noobject-skill")
	seedSkillVersion(t, pool, curator.workspaceID, skillID) // nothing seeded into a.packages

	var got detail
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
		t.Fatalf("detail with an unreadable package: want 200, got %d", code)
	}
	if got.Risk.ScanStatus != "unavailable" {
		t.Errorf("scan_status = %q, want unavailable", got.Risk.ScanStatus)
	}
	if got.Compatibility.SpecValidation != "unverified" {
		t.Errorf("spec_validation = %q, want unverified when the package cannot be read", got.Compatibility.SpecValidation)
	}
}
