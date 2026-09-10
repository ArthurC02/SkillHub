package apiserver_test

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

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

type packageStore map[string][]byte

func (s packageStore) Get(_ context.Context, key string) ([]byte, error) {
	data, ok := s[key]
	if !ok {
		return nil, errors.New("no such object: " + key)
	}
	return data, nil
}

func (s packageStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/" + key + "?signature=test", nil
}

func (s packageStore) PresignPut(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://objects.test/" + key + "?signature=test&method=put", nil
}

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

func seedLicensedVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, expression, source string, sourceID pgtype.UUID) {
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
		SourceID:          sourceID,
		ContentHash:       "sha256:" + skillID,
		PackageObjectKey:  "packages/" + skillID + ".tar",
		Manifest:          []byte(`{}`),
		LicenseExpression: &expression,
		LicenseSource:     &source,
	}); err != nil {
		t.Fatal(err)
	}
}

type detail struct {
	SkillID string `json:"skill_id"`
	Name    string `json:"name"`
	Scope   string `json:"scope"`
	Tier    struct {
		Value string `json:"value"`
		Label string `json:"label"`
	} `json:"tier"`
	Category struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Note  string `json:"note"`
	} `json:"category"`
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
	Source *struct {
		Type          string `json:"type"`
		URL           string `json:"url"`
		SourceVersion string `json:"source_version"`
	} `json:"source"`
	Redistribution struct {
		Value string `json:"value"`
		Label string `json:"label"`
		Note  string `json:"note"`
	} `json:"redistribution"`
	Risk struct {
		ScanStatus  string                               `json:"scan_status"`
		Disclosures []struct{ Code, Label, Note string } `json:"disclosures"`
		Counts      struct {
			Errors, Warnings, Infos int
		} `json:"counts"`
	} `json:"risk"`
	Compatibility struct {
		SpecValidation struct{ Value, Label, Note string } `json:"spec_validation"`
		Capability     struct{ Value, Label, Note string } `json:"capability"`
		Runtime        struct{ Value, Label, Note string } `json:"runtime"`
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

func TestAnonymousReadsCatalogSkillDetail(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-detail")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-detail-skill")
	url, ref := "https://github.com/example/catalog-detail", "v1.0.0"
	source, err := gen.New(pool).CreateSkillSource(context.Background(), gen.CreateSkillSourceParams{
		WorkspaceID: mustUUID(t, curator.workspaceID), SourceType: "git", SourceUrl: &url,
		SourceRef: &ref, ContentHash: "sha256:catalog-detail-source",
		FetchedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	seedLicensedVersion(t, pool, curator.workspaceID, skillID, "MIT", "repo-license-file", source.ID)
	a.packages["packages/"+skillID+".tar"] = demoPackage(t)

	var got detail

	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
		t.Fatalf("anonymous GET /api/skills/{id}: want 200, got %d", code)
	}
	if got.SkillID != skillID || got.Scope != "catalog" {
		t.Fatalf("detail = %+v, want the catalog skill", got)
	}
	if got.Version == nil || got.Version.ContentHash == "" {
		t.Error("DISC-003: the version's content hash is missing from the detail")
	}
	if got.Source == nil || got.Source.URL != url || got.Source.SourceVersion != ref {
		t.Errorf("source = %+v, want workspace-scoped git provenance", got.Source)
	}

	if got.License.Expression != "MIT" || got.License.Source != "repo-license-file" {
		t.Errorf("license = %+v, want MIT/repo-license-file", got.License)
	}
	if got.License.Status.Value != "declared" {
		t.Errorf("license status = %q, want declared", got.License.Status.Value)
	}

	if got.Redistribution.Value != "unknown" {
		t.Errorf("redistribution = %+v; a declared MIT must not release a skill on its own", got.Redistribution)
	}

	if got.Risk.ScanStatus != "scanned" || !hasDisclosureCode(got.Risk.Disclosures, "script-file") {
		t.Errorf("risk = %+v, want a scan that found scripts/run.py", got.Risk)
	}

	if got.Compatibility.SpecValidation.Value != "passed" {
		t.Errorf("spec_validation = %q, want passed", got.Compatibility.SpecValidation.Value)
	}
	if got.Compatibility.Capability.Value != "unverified" || got.Compatibility.Runtime.Value != "unverified" {
		t.Errorf("compatibility = %+v, want capability and runtime unverified before M2", got.Compatibility)
	}
	if got.Tier.Value != "indexed" || got.Tier.Label == "" {
		t.Errorf("tier = %+v, want the indexed badge (curation is not recorded anywhere yet)", got.Tier)
	}

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

func TestSkillDetailReportsTheRedistributionVerdict(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-redistribution")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-redistribution-skill")
	seedLicensedVersion(t, pool, curator.workspaceID, skillID, "MIT", "manifest", pgtype.UUID{})
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

	mallory := a.login(t, "mallory-detail")
	if code := getJSON(t, mallory.Client, a.URL+"/api/skills/"+skillID, nil); code != http.StatusNotFound {
		t.Fatalf("another user's GET of a private skill: want 404, got %d", code)
	}
}

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

func TestUnreadablePackageIsReportedAsUnknownNotClean(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)

	curator := a.login(t, "curator-noobject")
	markCatalog(t, pool, curator.workspaceID)
	skillID := seedSkill(t, pool, curator.workspaceID, "catalog-noobject-skill")
	seedSkillVersion(t, pool, curator.workspaceID, skillID)

	var got detail
	if code := getJSON(t, http.DefaultClient, a.URL+"/api/skills/"+skillID, &got); code != http.StatusOK {
		t.Fatalf("detail with an unreadable package: want 200, got %d", code)
	}
	if got.Risk.ScanStatus != "unavailable" {
		t.Errorf("scan_status = %q, want unavailable", got.Risk.ScanStatus)
	}
	if got.Compatibility.SpecValidation.Value != "unverified" {
		t.Errorf("spec_validation = %q, want unverified when the package cannot be read", got.Compatibility.SpecValidation.Value)
	}
}
