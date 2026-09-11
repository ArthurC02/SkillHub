package apiserver_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func publishedWorkspace(ws gen.Workspace) identity.Workspace {
	return identity.Workspace{
		ID: ws.ID, OwnerUserID: ws.OwnerUserID, Name: ws.Name,
		CreatedAt: ws.CreatedAt, UpdatedAt: ws.UpdatedAt, IsCatalog: ws.IsCatalog,
	}
}

// Get blocks until two reads have started, and Put blocks until released, so
// two concurrent callers can be driven through the same race deterministically
// instead of by timing.
type packagingRaceStore struct {
	mu         sync.Mutex
	base       packageStore
	reads      int
	puts       int
	bothRead   chan struct{}
	firstPut   chan struct{}
	secondPut  chan struct{}
	releasePut chan struct{}
}

type packagingFaultStore struct {
	base      packageStore
	getErr    error
	existsErr error
	putErr    error
	puts      int
	removes   int
}

func (s *packagingFaultStore) Get(ctx context.Context, key string) ([]byte, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.base.Get(ctx, key)
}

func (s *packagingFaultStore) Exists(_ context.Context, key string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	_, ok := s.base[key]
	return ok, nil
}

func (s *packagingFaultStore) Put(ctx context.Context, key string, data []byte) error {
	s.puts++
	if err := s.base.Put(ctx, key, data); err != nil {
		return err
	}
	return s.putErr
}

func (s *packagingFaultStore) Remove(ctx context.Context, key string) error {
	s.removes++
	return s.base.Remove(ctx, key)
}

func newPackagingRaceStore(base packageStore) *packagingRaceStore {
	return &packagingRaceStore{
		base: base, bothRead: make(chan struct{}), firstPut: make(chan struct{}),
		secondPut: make(chan struct{}), releasePut: make(chan struct{}),
	}
}

