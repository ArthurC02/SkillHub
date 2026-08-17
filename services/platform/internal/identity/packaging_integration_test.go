// PACK-001/002/003/004/005 through the real route table: what the packager
// refuses, what it produces, and the one promise the whole milestone rests on —
// that a package Skill Hub hands out is a package Skill Hub would take back.
//
// The assertions are on the produced BYTES rather than on intent, which is the
// precedent the cost-estimate test set: a packaging bug that leaks something is
// not reversible once a user has the file, so "the exporter meant to exclude it"
// is not evidence.
package identity_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/services/platform/internal/ingest"
	"github.com/ArthurC02/skillhub/services/platform/internal/packaging"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// --- fixtures ----------------------------------------------------------------

func packagingSKILLMD(name string) string {
	return "---\nname: " + name + "\ndescription: Reports on " + name +
		".\nlicense: MIT\n---\n\nUse it like this: [the script](scripts/run.py).\n"
}

// importFiles puts one package through the real import pipeline and returns the
// skill and version it produced. Real, because everything below reads package
// bytes: a fabricated package_object_key would make every check answer
// "unreadable" and the tests would pass for the wrong reason.
func importFiles(
	t *testing.T, a *api, pool *pgxpool.Pool, owner *client, files map[string]string,
) (skillID, versionID string) {
	t.Helper()
	ctx := context.Background()
	ws, err := gen.New(pool).GetWorkspace(ctx, gen.GetWorkspaceParams{
		ID: mustUUID(t, owner.workspaceID), OwnerUserID: mustUUID(t, owner.userID),
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := (&ingest.Service{Pool: pool, Store: a.packages}).UploadZip(ctx, ws, zipOf(t, files))
	if err != nil {
		t.Fatal(err)
	}
	if res.Report.Blocked {
		t.Fatalf("fixture package did not import: %+v", res.Report.Findings)
	}
	return uuidText(res.Skill.ID), uuidText(res.Version.ID)
}

// allowRedistribution is what a curator's verdict looks like in the database.
// Every skill starts at 'unknown', which blocks, so a packaging test that did
// not do this would be testing the gate and nothing else.
func allowRedistribution(t *testing.T, pool *pgxpool.Pool, skillID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		"UPDATE skills SET redistribution = 'allowed' WHERE id = $1", mustUUID(t, skillID)); err != nil {
		t.Fatal(err)
	}
}

// packagedSkill is the common arrangement: an imported package that is allowed
// to be handed out.
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

// zipEntries reads a produced package out of the object store by its content
// hash, which is also a check that the hash names the object that was written.
func zipEntries(t *testing.T, a *api, contentHash string) map[string][]byte {
	t.Helper()
	data, ok := a.packages["downloads/"+contentHash+".zip"]
	if !ok {
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

// --- PACK-009: the invariant the milestone rests on --------------------------

// A package the platform hands out has to be a package the platform would take
// back. If export and import are not two directions of one rule, Skill Hub can
// produce packages it cannot itself accept — and "可被重新驗證" would have no
// definition (m4/README §6.1).
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
			produced := a.packages["downloads/"+hash+".zip"]

			// The invariant, stated exactly as packaging-design §2.3 states it.
			fsys, err := ingest.PackageFS(produced)
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

			// Every entry has to be safe to extract onto a disk the platform does
			// not control. This check has no counterpart on the import side,
			// because the platform never unpacks and the user always does.
			entries := zipEntries(t, a, hash)
			for name := range entries {
				if !fs.ValidPath(name) || strings.Contains(name, "..") || strings.ContainsAny(name, `\`) {
					t.Errorf("entry %q would escape on extraction", name)
				}
			}

			// The platform adds three things and no more.
			if _, ok := entries[entryPath(t, target, "skillhub-manifest.json")]; !ok {
				t.Error("no manifest")
			}
			if _, ok := entries[entryPath(t, target, "INSTALL.md")]; !ok {
				t.Error("no install instructions")
			}

			// PDM-008: the standard package's SKILL.md is the source version's,
			// byte for byte. That equality IS the claim that Skill Hub is not
			// bound to one Agent, so it is asserted on bytes.
			if target == "standard" {
				want := readFromZip(t, source, "SKILL.md")
				if got := entries["SKILL.md"]; !bytes.Equal(got, want) {
					t.Errorf("the standard package rewrote SKILL.md:\ngot  %q\nwant %q", got, want)
				}
			}
		})
	}
}

// entryPath is where a platform-added file lands for one target: at the zip root
// for the standard package, inside the profile's single top-level directory for
// the two profiles.
func entryPath(t *testing.T, target, name string) string {
	t.Helper()
	if target == "standard" {
		return name
	}
	return "round-trip-skill/" + name
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

// --- ADR-027 decision 2: reproducibility -------------------------------------

// Built twice, byte for byte the same. Without this, content_hash answers "was
// this the same build" rather than "is this the file I had", and the whole
// deduplication story goes with it.
//
// Through the service rather than the endpoint on purpose: the endpoint is
// idempotent, so a second POST would answer with the first call's artifact and
// prove nothing about the builder.
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
	first, err := a.packaging.Plan(ctx, ws, mustUUID(t, skillID), mustUUID(t, versionID), "claude-code", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.packaging.Plan(ctx, ws, mustUUID(t, skillID), mustUUID(t, versionID), "claude-code", false)
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

// --- the four gates ----------------------------------------------------------

// Four reasons and not one `blocked: true`, because whether the user can do
// anything about it depends entirely on which one it was. Three of them fail
// closed: an unrecognised hold code still holds, and an unestablished licence
// still blocks (SEC-011, 02:DISC-003).
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
			// The read side of 0023 treats an unknown code as restricted, and so
			// does this: a code nobody recognises must never be the way content
			// unlocks.
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
			// Where every skill starts, and where anything unclassifiable stays.
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

			// The preview runs the checks the create call runs, so it can never
			// promise something the create call then refuses.
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

// The fourth gate. A package with an error-level finding must not be presented
// as a valid package, and the verdict comes from skillpkg — the packager does
// not define a second notion of "blocking".
func TestASecretBearingPackageCannotBeBuiltAndBuildResidueNeverTravels(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "leaker")

	// A credential in a .env-shaped file. Import would have refused this package,
	// so it is seeded straight into the store under a version row — which is also
	// the honest shape of the risk: bytes already in storage, being handed out.
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
	if n := downloadArtifactsFor(t, pool, versionID); n != 0 {
		t.Errorf("a refused build left %d artifacts behind", n)
	}

	// The other half of PACK-004: repository residue is dropped by the allow-list
	// before validation ever sees it, so a package carrying a private remote URL
	// and a token in .git/config still builds — without them.
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

// seedStoredVersion writes package bytes to the store and a version row pointing
// at them, bypassing import. Only for fixtures import would refuse: everything
// else goes through the real pipeline.
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

// downloadArtifactsFor counts this version's packages only. The tests share one
// database, so a table-wide count would answer for the whole suite.
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

// --- idempotency -------------------------------------------------------------

// Packaging is idempotent, which is what lets the download page truthfully say
// "the same version can be packaged again at any time" and what removes the need
// for a re-package endpoint (ADR-027 decision 1).
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

	// A different test-case choice is a different package, not the same one
	// quietly changed: include_test_cases is part of the identity.
	code, other := postJSON(t, c, packagingPath(skillID, versionID),
		`{"target":"standard","include_test_cases":true}`)
	if code != http.StatusCreated {
		t.Fatalf("third POST: got %d, body %v", code, other)
	}
	if other["duplicate"] != false || other["artifact_id"] == first["artifact_id"] {
		t.Errorf("packaging with test cases reused the artifact built without them: %v", other)
	}
}

// --- iron rule 3, ADR-027 decision 4 -----------------------------------------

// A fork is another copy of the same licensed material, so the verdict about
// handing it out travels with it. Without this, forking is the way around the
// gate — the same reason the 0023 hold is copied.
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

	// And the fork packages, which is the only way to exercise the lineage walk
	// on real rows. DISC-003 clause 5 asks for the ORIGINAL source, so one hop is
	// not an answer — the walk crosses into the upstream workspace on purpose.
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

// --- PACK-005 ----------------------------------------------------------------

// Only curated content travels, and what does not is listed with its reason
// rather than dropped — an empty list would read as "this Skill has no tests".
func TestOnlyCuratedTestCasesTravelAndTheRestAreNamed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	user := a.login(t, "test-case-owner")
	skillID, versionID := packagedSkill(t, a, pool, user, "tested-skill")
	seedTestCase(t, pool, user.workspaceID, skillID)

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
	if len(excluded) != 1 {
		t.Fatalf("the excluded test case was not reported: %v", preview)
	}
	if reason := excluded[0].(map[string]any)["reason"]; reason != "not_curated" {
		t.Errorf("exclusion reason = %v, want not_curated", reason)
	}

	// The same Test Case in a catalog workspace is curated, and then it travels.
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
	// The enum has one value, and that is the point: widening it is a contract
	// change, not a flag an exporter can flip.
	if portable["origin"] != "curated" {
		t.Errorf("origin = %v, want curated", portable["origin"])
	}
	for _, banned := range []string{"run_id", "evaluation", "trace", "verdict"} {
		if _, present := portable[banned]; present {
			t.Errorf("the portable test case carries %q, which is Run data", banned)
		}
	}
}

// dumpManifest writes one produced manifest out for external schema validation.
// A no-op unless the directory is configured, so it costs an ordinary test run
// nothing.
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

// --- the manifest ------------------------------------------------------------

// The manifest is a contract with a consumer outside this repository, so the
// rules its schema calls out are asserted on a real produced manifest rather
// than trusted to the writer.
//
// Set SKILLHUB_MANIFEST_DUMP_DIR and the packaging tests write the manifests
// they produced there, which is how the JSON Schema itself gets run over real
// output: jsonschema is pinned in CI's contracts job and is not reachable from a
// Go test, so the two halves are run separately rather than approximated here.
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
	// A blocked package is refused and never written, so a manifest asserting
	// otherwise describes a file that must not exist.
	validation := m["validation"].(map[string]any)
	if validation["blocked"] != false || len(validation["errors"].([]any)) != 0 {
		t.Errorf("validation = %v", validation)
	}
	// ADR-021 decision 1: the pair is all or nothing, and the SPDX status words
	// are not values this field carries.
	lic := m["license"].(map[string]any)
	if (lic["expression"] == nil) != (lic["source_tier"] == nil) {
		t.Errorf("licence expression and tier came apart: %v", lic)
	}
	if lic["expression"] == "NOASSERTION" || lic["expression"] == "NONE" {
		t.Errorf("licence expression carries a status word: %v", lic["expression"])
	}
	// 04 乙-4: a verdict without the image it was measured on is an extrapolation.
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
	// The hash covers every other file and not this one, so a consumer can
	// recompute it. Recomputing it here would be re-implementing the exporter;
	// what is checked is that it is a hash and that it is not the zip's.
	hashValue, _ := m["manifest_hash"].(string)
	if len(hashValue) != 64 {
		t.Errorf("manifest_hash = %q", hashValue)
	}
	if hashValue == hash {
		t.Error("manifest_hash equals content_hash; the two answer different questions and one of them is now unanswerable")
	}
	// PACK-003: the source it came from, with the origin actually filled in.
	src := m["source"].(map[string]any)
	origin, ok := src["origin"].(map[string]any)
	if !ok || origin["kind"] == nil {
		t.Fatalf("no origin: %v", src)
	}
	// Iron rule 11: no Run data has a field here, and none appears.
	for _, banned := range []string{"problem", "expected_impact", "proposed_content", "excerpt"} {
		if bytes.Contains(raw, []byte(`"`+banned+`"`)) {
			t.Errorf("the manifest carries %q", banned)
		}
	}
}

// --- PACK-002 ----------------------------------------------------------------

// An endpoint and not a frontend constant, because support status changes when a
// target is measured (contract-deltas §1.1).
func TestTheTargetsEndpointServesTheDeploymentsOwnProfiles(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "browser")

	var out struct {
		Targets []struct {
			ID              string   `json:"id"`
			Kind            string   `json:"kind"`
			Version         string   `json:"version"`
			SupportStatus   string   `json:"support_status"`
			InstallLocation string   `json:"install_location"`
			Notes           []string `json:"notes"`
		} `json:"targets"`
	}
	if code := getJSON(t, c.Client, c.base+"/packaging/targets", &out); code != http.StatusOK {
		t.Fatalf("GET /packaging/targets: got %d", code)
	}
	if len(out.Targets) != 3 {
		t.Fatalf("got %d targets, want the standard package plus two profiles (PDM-008)", len(out.Targets))
	}
	// The standard package comes first and is distinguishable from a profile:
	// PDM-008 requires the product to keep saying "one standard package plus two
	// install profiles", and one flat list cannot.
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

	// A deployment with no configuration says so, rather than serving an invented
	// list. 503 and not 200 with an empty array: "cannot package" and "nothing to
	// package" are different answers.
	a.packaging.Profiles = packaging.Profiles{}
	if code := getJSON(t, c.Client, c.base+"/packaging/targets", &out); code != http.StatusServiceUnavailable {
		t.Errorf("with no profiles configured, GET /packaging/targets answered %d", code)
	}
}