func (s *packagingRaceStore) Get(ctx context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	data, ok := s.base[key]
	s.reads++
	if s.reads == 2 {
		close(s.bothRead)
	}
	bothRead := s.bothRead
	s.mu.Unlock()
	if !ok {
		return nil, errors.New("no such object: " + key)
	}
	select {
	case <-bothRead:
		return data, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *packagingRaceStore) Put(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	s.puts++
	switch s.puts {
	case 1:
		close(s.firstPut)
	case 2:
		close(s.secondPut)
	}
	releasePut := s.releasePut
	s.mu.Unlock()
	select {
	case <-releasePut:
	case <-ctx.Done():
		return ctx.Err()
	}
	s.mu.Lock()
	s.base[key] = data
	s.mu.Unlock()
	return nil
}

func (s *packagingRaceStore) Remove(_ context.Context, key string) error {
	s.mu.Lock()
	delete(s.base, key)
	s.mu.Unlock()
	return nil
}

func (s *packagingRaceStore) Exists(_ context.Context, key string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.base[key]
	return ok, nil
}

func packagingSKILLMD(name string) string {
	return "---\nname: " + name + "\ndescription: Reports on " + name +
		".\nlicense: MIT\n---\n\nUse it like this: [the script](scripts/run.py).\n"
}

func importFiles(
	t *testing.T, a *api, pool *pgxpool.Pool, owner *client, files map[string]string,
) (skillID, versionID string) {
	t.Helper()
	return importFilesEnriched(t, a, pool, owner, files, nil)
}

func importFilesEnriched(
	t *testing.T, a *api, pool *pgxpool.Pool, owner *client, files map[string]string, llm *llmclient.Client,
) (skillID, versionID string) {
	t.Helper()
	ctx := context.Background()
	ws, err := gen.New(pool).GetWorkspace(ctx, gen.GetWorkspaceParams{
		ID: mustUUID(t, owner.workspaceID), OwnerUserID: mustUUID(t, owner.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	svc := &ingest.Service{Pool: pool, Store: a.packages, IndexSkill: func(ctx context.Context, tx pgx.Tx, p ingest.SkillProjection) error {
		return catalog.IndexSkillEnriched(ctx, tx, catalog.EnrichedSkillProjection{
			SkillID: p.SkillID, WorkspaceID: p.WorkspaceID, Name: p.Name, Summary: p.Summary,
			EnrichedSummary: p.EnrichedSummary, TaskExamples: p.TaskExamples, Tags: p.Tags,
			Limitations: p.Limitations, Scan: p.Scan, Embedding: p.Embedding,
			EnrichmentStatus: p.EnrichmentStatus, EnrichmentModel: p.EnrichmentModel,
			EnrichmentPromptVersion: p.EnrichmentPromptVersion,
		})
	}}
	if llm != nil {
		svc.LLM = llm
	}
	res, err := svc.UploadZip(ctx, publishedWorkspace(ws), zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Blocked {
		t.Fatalf("fixture package did not import: %+v", res.Report.Findings)
	}
	if llm == nil {

		if _, err := pool.Exec(ctx, "UPDATE search_documents SET enrichment_status = 'enriched' WHERE skill_id = $1", res.Skill.ID); err != nil {
			t.Fatal(err)
		}
	}
	return uuidText(res.Skill.ID), uuidText(res.Version.ID)
}

func allowRedistribution(t *testing.T, pool *pgxpool.Pool, skillID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET redistribution = 'allowed' WHERE id = $1", mustUUID(t, skillID)); err != nil {
		t.Fatal(err)
	}
}

func packagedSkill(t *testing.T, a *api, pool *pgxpool.Pool, c *client, name string) (skillID, versionID string) {
	t.Helper()
	skillID, versionID = importFiles(t, a, pool, c, map[string]string{
		"SKILL.md":       packagingSKILLMD(name),
		"scripts/run.py": "print('hello')\n",
		"reference.md":   "Some reference material.\n",
	})
	allowRedistribution(t, pool, skillID)
	return skillID, versionID
}

func packagingPath(skillID, versionID string) string {
	return "/skills/" + skillID + "/versions/" + versionID + "/packaging"
}

func zipEntries(t *testing.T, a *api, contentHash string) map[string][]byte {
	t.Helper()
	var data []byte
	for key, candidate := range a.packages {
		if strings.HasPrefix(key, "downloads/") && strings.HasSuffix(key, "/"+contentHash+".zip") {
			data = candidate
			break
		}
	}
	if data == nil {
		t.Fatalf("no object was stored for content hash %s", contentHash)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		body := new(bytes.Buffer)
		if _, err := body.ReadFrom(rc); err != nil {
			t.Fatal(err)
		}
		rc.Close()
		out[f.Name] = body.Bytes()
	}
	return out
}

func TestEveryTargetProducesAPackageThePlatformWouldAcceptBack(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "packager")
	skillID, versionID := packagedSkill(t, a, pool, c, "round-trip-skill")

	source := a.packages[sourceKeyOf(t, pool, versionID)]

	for _, target := range []string{"standard", "claude-code", "claude-agent-sdk"} {
		t.Run(target, func(t *testing.T) {
			code, body := postJSON(t, c, packagingPath(skillID, versionID),
				`{"target":"`+target+`"}`)
			if code != http.StatusCreated {
				t.Fatalf("POST packaging: got %d, body %v", code, body)
			}
			hash, _ := body["content_hash"].(string)
			var produced []byte
			for key, candidate := range a.packages {
				if strings.HasPrefix(key, "downloads/") && strings.HasSuffix(key, "/"+hash+".zip") {
					produced = candidate
					break
				}
			}

			fsys, err := skillpkg.PackageFS(produced)
			if err != nil {
				t.Fatalf("the produced package could not be opened the way import opens one: %v", err)
			}
			report := skillpkg.Validate(fsys)
			if report.Blocked {
				t.Fatalf("the platform would refuse its own package: %+v", report.Findings)
			}
			if report.Manifest == nil || report.Manifest.Name != "round-trip-skill" {
				t.Fatalf("the round-tripped package is not the same skill: %+v", report.Manifest)
			}

			entries := zipEntries(t, a, hash)
			for name := range entries {
				if !fs.ValidPath(name) || strings.Contains(name, "..") || strings.ContainsAny(name, `\`) {
					t.Errorf("entry %q would escape on extraction", name)
				}
			}

			if _, ok := entries[entryPath(t, target, "round-trip-skill", "skillhub-manifest.json")]; !ok {
				t.Error("no manifest")
			}
			if _, ok := entries[entryPath(t, target, "round-trip-skill", "INSTALL.md")]; !ok {
				t.Error("no install instructions")
			}

			if target == "standard" {
				want := readFromZip(t, source, "SKILL.md")
				if got := entries["SKILL.md"]; !bytes.Equal(got, want) {
					t.Errorf("the standard package rewrote SKILL.md:\ngot  %q\nwant %q", got, want)
				}
			}
		})
	}
}

func entryPath(t *testing.T, target, skill, name string) string {
	t.Helper()
	if target == "standard" {
		return name
	}
	return skill + "/" + name
}

func readFromZip(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := zr.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(rc); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sourceKeyOf(t *testing.T, pool *pgxpool.Pool, versionID string) string {
	t.Helper()
	var key string
	if err := pool.QueryRow(context.Background(),
		"SELECT package_object_key FROM skill_versions WHERE id = $1",
		mustUUID(t, versionID)).Scan(&key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestBuildingTheSamePackageTwiceProducesTheSameContentHash(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "reproducer")
	skillID, versionID := packagedSkill(t, a, pool, c, "reproducible-skill")

	ctx := context.Background()
	ws, err := gen.New(pool).GetWorkspace(ctx, gen.GetWorkspaceParams{
		ID: mustUUID(t, c.workspaceID), OwnerUserID: mustUUID(t, c.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := a.packaging.Plan(ctx, publishedWorkspace(ws), mustUUID(t, skillID), mustUUID(t, versionID), "claude-code", false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	second, err := a.packaging.Plan(ctx, publishedWorkspace(ws), mustUUID(t, skillID), mustUUID(t, versionID), "claude-code", false)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Allowed || !second.Allowed {
		t.Fatalf("packaging was refused: %s / %s", first.BlockedReason, second.BlockedReason)
	}
	if first.ContentHash != second.ContentHash {
		t.Errorf("two builds produced different bytes:\n%s\n%s", first.ContentHash, second.ContentHash)
	}
	if first.ManifestHash != second.ManifestHash {
		t.Errorf("two builds produced different manifest hashes:\n%s\n%s", first.ManifestHash, second.ManifestHash)
	}
	if !bytes.Equal(first.Zip, second.Zip) {
		t.Error("the hashes matched but the bytes did not, which means one of them is not over the bytes")
	}
}

func TestAWorkspaceCanDownloadWhatItSuppliedItself(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "brought-my-own")

	skillID, versionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: my-own-skill\ndescription: A skill I wrote myself.\nlicense: MIT\n---\n\nDo the thing.\n",
		"LICENSE":  "MIT License\n\nPermission is hereby granted, free of charge...\n",
	})

	var redistribution string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id = $1", mustUUID(t, skillID),
	).Scan(&redistribution); err != nil {
		t.Fatal(err)
	}

	if redistribution != "self_supplied" {
		t.Errorf("redistribution = %q, want self_supplied", redistribution)
	}

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d (%v / %v)", code, body["blocked_reason"], body["blocked_message"])
	}
	artifactID, _ := body["artifact_id"].(string)
	if artifactID == "" {
		t.Fatalf("no artifact id in %v", body)
	}

	resp, err := c.Get(c.base + "/downloads/" + artifactID + "/content")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET content: got %d", resp.StatusCode)
	}
	if n, _ := io.Copy(io.Discard, resp.Body); n == 0 {
		t.Error("the download served no bytes")
	}
}

func TestAnUploadIntoTheCatalogueIsNotSelfSupplied(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "curator")

	if _, err := pool.Exec(context.Background(),
		"UPDATE workspaces SET is_catalog = true WHERE id = $1", mustUUID(t, c.workspaceID),
	); err != nil {
		t.Fatal(err)
	}

	skillID, versionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: curated-skill\ndescription: Something the platform is handing out.\n---\n\nDo the thing.\n",
	})

	var redistribution string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id = $1", mustUUID(t, skillID),
	).Scan(&redistribution); err != nil {
		t.Fatal(err)
	}
	if redistribution != "unknown" {
		t.Errorf("redistribution = %q, want unknown: the catalogue is content the "+
			"platform hands to other people, so it still needs a verdict", redistribution)
	}
	if code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code !=
		http.StatusUnprocessableEntity || body["blocked_reason"] != "license_unknown" {
		t.Errorf("POST packaging: got %d / %v, want 422 license_unknown", code, body["blocked_reason"])
	}
}

func TestEachGateRefusesAPackageAndSaysWhich(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "gatekeeper")

	for _, tc := range []struct {
		name, setup, wantReason string
	}{
		{
			name:       "a licensing hold",
			setup:      "UPDATE skills SET redistribution='allowed', access_restriction='license-review' WHERE id=$1",
			wantReason: "license_hold",
		},
		{

			name:       "a hold under a reason code nothing recognises",
			setup:      "UPDATE skills SET redistribution='allowed', access_restriction='some-future-reason' WHERE id=$1",
			wantReason: "license_hold",
		},
		{
			name:       "content that may not be redistributed",
			setup:      "UPDATE skills SET redistribution='blocked' WHERE id=$1",
			wantReason: "not_redistributable",
		},
		{

			name:       "a licence nobody established",
			setup:      "UPDATE skills SET redistribution='unknown' WHERE id=$1",
			wantReason: "license_unknown",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			skillID, versionID := packagedSkill(t, a, pool, c, "gated-"+strings.ReplaceAll(tc.name, " ", "-"))
			if _, err := pool.Exec(context.Background(), tc.setup, mustUUID(t, skillID)); err != nil {
				t.Fatal(err)
			}

			code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
			if code != http.StatusUnprocessableEntity {
				t.Fatalf("POST packaging: got %d, body %v", code, body)
			}
			if got := body["blocked_reason"]; got != tc.wantReason {
				t.Errorf("blocked_reason = %v, want %s", got, tc.wantReason)
			}
			if n := downloadArtifactsFor(t, pool, versionID); n != 0 {
				t.Errorf("a refused request left %d artifacts behind", n)
			}

			var preview map[string]any
			if got := getJSON(t, c.Client,
				c.base+packagingPath(skillID, versionID)+"/preview?target=standard", &preview); got != http.StatusOK {
				t.Fatalf("GET preview: got %d", got)
			}
			if preview["allowed"] != false || preview["blocked_reason"] != tc.wantReason {
				t.Errorf("the preview disagrees with the packaging call: %v", preview)
			}
		})
	}
}

func TestASecretBearingPackageCannotBeBuiltAndBuildResidueNeverTravels(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "leaker")

	skillID := seedSkill(t, pool, c.workspaceID, "secret-bearing-skill")
	withSecret := zipOf(t, map[string]string{
		"SKILL.md": packagingSKILLMD("secret-bearing-skill"),
		".env":     "OPENAI_API_KEY=sk-thisisnotarealkeybutitmatchesthepattern0123456789\n",
	})
	seedStoredVersion(t, a, pool, c, skillID, "secretbearing", withSecret)
	allowRedistribution(t, pool, skillID)
	versionID := latestVersionID(t, pool, skillID)

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("a package carrying a credential was built: got %d, body %v", code, body)
	}
	if body["blocked_reason"] != "validation_blocked" {
		t.Errorf("blocked_reason = %v, want validation_blocked", body["blocked_reason"])
	}

	validation, _ := body["validation"].(map[string]any)
	errs, _ := validation["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("a validation refusal carried no errors: %v", validation)
	}
	for _, f := range errs {
		if sev := f.(map[string]any)["severity"]; sev != "error" {
			t.Errorf("finding severity = %v, want error: %v", sev, f)
		}
	}
	if n := downloadArtifactsFor(t, pool, versionID); n != 0 {
		t.Errorf("a refused build left %d artifacts behind", n)
	}

	residueID := seedSkill(t, pool, c.workspaceID, "residue-skill")
	withResidue := zipOf(t, map[string]string{
		"SKILL.md":    packagingSKILLMD("residue-skill"),
		".git/config": "[remote \"origin\"]\n\turl = https://ghp_0123456789012345678901234567890123456@github.com/x/y\n",
		"notes.md":    "Ordinary content.\n",
	})
	seedStoredVersion(t, a, pool, c, residueID, "gitresidue", withResidue)
	allowRedistribution(t, pool, residueID)
	residueVersionID := latestVersionID(t, pool, residueID)

	code, body = postJSON(t, c, packagingPath(residueID, residueVersionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	for name, content := range zipEntries(t, a, hash) {
		if strings.HasPrefix(name, ".git/") {
			t.Errorf("%s travelled", name)
		}
		if bytes.Contains(content, []byte("ghp_")) {
			t.Errorf("%s carries a credential", name)
		}
	}
}

func seedStoredVersion(
	t *testing.T, a *api, pool *pgxpool.Pool, c *client, skillID, hash string, data []byte,
) {
	t.Helper()
	if err := a.packages.Put(context.Background(), "packages/"+hash+".zip", data); err != nil {
		t.Fatal(err)
	}
	source, err := gen.New(pool).CreateSkillSource(context.Background(), gen.CreateSkillSourceParams{
		WorkspaceID: mustUUID(t, c.workspaceID), SourceType: "upload", ContentHash: hash,
		FetchedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.New(pool).CreateSkillVersion(context.Background(), gen.CreateSkillVersionParams{
		WorkspaceID: mustUUID(t, c.workspaceID), SkillID: mustUUID(t, skillID), SourceID: source.ID,
		ContentHash: hash, PackageObjectKey: "packages/" + hash + ".zip", Manifest: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
}

func downloadArtifactsFor(t *testing.T, pool *pgxpool.Pool, versionID string) int {
	t.Helper()
	return countRow(t, pool,
		"SELECT count(*) FROM download_artifacts WHERE skill_version_id = $1", mustUUID(t, versionID))
}

func latestVersionID(t *testing.T, pool *pgxpool.Pool, skillID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(),
		"SELECT id::text FROM skill_versions WHERE skill_id=$1 ORDER BY version_number DESC LIMIT 1",
		mustUUID(t, skillID)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPackagingTheSameThingTwiceReturnsTheArtifactThatAlreadyExists(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "repeater")
	skillID, versionID := packagedSkill(t, a, pool, c, "idempotent-skill")

	code, first := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("first POST: got %d, body %v", code, first)
	}
	if first["duplicate"] != false {
		t.Errorf("the first package was reported as a duplicate: %v", first)
	}
	code, second := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("second POST: got %d, body %v", code, second)
	}
	if second["duplicate"] != true {
		t.Errorf("the second package was not reported as a duplicate: %v", second)
	}
	if second["artifact_id"] != first["artifact_id"] {
		t.Errorf("a second artifact was produced: %v vs %v", second["artifact_id"], first["artifact_id"])
	}
	if n := downloadArtifactsFor(t, pool, versionID); n != 1 {
		t.Errorf("%d download artifacts exist for this version, want 1", n)
	}

	code, other := postJSON(t, c, packagingPath(skillID, versionID),
		`{"target":"standard","include_test_cases":true}`)
	if code != http.StatusCreated {
		t.Fatalf("third POST: got %d, body %v", code, other)
	}
	if other["duplicate"] != false || other["artifact_id"] == first["artifact_id"] {
		t.Errorf("packaging with test cases reused the artifact built without them: %v", other)
	}
}

func TestConcurrentPackagingCreatesOneArtifact(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "concurrent-repeater")
	skillID, versionID := packagedSkill(t, a, pool, c, "concurrent-idempotent-skill")
	ws, err := gen.New(pool).GetWorkspace(context.Background(), gen.GetWorkspaceParams{
		ID: mustUUID(t, c.workspaceID), OwnerUserID: mustUUID(t, c.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	store := newPackagingRaceStore(a.packages)
	a.packaging.Store = store

	type outcome struct {
		result packaging.Result
		err    error
	}
	start := make(chan struct{})
	out := make(chan outcome, 2)
	skillUUID, versionUUID := mustUUID(t, skillID), mustUUID(t, versionID)
	for range 2 {
		go func() {
			<-start
			result, err := a.packaging.Create(context.Background(), publishedWorkspace(ws),
				skillUUID, versionUUID, "standard", false)
			out <- outcome{result, err}
		}()
	}
	close(start)
	select {
	case <-store.firstPut:
	case <-time.After(5 * time.Second):
		t.Fatal("neither request reached the object write")
	}
	select {
	case <-store.secondPut:
		t.Fatal("both requests reached object storage; the idempotency lock did not serialize them")
	case <-time.After(250 * time.Millisecond):
		close(store.releasePut)
	}
	first, second := <-out, <-out
	for _, got := range []outcome{first, second} {
		if got.err != nil {
			t.Fatal(got.err)
		}
	}
	if first.result.Artifact.ArtifactID != second.result.Artifact.ArtifactID {
		t.Fatalf("concurrent requests created different artifacts: %s and %s",
			first.result.Artifact.ArtifactID, second.result.Artifact.ArtifactID)
	}
	if first.result.Duplicate == second.result.Duplicate {
		t.Fatalf("duplicate flags = %t and %t, want one producer and one reuse",
			first.result.Duplicate, second.result.Duplicate)
	}
	if n := downloadArtifactsFor(t, pool, versionID); n != 1 {
		t.Errorf("%d artifacts exist after concurrent packaging, want 1", n)
	}
}

func TestMutablePackagingInputsProduceNewArtifacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "mutable-package-inputs")
	makeCatalog(t, pool, curator.workspaceID)
	skillID, versionID := packagedSkill(t, a, pool, curator, "mutable-input-skill")

	build := func(include bool) map[string]any {
		t.Helper()
		code, body := postJSON(t, curator, packagingPath(skillID, versionID), fmt.Sprintf(
			`{"target":"standard","include_test_cases":%t}`, include))
		if code != http.StatusCreated {
			t.Fatalf("POST packaging: got %d, body %v", code, body)
		}
		return body
	}
	beforeCompatibility := build(false)
	if _, err := pool.Exec(context.Background(), `INSERT INTO skill_runtime_compatibility
        (skill_version_id, runtime_image, capability, runtime)
        VALUES ($1, 'ghcr.io/example/runtime@sha256:1111', 'activated', 'native')`,
		mustUUID(t, versionID)); err != nil {
		t.Fatal(err)
	}
	afterCompatibility := build(false)
	if afterCompatibility["duplicate"] != false ||
		afterCompatibility["content_hash"] == beforeCompatibility["content_hash"] {
		t.Errorf("a new compatibility measurement reused stale bytes: before=%v after=%v",
			beforeCompatibility, afterCompatibility)
	}

	testCaseID := seedTestCase(t, pool, curator.workspaceID, skillID)
	beforeEdit := build(true)
	if _, err := pool.Exec(context.Background(),
		"UPDATE test_cases SET user_prompt = 'a changed portable prompt' WHERE id = $1",
		mustUUID(t, testCaseID)); err != nil {
		t.Fatal(err)
	}
	afterEdit := build(true)
	if afterEdit["duplicate"] != false || afterEdit["content_hash"] == beforeEdit["content_hash"] {
		t.Errorf("an edited portable Test Case reused stale bytes: before=%v after=%v", beforeEdit, afterEdit)
	}
}

func TestPackagingObjectFailuresAreFailClosedAndCompensated(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "package-object-failures")
	skillID, versionID := packagedSkill(t, a, pool, owner, "object-failure-skill")
	ws, err := gen.New(pool).GetWorkspace(context.Background(), gen.GetWorkspaceParams{
		ID: mustUUID(t, owner.workspaceID), OwnerUserID: mustUUID(t, owner.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	create := func() error {
		_, err := a.packaging.Create(context.Background(), publishedWorkspace(ws), mustUUID(t, skillID),
			mustUUID(t, versionID), "standard", false)
		return err
	}

	getFailure := &packagingFaultStore{base: a.packages, getErr: errors.New("stored package unreadable")}
	a.packaging.Store = getFailure
	if err := create(); err == nil || getFailure.puts != 0 || getFailure.removes != 0 {
		t.Fatalf("source Get failure was not fail-closed: err=%v puts=%d removes=%d",
			err, getFailure.puts, getFailure.removes)
	}
	for key := range a.packages {
		if strings.HasPrefix(key, "downloads/"+owner.workspaceID+"/") {
			t.Errorf("a failed source read still left a download object %q", key)
		}
	}

	existsFailure := &packagingFaultStore{base: a.packages, existsErr: errors.New("exists unavailable")}
	a.packaging.Store = existsFailure
	if err := create(); err == nil || existsFailure.puts != 0 || existsFailure.removes != 0 {
		t.Fatalf("Exists failure was not fail-closed: err=%v puts=%d removes=%d",
			err, existsFailure.puts, existsFailure.removes)
	}

	putFailure := &packagingFaultStore{base: a.packages, putErr: errors.New("lost Put response")}
	a.packaging.Store = putFailure
	if err := create(); err == nil {
		t.Fatal("ambiguous Put failure reported success")
	}
	if putFailure.puts != 1 || putFailure.removes != 1 {
		t.Fatalf("ambiguous Put: puts=%d removes=%d, want one write and one compensation",
			putFailure.puts, putFailure.removes)
	}
	for key := range a.packages {
		if strings.HasPrefix(key, "downloads/"+owner.workspaceID+"/") {
			t.Errorf("compensation left download object %q", key)
		}
	}
}

func TestForkCarriesTheRedistributionVerdict(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "curator")
	makeCatalog(t, pool, owner.workspaceID)
	skillID, _ := packagedSkill(t, a, pool, owner, "forkable-skill")

	forker := a.login(t, "forker")
	code, body := postJSON(t, forker, "/skills/"+skillID+"/fork", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, body)
	}
	forkID, _ := body["skill_id"].(string)
	var got string
	if err := pool.QueryRow(context.Background(),
		"SELECT redistribution FROM skills WHERE id=$1", mustUUID(t, forkID)).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "allowed" {
		t.Errorf("the fork's redistribution is %q; an established verdict was lost in the copy", got)
	}

	forkVersionID := latestVersionID(t, pool, forkID)
	code, body = postJSON(t, forker, packagingPath(forkID, forkVersionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging a fork: got %d, body %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	raw := zipEntries(t, a, hash)["skillhub-manifest.json"]
	dumpManifest(t, "fork-origin", raw)

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	origin := m["source"].(map[string]any)["origin"].(map[string]any)
	if origin["kind"] != "fork" {
		t.Fatalf("origin kind = %v, want fork: %v", origin["kind"], origin)
	}
	if origin["upstream_skill_id"] != skillID {
		t.Errorf("upstream_skill_id = %v, want %s", origin["upstream_skill_id"], skillID)
	}
	if chain, _ := origin["chain"].([]any); len(chain) == 0 {
		t.Errorf("the fork chain is empty: %v", origin)
	}
	if origin["root_source"] == nil {
		t.Error("no root source; a gap reads as 'there was no upstream', which is a different and false statement")
	}
}

func TestOnlyCuratedTestCasesTravelAndTheRestAreNamed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	user := a.login(t, "test-case-owner")
	skillID, versionID := packagedSkill(t, a, pool, user, "tested-skill")
	seedTestCase(t, pool, user.workspaceID, skillID)

	withFile := seedTestCase(t, pool, user.workspaceID, skillID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO datasets (workspace_id, test_case_id, file_name, content_type,
		                      size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'private.csv', 'text/csv', 12, 'sha256:private',
		        'datasets/private.csv', now() + interval '90 days')`,
		mustUUID(t, user.workspaceID), mustUUID(t, withFile)); err != nil {
		t.Fatal(err)
	}

	var preview map[string]any
	if code := getJSON(t, user.Client,
		user.base+packagingPath(skillID, versionID)+"/preview?target=standard&include_test_cases=true",
		&preview); code != http.StatusOK {
		t.Fatalf("GET preview: got %d", code)
	}
	if n := len(preview["included_test_cases"].([]any)); n != 0 {
		t.Errorf("%d test cases of a user's own workspace were packaged; only curated content travels", n)
	}
	excluded, _ := preview["excluded_test_cases"].([]any)
	if len(excluded) != 2 {
		t.Fatalf("both excluded test cases were not reported: %v", preview)
	}
	reasons := map[string]string{}
	for _, e := range excluded {
		row := e.(map[string]any)
		reasons[row["test_case_id"].(string)], _ = row["reason"].(string)
	}
	if reasons[withFile] != "user_uploaded_dataset" {
		t.Errorf("exclusion reason for the case carrying a file = %q, want user_uploaded_dataset", reasons[withFile])
	}
	for id, reason := range reasons {
		if id != withFile && reason != "not_curated" {
			t.Errorf("exclusion reason for the case with no file = %q, want not_curated", reason)
		}
	}

	curator := a.login(t, "content-curator")
	makeCatalog(t, pool, curator.workspaceID)
	curatedSkill, curatedVersion := packagedSkill(t, a, pool, curator, "curated-skill")
	seedTestCase(t, pool, curator.workspaceID, curatedSkill)

	code, body := postJSON(t, curator, packagingPath(curatedSkill, curatedVersion),
		`{"target":"standard","include_test_cases":true}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	if body["includes_test_cases"] != true {
		t.Errorf("includes_test_cases = %v", body["includes_test_cases"])
	}
	hash, _ := body["content_hash"].(string)
	entries := zipEntries(t, a, hash)
	var caseJSON []byte
	for name, content := range entries {
		if strings.HasPrefix(name, "test-cases/") && strings.HasSuffix(name, "/case.json") {
			caseJSON = content
		}
	}
	if caseJSON == nil {
		t.Fatalf("no portable test case in the package: %v", keysOf(entries))
	}
	var portable map[string]any
	if err := json.Unmarshal(caseJSON, &portable); err != nil {
		t.Fatal(err)
	}

	if portable["origin"] != "curated" {
		t.Errorf("origin = %v, want curated", portable["origin"])
	}
	for _, banned := range []string{"run_id", "evaluation", "trace", "verdict"} {
		if _, present := portable[banned]; present {
			t.Errorf("the portable test case carries %q, which is Run data", banned)
		}
	}
}

func TestAForksCuratedTestCasesTravelAndItsOwnStillDoNot(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	curator := a.login(t, "curator-with-tests")
	makeCatalog(t, pool, curator.workspaceID)
	curatedSkill, _ := packagedSkill(t, a, pool, curator, "curated-with-tests")
	curatedCase := seedTestCase(t, pool, curator.workspaceID, curatedSkill)

	forker := a.login(t, "fork-and-package")
	code, body := postJSON(t, forker, "/skills/"+curatedSkill+"/fork", `{}`)
	if code != http.StatusCreated {
		t.Fatalf("POST fork: got %d, body %v", code, body)
	}
	forkID, _ := body["skill_id"].(string)

	ownCase := seedTestCase(t, pool, forker.workspaceID, forkID)

	forkVersionID := latestVersionID(t, pool, forkID)
	code, body = postJSON(t, forker, packagingPath(forkID, forkVersionID),
		`{"target":"standard","include_test_cases":true}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging a fork: got %d, body %v", code, body)
	}
	if body["includes_test_cases"] != true {
		t.Errorf("includes_test_cases = %v; the curated case did not travel", body["includes_test_cases"])
	}

	hash, _ := body["content_hash"].(string)
	entries := zipEntries(t, a, hash)
	var carried []string
	for name, content := range entries {
		if !strings.HasPrefix(name, "test-cases/") || !strings.HasSuffix(name, "/case.json") {
			continue
		}
		carried = append(carried, name)
		var portable map[string]any
		if err := json.Unmarshal(content, &portable); err != nil {
			t.Fatal(err)
		}
		if portable["origin"] != "curated" {
			t.Errorf("%s origin = %v, want curated", name, portable["origin"])
		}
	}
	if len(carried) != 1 {
		t.Fatalf("%d portable test cases in the fork's package, want exactly the curated one: %v",
			len(carried), keysOf(entries))
	}

	var preview map[string]any
	if code := getJSON(t, forker.Client,
		forker.base+packagingPath(forkID, forkVersionID)+"/preview?target=standard&include_test_cases=true",
		&preview); code != http.StatusOK {
		t.Fatalf("GET preview: got %d", code)
	}
	included, _ := preview["included_test_cases"].([]any)
	if len(included) != 1 {
		t.Fatalf("preview lists %d included test cases, want 1: %v", len(included), preview["included_test_cases"])
	}
	if id, _ := included[0].(map[string]any)["test_case_id"].(string); id != curatedCase {
		t.Errorf("the included test case is %s, want the curated one %s", id, curatedCase)
	}
	excluded, _ := preview["excluded_test_cases"].([]any)
	if len(excluded) != 1 {
		t.Fatalf("preview lists %d excluded test cases, want the forker's own: %v", len(excluded), preview["excluded_test_cases"])
	}
	row, _ := excluded[0].(map[string]any)
	if id, _ := row["test_case_id"].(string); id != ownCase {
		t.Errorf("the excluded test case is %s, want the forker's own %s", id, ownCase)
	}
	if row["reason"] != "not_curated" {
		t.Errorf("exclusion reason = %v, want not_curated", row["reason"])
	}
}

const mitText = `MIT License

Copyright (c) 2026 A. Author <author@example.test>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction.
`

const carriedProvenance = `{"carried_from":"LICENSE","repo":"https://github.com/example/skills","commit":"abc123"}`

func licensedSKILLMD(name string) string {
	return "---\nname: " + name + "\ndescription: Reports on " + name + ".\n---\n\nProse.\n"
}

func TestTheLicenceAuthorAndProvenanceFilesTravelInEveryTargetsPackage(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "licence-keeper")

	source := map[string]string{
		"SKILL.md":                     licensedSKILLMD("attributed-skill"),
		"LICENSE":                      mitText,
		"LICENSE.repo":                 mitText,
		"LICENSE.repo.provenance.json": carriedProvenance,
		"scripts/run.py":               "print('hello')\n",
	}
	skillID, versionID := importFiles(t, a, pool, c, source)
	allowRedistribution(t, pool, skillID)

	for _, target := range []string{"standard", "claude-code", "claude-agent-sdk"} {
		t.Run(target, func(t *testing.T) {
			code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"`+target+`"}`)
			if code != http.StatusCreated {
				t.Fatalf("POST packaging: got %d, body %v", code, body)
			}
			entries := zipEntries(t, a, body["content_hash"].(string))

			for _, name := range []string{"LICENSE", "LICENSE.repo", "LICENSE.repo.provenance.json"} {
				got, ok := entries[entryPath(t, target, "attributed-skill", name)]
				if !ok {
					t.Errorf("%s is not in the package; the licence did not travel with the bytes it licenses", name)
					continue
				}
				if string(got) != source[name] {
					t.Errorf("%s was rewritten:\ngot  %q\nwant %q", name, got, source[name])
				}
			}

			if !bytes.Contains(entries[entryPath(t, target, "attributed-skill", "LICENSE")], []byte("A. Author <author@example.test>")) {
				t.Error("the copyright holder did not survive the copy")
			}

			var m map[string]any
			manifest := entries[entryPath(t, target, "attributed-skill", "skillhub-manifest.json")]
			if err := json.Unmarshal(manifest, &m); err != nil {
				t.Fatal(err)
			}
			lic := m["license"].(map[string]any)
			if lic["expression"] != "MIT" || lic["source_tier"] != "package-license-file" {
				t.Errorf("licence = %v; the package states MIT in a root LICENSE file", lic)
			}
		})
	}
}

func TestThePreviewCarriesTheDependenciesTheInstallInstructionsWillList(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "dependency-reader")
	skillID, versionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md":       licensedSKILLMD("undeclaring-skill"),
		"LICENSE":        mitText,
		"scripts/run.py": "import pandas\n\nprint(pandas)\n",
	})
	allowRedistribution(t, pool, skillID)

	var preview struct {
		Allowed      bool     `json:"allowed"`
		Dependencies []string `json:"dependencies"`
	}
	if code := getJSON(t, c.Client,
		c.base+packagingPath(skillID, versionID)+"/preview?target=standard", &preview); code != http.StatusOK {
		t.Fatalf("GET preview: got %d", code)
	}
	if !preview.Allowed {
		t.Fatal("the fixture package was refused")
	}
	joined := strings.Join(preview.Dependencies, "\n")
	if !strings.Contains(joined, "pandas") {
		t.Errorf("the preview does not name the dependency: %v", preview.Dependencies)
	}
	if !strings.Contains(joined, "從未宣告") {
		t.Errorf("the preview does not say the dependency was never declared: %v", preview.Dependencies)
	}

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	install := zipEntries(t, a, body["content_hash"].(string))["INSTALL.md"]
	for _, line := range preview.Dependencies {
		if !bytes.Contains(install, []byte(line)) {
			t.Errorf("INSTALL.md does not carry the preview's line %q", line)
		}
	}
}

func TestThePreviewSaysHowLongThePackageIsKeptAndRefusesWhenNobodyDecided(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "retention-disclosure")
	skillID, versionID := packagedSkill(t, a, pool, c, "kept-for-a-while")
	a.packaging.Retention = policy.DownloadRetention(72 * time.Hour)

	var preview struct {
		Allowed       bool `json:"allowed"`
		RetentionDays int  `json:"retention_days"`
	}
	previewURL := c.base + packagingPath(skillID, versionID) + "/preview?target=standard"
	if code := getJSON(t, c.Client, previewURL, &preview); code != http.StatusOK {
		t.Fatalf("GET preview: got %d", code)
	}
	if !preview.Allowed {
		t.Fatal("the fixture package was refused")
	}
	if preview.RetentionDays != 3 {
		t.Errorf("preview.retention_days = %d, want 3 for a 72h deployment", preview.RetentionDays)
	}

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	expiresAt, ok := body["expires_at"].(string)
	if !ok {
		t.Fatalf("the created artifact carries no expires_at: %v", body)
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		t.Fatal(err)
	}
	disclosed := time.Duration(preview.RetentionDays) * 24 * time.Hour
	if left := time.Until(expires); left > disclosed+time.Hour || left < disclosed-time.Hour {
		t.Errorf("the preview promised %s of retention but the artifact expires in %s", disclosed, left)
	}

	a.packaging.Retention = 0
	if code := getJSON(t, c.Client, previewURL, &preview); code != http.StatusServiceUnavailable {
		t.Errorf("GET preview with no ratified retention: got %d, want 503", code)
	}
	if code, _ := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`); code != http.StatusServiceUnavailable {
		t.Errorf("POST packaging with no ratified retention: got %d, want 503", code)
	}
}

func TestTheStandardPackageIsTheSourceBytesPlusExactlyThreeFiles(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "round-tripper")

	source := map[string]string{
		"SKILL.md":       packagingSKILLMD("byte-stable-skill"),
		"scripts/run.py": "print('hello')\n",
		"reference.md":   "Some reference material.\n",
		".editorconfig":  "root = true\n",
	}
	skillID, versionID := importFiles(t, a, pool, c, source)
	allowRedistribution(t, pool, skillID)

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	entries := zipEntries(t, a, body["content_hash"].(string))

	stripped := map[string]string{}
	for name, content := range entries {
		if name == "skillhub-manifest.json" || name == "INSTALL.md" ||
			strings.HasPrefix(name, "test-cases/") {
			continue
		}
		stripped[name] = string(content)
	}
	if len(stripped) != len(source) {
		t.Fatalf("the package carries %d source files, the source has %d: %v vs %v",
			len(stripped), len(source), keysOfString(stripped), keysOfString(source))
	}
	for name, want := range source {
		if got, ok := stripped[name]; !ok || got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}

	fsys, err := skillpkg.PackageFS(zipOf(t, stripped))
	if err != nil {
		t.Fatalf("the stripped package could not be opened the way import opens one: %v", err)
	}
	report := skillpkg.Validate(fsys)
	if report.Blocked {
		t.Fatalf("the platform would refuse its own package with its own files removed: %+v", report.Findings)
	}
	if report.Manifest == nil || report.Manifest.Name != "byte-stable-skill" {
		t.Fatalf("the stripped package is not the same skill: %+v", report.Manifest)
	}
}

func keysOfString(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func dumpManifest(t *testing.T, name string, raw []byte) {
	t.Helper()
	dir := os.Getenv("SKILLHUB_MANIFEST_DUMP_DIR")
	if dir == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestTheManifestKeepsTheBoundariesItsContractDraws(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "manifest-reader")
	skillID, versionID := packagedSkill(t, a, pool, c, "documented-skill")

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	raw := zipEntries(t, a, hash)["skillhub-manifest.json"]
	if raw == nil {
		t.Fatal("no manifest in the package")
	}
	dumpManifest(t, "import-origin", raw)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"schema_version", "packaged_at", "packager_version", "profile_id", "profile_version",
		"source", "license", "validation", "compatibility",
		"included_test_cases", "excluded_test_cases", "manifest_hash",
	} {
		if _, ok := m[required]; !ok {
			t.Errorf("manifest has no %q", required)
		}
	}

	validation := m["validation"].(map[string]any)
	if validation["blocked"] != false || len(validation["errors"].([]any)) != 0 {
		t.Errorf("validation = %v", validation)
	}

	lic := m["license"].(map[string]any)
	if (lic["expression"] == nil) != (lic["source_tier"] == nil) {
		t.Errorf("licence expression and tier came apart: %v", lic)
	}
	if lic["expression"] == "NOASSERTION" || lic["expression"] == "NONE" {
		t.Errorf("licence expression carries a status word: %v", lic["expression"])
	}

	compat := m["compatibility"].(map[string]any)
	if compat["capability"] != "unverified" || compat["behaviour"] != "unverified" {
		if _, ok := compat["runtime_image"]; !ok {
			t.Errorf("a measured axis with no runtime image: %v", compat)
		}
	}
	if _, ok := compat["runtime_image"]; ok &&
		compat["capability"] == "unverified" && compat["behaviour"] == "unverified" {
		t.Errorf("an unmeasured version names a runtime image: %v", compat)
	}

	hashValue, _ := m["manifest_hash"].(string)
	if len(hashValue) != 64 {
		t.Errorf("manifest_hash = %q", hashValue)
	}
	if hashValue == hash {
		t.Error("manifest_hash equals content_hash; the two answer different questions and one of them is now unanswerable")
	}

	src := m["source"].(map[string]any)
	origin, ok := src["origin"].(map[string]any)
	if !ok || origin["kind"] == nil {
		t.Fatalf("no origin: %v", src)
	}

	for _, banned := range []string{"problem", "expected_impact", "proposed_content", "excerpt"} {
		if bytes.Contains(raw, []byte(`"`+banned+`"`)) {
			t.Errorf("the manifest carries %q", banned)
		}
	}
}

func TestTheTargetsEndpointServesTheDeploymentsOwnProfiles(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "browser")

	var out struct {
		Targets []struct {
			ID                 string   `json:"id"`
			Kind               string   `json:"kind"`
			Version            string   `json:"version"`
			SupportStatus      string   `json:"support_status"`
			InstallLocation    string   `json:"install_location"`
			VerificationPrompt string   `json:"verification_prompt"`
			VerificationSteps  []string `json:"verification_steps"`
			EnvVars            []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
				Example  string `json:"example"`
			} `json:"env_vars"`
			Notes []string `json:"notes"`
		} `json:"targets"`
	}
	if code := getJSON(t, c.Client, c.base+"/packaging/targets", &out); code != http.StatusOK {
		t.Fatalf("GET /packaging/targets: got %d", code)
	}
	if len(out.Targets) != 3 {
		t.Fatalf("got %d targets, want the standard package plus two profiles (PDM-008)", len(out.Targets))
	}

	if out.Targets[0].ID != "standard" || out.Targets[0].Kind != "standard_package" {
		t.Errorf("first target = %+v", out.Targets[0])
	}
	if out.Targets[0].InstallLocation != "" {
		t.Errorf("the standard package claims an install location: %q", out.Targets[0].InstallLocation)
	}
	for _, target := range out.Targets[1:] {
		if target.Kind != "profile" || target.InstallLocation == "" {
			t.Errorf("profile %s = %+v", target.ID, target)
		}
	}

	for _, target := range out.Targets {
		if target.VerificationPrompt == "" && len(target.VerificationSteps) == 0 {
			t.Errorf("target %s offers no way to check the install worked", target.ID)
		}
	}
	if out.Targets[0].VerificationPrompt != "" {
		t.Errorf("the standard package carries a verification prompt, but it names no agent to run it against: %q",
			out.Targets[0].VerificationPrompt)
	}

	if len(out.Targets[0].EnvVars) != 0 {
		t.Errorf("the standard package names an environment variable: %+v", out.Targets[0].EnvVars)
	}
	var sdk = out.Targets[2]
	if sdk.ID != "claude-agent-sdk" {
		t.Fatalf("third target = %s", sdk.ID)
	}
	if len(sdk.EnvVars) == 0 {
		t.Error("the SDK target declares no environment variable, but installing it needs a key")
	}

	a.packaging.Profiles = packaging.Profiles{}
	if code := getJSON(t, c.Client, c.base+"/packaging/targets", &out); code != http.StatusServiceUnavailable {
		t.Errorf("with no profiles configured, GET /packaging/targets answered %d", code)
	}
}

func TestTheManifestAndTheAPIAgreeOnTheValidation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "one-verdict")
	skillID, versionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: one-verdict-skill\ndescription: Reports on one verdict.\n" +
			"license: MIT\nmetadata:\n  version: 1\n  updated: 2026-01-01\n---\n\n" +
			"Use [the script](scripts/run.py); docs at https://example.com/guide.\n",
		"scripts/run.py":   "import requests\nprint('hello')\n",
		"requirements.txt": "requests==2.31.0\n",
		"reference.md":     "See https://example.org/spec for the details.\n",
	})
	allowRedistribution(t, pool, skillID)

	ctx := context.Background()
	ws, err := gen.New(pool).GetWorkspace(ctx, gen.GetWorkspaceParams{
		ID: mustUUID(t, c.workspaceID), OwnerUserID: mustUUID(t, c.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := a.packaging.Plan(ctx, publishedWorkspace(ws), mustUUID(t, skillID), mustUUID(t, versionID), "standard", false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Allowed {
		t.Fatalf("packaging was refused: %s", plan.BlockedReason)
	}
	var m struct {
		Validation packaging.ManifestValidation `json:"validation"`
	}
	if err := json.Unmarshal(readFromZip(t, plan.Zip, "skillhub-manifest.json"), &m); err != nil {
		t.Fatal(err)
	}
	inManifest, onScreen := findingLines(m.Validation), findingLines(plan.Validation)
	if !reflect.DeepEqual(inManifest, onScreen) {
		t.Errorf("the package says one thing and the API says another:\nmanifest: %v\napi:      %v",
			inManifest, onScreen)
	}
}

func findingLines(v packaging.ManifestValidation) []string {
	out := []string{}
	for _, group := range [][]packaging.ManifestFinding{v.Errors, v.Warnings, v.Infos} {
		for _, f := range group {
			out = append(out, f.Code+"|"+f.Path+"|"+f.Message+"|"+strings.Join(f.Details, ","))
		}
	}
	return out
}

func TestASourcePackagesOwnManifestDoesNotTravel(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "forger")
	forged := `{"license":{"expression":"MIT"},"validation":{"blocked":false}}` + "\n"
	skillID, versionID := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md":               packagingSKILLMD("forged-manifest-skill"),
		"scripts/run.py":         "print('hello')\n",
		"skillhub-manifest.json": forged,
	})
	allowRedistribution(t, pool, skillID)

	code, body := postJSON(t, c, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, body)
	}
	hash, _ := body["content_hash"].(string)
	var produced []byte
	for key, candidate := range a.packages {
		if strings.HasPrefix(key, "downloads/") && strings.HasSuffix(key, "/"+hash+".zip") {
			produced = candidate
			break
		}
	}
	if produced == nil {
		t.Fatalf("no object was stored for content hash %s", hash)
	}

	zr, err := zip.NewReader(bytes.NewReader(produced), int64(len(produced)))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range zr.File {
		if f.Name == "skillhub-manifest.json" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("the package carries %d entries named skillhub-manifest.json, want exactly 1", n)
	}

	got := readFromZip(t, produced, "skillhub-manifest.json")
	if bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace([]byte(forged))) {
		t.Fatal("the author's own file was shipped as the platform's manifest")
	}
	var m struct {
		SchemaVersion string `json:"schema_version"`
		ManifestHash  string `json:"manifest_hash"`
	}
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m.SchemaVersion == "" || m.ManifestHash == "" {
		t.Fatalf("the surviving manifest is not the platform's: %s", got)
	}
}
