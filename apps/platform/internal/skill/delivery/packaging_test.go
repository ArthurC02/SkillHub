package packaging

// The parts of the packager that need no database: what may travel, how the zip
// is written, what the two hashes cover, and what a profile is allowed to say.
//
// The database-backed half — the four gates, idempotency, and the promise that a
// produced package re-imports — is in
// apps/platform/internal/creator/workspace/packaging_integration_test.go, because it
// runs through the real route table.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestRequireOwnerReadsDoesNotInspectTestLabInternals(t *testing.T) {
	svc := &Service{
		TestLab:            &testlab.Service{},
		AppliedSuggestions: func(context.Context, pgtype.UUID, pgtype.UUID) ([]AppliedSuggestion, error) { return nil, nil },
		SourceLineage:      func(context.Context, pgtype.UUID) (LineageSource, error) { return LineageSource{}, nil },
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{}, false, nil
		},
		ReadVersion: func(context.Context, pgtype.UUID, pgtype.UUID) (VersionFacts, bool, error) {
			return VersionFacts{}, false, nil
		},
		ReadCompatibility: func(context.Context, pgtype.UUID) (RuntimeCompatibility, bool, error) {
			return RuntimeCompatibility{}, false, nil
		},
		ReadPrevious: func(context.Context, pgtype.UUID, pgtype.UUID, int32) (PreviousVersion, bool, error) {
			return PreviousVersion{}, false, nil
		},
		ReadLineage: func(context.Context, pgtype.UUID) (LineageStep, bool, error) { return LineageStep{}, false, nil },
		ReadOldest:  func(context.Context, pgtype.UUID) (OldestVersion, bool, error) { return OldestVersion{}, false, nil },
	}
	if err := svc.requireOwnerReads(); err != nil {
		t.Fatalf("injected Test Lab owner was rejected because of its private configuration: %v", err)
	}
}

// realProfilesDir is the shipped configuration, not a copy of it. Reading the
// real files is what makes these tests notice a profile edit that would change
// produced packages — a fixture copy would keep passing while the deployment
// changed underneath it.
const realProfilesDir = "../../../../../contracts/packaging/profiles"

func loadRealProfiles(t *testing.T) Profiles {
	t.Helper()
	p, err := LoadProfiles(realProfilesDir)
	if err != nil {
		t.Fatalf("LoadProfiles(%s): %v", realProfilesDir, err)
	}
	if len(p) == 0 {
		t.Fatalf("no packaging profiles in %s", realProfilesDir)
	}
	return p
}

// The shipped profile set is what the endpoint serves and what the builder
// applies, so a profile that would make the packager break a rule it cannot see
// has to fail here rather than in a produced package.
func TestTheShippedProfilesLoadAndKeepTheStandardPackageUnmodified(t *testing.T) {
	profiles := loadRealProfiles(t)
	for _, id := range TargetIDs {
		if _, ok := profiles[id]; !ok {
			t.Errorf("packaging target %q has no profile", id)
		}
	}
	std, ok := profiles["standard"]
	if !ok {
		t.Fatal("no standard profile")
	}
	// PDM-008: the standard package is the checkable form of "Skill Hub is not
	// bound to one Agent". Both of these would retire that claim silently.
	if len(std.FrontmatterAdditions) != 0 {
		t.Errorf("the standard package adds frontmatter: %v", std.FrontmatterAdditions)
	}
	if std.Install.TopLevelDir != nil {
		t.Errorf("the standard package wraps the zip in %q; its root must be the package root", *std.Install.TopLevelDir)
	}
	if std.Kind != "standard_package" {
		t.Errorf("standard kind = %q, want standard_package", std.Kind)
	}
}

func TestAProfileMayNotRewriteAFieldTheSpecDefines(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "claude-code.json"), `{
	  "schema_version": "1.0", "id": "claude-code", "kind": "profile", "version": "1.0.0",
	  "display_name": "x", "support_status": "verified",
	  "install": {"locations": [], "top_level_dir": null},
	  "frontmatter_additions": {"description": "a better description"},
	  "env_vars": [], "known_limitations": []
	}`)
	if _, err := LoadProfiles(dir); err == nil {
		t.Fatal("a profile overwriting `description` was accepted; ADR-012 forbids an Adapter rewriting a Skill's intent")
	}
}

func TestAMissingProfileDirectoryIsNoTargetsRatherThanDefaults(t *testing.T) {
	p, err := LoadProfiles(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("a missing directory is a configuration state, not an error: %v", err)
	}
	if len(p) != 0 {
		t.Fatalf("got %d targets out of nowhere; a hard-coded fallback is the second truth the endpoint exists to avoid", len(p))
	}
}

// ADR-027 decision 2: within one packager version, the same content produces the
// same bytes. Without this, content_hash answers "was this the same process
// invocation" instead of "is this the file I had".
func TestWritingTheSameFilesTwiceProducesTheSameBytes(t *testing.T) {
	files := []exportFile{
		{path: "SKILL.md", data: []byte("---\nname: a\ndescription: b\n---\nbody\n")},
		{path: "scripts/run.py", data: []byte("print('hi')\n")},
		{path: "INSTALL.md", data: []byte("# install\n")},
	}
	first, err := writeZip(files, "")
	if err != nil {
		t.Fatal(err)
	}
	// Reversed input order: entry order is decided by path, not by the order the
	// builder happened to collect them in.
	reversed := []exportFile{files[2], files[1], files[0]}
	time.Sleep(time.Millisecond) // any wall-clock leak would show here
	second, err := writeZip(reversed, "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("two builds of the same content produced different bytes")
	}

	zr, err := zip.NewReader(bytes.NewReader(first), int64(len(first)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
		if !f.Modified.Equal(zipEpoch) {
			t.Errorf("%s carries modification time %s, want the zip epoch %s", f.Name, f.Modified, zipEpoch)
		}
		if f.ExternalAttrs != 0 {
			t.Errorf("%s carries external attributes %d; a fixed value is what keeps the bytes stable",
				f.Name, f.ExternalAttrs)
		}
		if len(f.Extra) != 0 {
			t.Errorf("%s carries a zip extra field; canonical archives require none", f.Name)
		}
	}
	want := []string{"INSTALL.md", "SKILL.md", "scripts/run.py"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("entry order %v, want %v", names, want)
	}
}

// ADR-027 decision 1: manifest_hash answers "is the CONTENT the same", so it
// covers no zip metadata and not the manifest itself.
func TestTheManifestHashCoversEveryFileExceptTheManifest(t *testing.T) {
	base := []exportFile{
		{path: "SKILL.md", data: []byte("a")},
		{path: "b/c.txt", data: []byte("b")},
	}
	want, err := manifestHash(base)
	if err != nil {
		t.Fatal(err)
	}
	withManifest := append(append([]exportFile{}, base...),
		exportFile{path: ManifestFile, data: []byte(`{"packaged_at":"whenever"}`)})
	got, err := manifestHash(withManifest)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Error("the manifest changed manifest_hash; a hash that covers packaged_at cannot answer whether two builds have the same content")
	}
	changed, err := manifestHash([]exportFile{base[0], {path: "b/c.txt", data: []byte("B")}})
	if err != nil {
		t.Fatal(err)
	}
	if changed == want {
		t.Error("changing a file's bytes did not change manifest_hash")
	}
}

// The allow-list. Every case here is something a real source package can carry
// and none of it is Skill content; .git in particular can hold a private remote
// URL and a credential cache (PACK-004's "internal paths").
func TestBuildAndVersionControlResidueNeverTravels(t *testing.T) {
	src := fstest.MapFS{
		"SKILL.md":                   &fstest.MapFile{Data: []byte("---\nname: a\ndescription: b\n---\n")},
		".git/config":                &fstest.MapFile{Data: []byte("[remote]\n url = git@private\n")},
		".github/workflows/ci.yml":   &fstest.MapFile{Data: []byte("on: push\n")},
		"node_modules/left-pad/i.js": &fstest.MapFile{Data: []byte("module.exports=1\n")},
		"src/__pycache__/a.pyc":      &fstest.MapFile{Data: []byte{0}},
		".venv/bin/python":           &fstest.MapFile{Data: []byte("#!/bin/sh\n")},
		".env":                       &fstest.MapFile{Data: []byte("API_TOKEN=my-private-test-token\n")},
		"config/.env.local":          &fstest.MapFile{Data: []byte("TOKEN=secret\n")},
		".npmrc":                     &fstest.MapFile{Data: []byte("//registry/:_authToken=secret\n")},
		".aws/credentials":           &fstest.MapFile{Data: []byte("aws_secret_access_key=secret\n")},
		".config/gcloud/application_default_credentials.json": &fstest.MapFile{Data: []byte("{}")},
		".ENV":                           &fstest.MapFile{Data: []byte("TOKEN=secret\n")},
		".NPMRC":                         &fstest.MapFile{Data: []byte("token=secret\n")},
		".git-credentials":               &fstest.MapFile{Data: []byte("https://token@example.test\n")},
		".config/gcloud/credentials.db":  &fstest.MapFile{Data: []byte("secret")},
		".config/gh/hosts.yml":           &fstest.MapFile{Data: []byte("oauth_token: secret\n")},
		".env.example":                   &fstest.MapFile{Data: []byte("API_TOKEN=\n")},
		"fixtures/home/.git-credentials": &fstest.MapFile{Data: []byte("secret")},
		"examples/.config/gcloud/access_tokens.db": &fstest.MapFile{Data: []byte("secret")},
		"examples/.config/gh/hosts.yml":            &fstest.MapFile{Data: []byte("oauth_token: secret\n")},
		"scripts/run.py":                           &fstest.MapFile{Data: []byte("print(1)\n")},
		"link":                                     &fstest.MapFile{Data: []byte("SKILL.md"), Mode: fs.ModeSymlink},
	}
	files, dropped, err := collect(src)
	if err != nil {
		t.Fatal(err)
	}
	// Every removal is now reported, not just performed (04 完整性 review): a
	// package that lost a file and one that never had it used to look the same.
	droppedPaths := map[string]string{}
	for _, e := range dropped {
		if e.Reason == "" {
			t.Errorf("%s was removed with no reason", e.Path)
		}
		droppedPaths[e.Path] = e.Reason
	}
	for path, reason := range map[string]string{
		".git/":                          ReasonExcludedDir,
		"fixtures/home/.git-credentials": ReasonCredentialFile,
		"link":                           ReasonNotRegularFile,
	} {
		if droppedPaths[path] != reason {
			t.Errorf("%s reported as %q, want %q (all removals: %v)", path, droppedPaths[path], reason, droppedPaths)
		}
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.path] = true
	}
	for _, want := range []string{"SKILL.md", "scripts/run.py", ".env.example"} {
		if !got[want] {
			t.Errorf("%s did not travel", want)
		}
	}
	for _, banned := range []string{
		".git/config", ".github/workflows/ci.yml", "node_modules/left-pad/i.js",
		"src/__pycache__/a.pyc", ".venv/bin/python", ".env", "config/.env.local",
		".npmrc", ".aws/credentials", ".config/gcloud/application_default_credentials.json",
		".ENV", ".NPMRC", ".git-credentials", ".config/gcloud/credentials.db", ".config/gh/hosts.yml",
		"fixtures/home/.git-credentials", "examples/.config/gcloud/access_tokens.db", "examples/.config/gh/hosts.yml",
		// A zip can carry a symlink and extraction tools disagree about what to
		// do with one. That is not a decision a package makes on someone's disk.
		"link",
	} {
		if got[banned] {
			t.Errorf("%s travelled", banned)
		}
	}
	// Every surviving path has to be safe to write to a disk the platform does
	// not control. This check exists only on the export side, because the
	// platform never unpacks to disk and the user always does.
	for _, f := range files {
		if !fs.ValidPath(f.path) || strings.Contains(f.path, "..") || strings.ContainsAny(f.path, `\`) {
			t.Errorf("%q is not a path that can be safely extracted", f.path)
		}
	}
}

func TestAProfileAddsFrontmatterFieldsWithoutTouchingTheOnesThatAreThere(t *testing.T) {
	const md = "---\nname: a\ndescription: b\n---\n\nBody stays.\n"
	unchanged, err := addFrontmatter([]byte(md), nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != md {
		t.Fatal("a profile with no additions rewrote SKILL.md; the standard package's whole claim is that it does not")
	}
	out, err := addFrontmatter([]byte(md), map[string]any{"x-skillhub-target": "claude-code"})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "name: a") || !strings.Contains(got, "description: b") ||
		!strings.HasSuffix(got, "\n\nBody stays.\n") {
		t.Errorf("existing fields or body were disturbed:\n%s", got)
	}
	if !strings.Contains(got, `x-skillhub-target: "claude-code"`) {
		t.Errorf("addition missing:\n%s", got)
	}
	if _, err := addFrontmatter([]byte(md), map[string]any{"description": "hijacked"}); err == nil {
		t.Error("a reserved field was overwritten")
	}
	if _, err := addFrontmatter([]byte("no frontmatter here\n"), map[string]any{"a": "b"}); err == nil {
		t.Error("a file with no frontmatter was repaired instead of refused")
	}
}

// manifest_hash only stays equal across two builds of unchanged content if the
// slug does, so this is not cosmetic.
func TestATestCaseSlugIsStableAndSafeAsADirectoryName(t *testing.T) {
	const id = "11111111-2222-4333-8444-555555555555"
	first := testCaseSlug("Monthly Report — from the ledger!", id)
	if first != testCaseSlug("Monthly Report — from the ledger!", id) {
		t.Fatal("the slug is not stable")
	}
	if first == testCaseSlug("Monthly Report — from the ledger!", "99999999-2222-4333-8444-555555555555") {
		t.Error("two test cases with the same name collide")
	}
	for _, name := range []string{"Monthly Report — from the ledger!", "只有中文", "", "a/../b"} {
		slug := testCaseSlug(name, id)
		if !slugRule.MatchString(slug) {
			t.Errorf("slug %q for name %q is not a safe directory name", slug, name)
		}
	}
}

// INSTALL.md is assembled from reviewed configuration, never model-written, and
// PACK-008 puts the support status where it is read before any instruction.
func TestInstallInstructionsStateTheSupportStatusAndAtLeastOneCheck(t *testing.T) {
	profiles := loadRealProfiles(t)
	for _, p := range profiles.Ordered() {
		out := renderInstall(p, "demo-skill", []string{"requirements.txt: package declares external dependencies"})
		// 02:PACK-002 第 2 條 is two clauses, and only the first was ever asserted:
		// the status has to be shown AND it must not read as a promise that the
		// package works. Both wordings carry a disclaimer, so both are named here
		// — deleting either sentence used to leave this suite green.
		switch p.SupportStatus {
		case "unverified":
			if !strings.Contains(out, "Support status: unverified") {
				t.Errorf("%s: an unverified target does not say so", p.ID)
			}
			if !strings.Contains(out, "nothing here promises it will work") {
				t.Errorf("%s: unverified is stated without disclaiming that it works:\n%s", p.ID, out)
			}
		case "verified":
			if !strings.Contains(out, "Support status: verified") {
				t.Errorf("%s: support status missing", p.ID)
			}
			// `verified` is the target, never the Skill. Without this the strongest
			// word in the document would be the one with no boundary on it.
			if !strings.Contains(out, "That is not a promise about this Skill") {
				t.Errorf("%s: verified is stated without disclaiming what it covers:\n%s", p.ID, out)
			}
		default:
			t.Errorf("%s: unknown support status %q", p.ID, p.SupportStatus)
		}
		// PACK-008's other half, and the one a reader's eye enforces: the status
		// comes before any instruction. A document that says the same words at the
		// bottom has told the installer after they installed it.
		status := strings.Index(out, "**Support status:")
		firstSection := strings.Index(out, "\n## ")
		if status < 0 || firstSection < 0 || status > firstSection {
			t.Errorf("%s: the support status is not before the first section (status at %d, "+
				"first section at %d):\n%s", p.ID, status, firstSection, out)
		}
		if !strings.Contains(out, "Check that it worked") {
			t.Errorf("%s: PACK-007 wants a post-install check", p.ID)
		}
		if !strings.Contains(out, "requirements.txt") {
			t.Errorf("%s: declared dependencies are not listed", p.ID)
		}
		if strings.Contains(out, "<name>") {
			t.Errorf("%s: an unsubstituted <name> reached the instructions:\n%s", p.ID, out)
		}
	}
}

// 02:PACK-002 第 1 條's dependency half, and the reason it is not just the
// declared ones: a package that declares nothing while its scripts import pandas
// is the first case where somebody follows the instructions and the Skill still
// does not run. That finding existed; it reached the manifest and the preview and
// never the document the installer reads (04 丙-18).
func TestInstallInstructionsListWhatTheScriptsImportWithoutDeclaring(t *testing.T) {
	report := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityInfo, Code: "package-dependencies", Path: "SKILL.md",
			Message: "package evidences 1 third-party dependency: pandas", Details: []string{"pandas"}},
		{Severity: skillpkg.SeverityWarning, Code: "undeclared-dependency", Path: "SKILL.md",
			Message: "code imports 1 package the package never declares: pandas", Details: []string{"pandas"}},
		// Not a dependency finding; it must not leak into the section.
		{Severity: skillpkg.SeverityWarning, Code: "binary-file", Path: "bin/tool",
			Message: "an executable file"},
	}}
	notes := dependencyNotes(report)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "never declares") {
		t.Errorf("the undeclared dependency is not in the install notes: %v", notes)
	}
	if strings.Contains(joined, "executable file") {
		t.Errorf("a non-dependency finding reached the dependency section: %v", notes)
	}
	out := renderInstall(loadRealProfiles(t)["standard"], "demo-skill", notes)
	if !strings.Contains(out, "never declares") {
		t.Errorf("INSTALL.md does not carry the undeclared dependency:\n%s", out)
	}
	// The heading has to cover both halves. "Dependencies this package declares"
	// would be wrong about the only line that is not obvious from the package.
	if strings.Contains(out, "Dependencies this package declares") {
		t.Error("the dependency heading still claims the list is only what was declared")
	}
}

// The shipped profiles are the source of the env_vars the target list serves, and
// 02:PACK-002 第 1 條 wants them on the page rather than only inside a package the
// user has not built. Asserting the shape here rather than only over HTTP keeps
// the rule with the data: iron rule 11 means an example is a placeholder, never a
// key, and the schema refuses one — this is the floor under a hand-edited file.
func TestTheProfilesDeclareEnvVarsWithoutCredentials(t *testing.T) {
	for _, p := range loadRealProfiles(t).Ordered() {
		for _, v := range p.EnvVars {
			if v.Name == "" || v.Description == "" {
				t.Errorf("%s: env var %+v has nothing to show a user", p.ID, v)
			}
			if credentialShaped(v.Example) {
				t.Errorf("%s: env var %s carries a credential-shaped example", p.ID, v.Name)
			}
		}
	}
}

// credentialShaped assembles the prefixes at run time rather than spelling them
// out, the same discipline tools/qa/skillpkg-corpus/generate.py uses: this
// repository's own pre-push scan greps for those literals, and a test that hard
// codes them makes the scan cry wolf on every future run.
func credentialShaped(s string) bool {
	for _, half := range []string{"proj", "ant"} {
		if strings.Contains(s, "sk-"+half+"-") {
			return true
		}
	}
	return false
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// slugRule is portable-test-case.schema.json's `slug` pattern, verbatim.
var slugRule = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// A closed schema is only closed if the writer honours it, and the branch that
// matters is `improvement`: the suggestion prose quotes a Run's private inputs.
func TestTheImprovementOriginCarriesNoSuggestionProse(t *testing.T) {
	body, err := json.Marshal(improvementOrigin{
		Kind: "improvement", EvaluationID: "e", Suggestions: []suggestionRef{
			{Category: "skill", TargetPath: "SKILL.md"},
		},
		Base: lineageHop{SkillID: "s", SkillVersionID: "v", VersionNumber: 1}, RootSource: unavailable,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"problem", "expected_impact", "proposed_content", "excerpt", "evidence"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("the improvement origin carries %q: %s", banned, body)
		}
	}
}

func TestManifestOriginRefusesWithoutOwnerReaders(t *testing.T) {
	if _, err := (&Service{}).originOf(context.Background(), identity.Workspace{}, SkillFacts{}, VersionFacts{}); err == nil {
		t.Error("manifest origin succeeded without eval and ingest owner readers")
	}
}

// 04 丙-42. Two axes, and the whole point is that they do not collapse into one:
// a superseded package is still servable, and an expired one is still whichever
// version it was.
func TestVersionStateIsSeparateFromServeState(t *testing.T) {
	stale := Artifact{Status: "available", VersionNumber: 2, LatestVersionNumber: 5}.
		withVersionState().withServeState(time.Now().Add(time.Hour), time.Time{})
	if stale.VersionState.Value != "superseded" {
		t.Errorf("version_state = %q, want superseded", stale.VersionState.Value)
	}
	if !stale.Servable || stale.ServeState.Value != "available" {
		t.Errorf("a superseded package is still downloadable: %+v", stale)
	}
	// The numbers are in the words, because 「已被取代」 without saying by what
	// sends the reader to another page to find out how far behind they are.
	if !strings.Contains(stale.VersionState.Label, "v2") ||
		!strings.Contains(stale.VersionState.Label, "v5") {
		t.Errorf("both numbers belong in the label, got %q", stale.VersionState.Label)
	}

	newest := Artifact{Status: "available", VersionNumber: 5, LatestVersionNumber: 5}.
		withVersionState().withServeState(time.Now().Add(-time.Hour), time.Time{})
	if newest.VersionState.Value != "current" {
		t.Errorf("version_state = %q, want current", newest.VersionState.Value)
	}
	if newest.ServeState.Value != "expired" {
		t.Errorf("expiry still decides serving on its own axis: %+v", newest)
	}
}

// 04 丙-91. The two writers of `purged_at` are retention and the reconciler, and
// the row has to keep saying which — including after the expiry that would
// otherwise absorb the answer.
//
// Why this is worth a test rather than a comment: 「已過期」 on a lost package is
// TRUE, so nothing downstream can catch it. The screen reads correctly, the
// owner is told the retention policy did what it says, and the one person who
// could report that the platform lost a file has just been given a reason not
// to.
func TestALostPackageIsNotDescribedAsExpired(t *testing.T) {
	expiry := time.Now().Add(-24 * time.Hour)

	// Purged a week before the deadline: only the reconciler writes that, and the
	// deadline has since passed, which is exactly when this used to flip.
	lost := Artifact{Status: "available"}.
		withServeState(expiry, expiry.Add(-7*24*time.Hour))
	if lost.ServeState.Value != "lost" {
		t.Errorf("serve_state = %q, want lost", lost.ServeState.Value)
	}
	if lost.Servable {
		t.Error("a lost package is not servable")
	}
	// The note has to say the two things the reader acts on: this was not the
	// policy, and the content is still reachable by packaging again.
	for _, want := range []string{"不是保存期到期", "重新打包", "回報"} {
		if !strings.Contains(lost.ServeState.Note, want) {
			t.Errorf("note %q is missing %q", lost.ServeState.Note, want)
		}
	}

	// Purged at the deadline, which is the only time retention purges. Still
	// 「已過期」, because it is.
	retired := Artifact{Status: "available"}.withServeState(expiry, expiry.Add(time.Minute))
	if retired.ServeState.Value != "expired" {
		t.Errorf("serve_state = %q, want expired", retired.ServeState.Value)
	}

	// Lost and not yet expired: the sentence is the same one, which is the point
	// of deriving it from the pair rather than from the absence of an expiry.
	fresh := Artifact{Status: "available"}.
		withServeState(time.Now().Add(24*time.Hour), time.Now())
	if fresh.ServeState.Value != "lost" {
		t.Errorf("serve_state = %q, want lost", fresh.ServeState.Value)
	}

	// A rejected package that is also missing keeps saying rejected: the bytes
	// were never on offer, and losing them changes nothing the reader can act on.
	rejected := Artifact{Status: "rejected"}.
		withServeState(expiry, expiry.Add(-time.Hour))
	if rejected.ServeState.Value != "rejected" {
		t.Errorf("serve_state = %q, want rejected", rejected.ServeState.Value)
	}
}

// The `lost` derivation above reads one column and depends on two statements it
// cannot see, so this is the machine for the sentence in withServeState: the two
// writers of `purged_at` are separated by the retention deadline, and stay
// separated.
//
// Widen either predicate and the derivation goes quietly wrong in the direction
// that hurts — a reconciler allowed to mark expired rows would stamp losses at
// or after the deadline, and every one of them would read as 「已過期」 again,
// which is the defect 丙-91 is. Nothing else would fail: the column is set, the
// row is unservable, and the sentence is grammatical.
//
// Read as text and not as behaviour because behaviour needs a database. That is
// weaker on purpose and stated rather than hidden: it proves the shipped
// statements still say this, not that Postgres agrees.
func TestTheTwoWritersOfPurgedAtStaySeparatedByTheDeadline(t *testing.T) {
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "db", "queries", "reconcile.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, predicate, why string }{
		{"ListArtifactsPastRetention", "expires_at <= now()",
			"retention must only ever purge rows whose deadline has passed; without this a " +
				"retention purge could be stamped before the deadline and read as a loss"},
		{"ListArtifactsClaimingObject", "expires_at > now()",
			"the reconciler must only ever mark rows that are still claiming to be downloadable; " +
				"without this a loss could be stamped after the deadline and read as an expiry"},
	} {
		body, ok := namedQuery(string(sql), tc.query)
		if !ok {
			t.Errorf("-- name: %s is gone from reconcile.sql; the `lost` derivation "+
				"in withServeState has lost half its subject", tc.query)
			continue
		}
		if !strings.Contains(body, tc.predicate) {
			t.Errorf("%s no longer has `%s`: %s", tc.query, tc.predicate, tc.why)
		}
	}
}

// namedQuery returns one sqlc statement's text, from its `-- name:` line to the
// next one.
func namedQuery(sql, name string) (string, bool) {
	start := strings.Index(sql, "-- name: "+name+" ")
	if start < 0 {
		return "", false
	}
	rest := sql[start+1:]
	if next := strings.Index(rest, "-- name: "); next >= 0 {
		return rest[:next], true
	}
	return rest, true
}

// gateFlags must release for `generated`, and the way this breaks is the
// reason it has a test: delete the case and it falls through to `default`,
// which returns BlockedLicenseUnknown. The user is then told "nobody has
// established whether this skill may be redistributed" about a package the
// platform wrote for them thirty seconds ago, and nothing fails except them.
//
// That is exactly the defect ADR-045 was written about, one value later.
func TestGeneratedReleasesThePackagingGate(t *testing.T) {
	reason, message := gateFlags(nil, RedistributionGenerated)
	if reason != "" || message != "" {
		t.Fatalf("generated must release the gate, got reason=%q message=%q", reason, message)
	}
}

// Every redistribution value, with and without a hold, against the one function
// that decides whether bytes leave the platform.
//
// It replaces a test that compared two different string constants for equality —
// an assertion the compiler settles, which can only go red if somebody edits one
// constant to be the other. What was actually untested was three of gateFlags'
// five branches: `self_supplied` releasing, `blocked` and the fail-closed
// `default` refusing. Those were covered only by the integration tests, which
// skip on a machine with no SKILLHUB_TEST_DATABASE_URL — so on such a machine
// replacing the default branch with `return "", ""` turned nothing red at all.
func TestTheDownloadGateAnswersEveryRedistributionValue(t *testing.T) {
	hold := "license-review"
	for _, tc := range []struct {
		name           string
		redistribution string
		wantReason     string
	}{
		{"allowed releases", RedistributionAllowed, ""},
		{"self_supplied releases: retrieval, not redistribution", RedistributionSelfSupplied, ""},
		{"generated releases: no upstream author for a licence to protect", RedistributionGenerated, ""},
		{"blocked refuses", RedistributionBlocked, BlockedNotRedistributable},
		// The two that share the fail-closed default. `unknown` is where every
		// skill starts, so this branch is the commonest refusal in the product;
		// the sixth value is a stand-in for whatever the column grows next, and
		// it must land here rather than release.
		{"unknown refuses", "unknown", BlockedLicenseUnknown},
		{"a value nobody has written yet refuses", "value-added-next-year", BlockedLicenseUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reason, message := gateFlags(nil, tc.redistribution)
			if reason != tc.wantReason {
				t.Errorf("gateFlags(nil, %q) reason = %q, want %q", tc.redistribution, reason, tc.wantReason)
			}
			// A refusal with no sentence is a screen that says 「不能下載」 and
			// nothing else; a release with one is a message nobody will see.
			if (message == "") != (tc.wantReason == "") {
				t.Errorf("gateFlags(nil, %q) = (%q, %q): a reason needs a message and a release needs none",
					tc.redistribution, reason, message)
			}
			// The hold outranks all five, which is the ordering ADR-041 states and
			// the one a new case placed above the access-restriction check breaks.
			if reason, _ := gateFlags(&hold, tc.redistribution); reason != BlockedLicenseHold {
				t.Errorf("a hold over %q gave %q, want %q", tc.redistribution, reason, BlockedLicenseHold)
			}
		})
	}
}

// A hold outranks every redistribution value, generated included. Ordering is
// the rule ADR-041 states and the one that is easy to get wrong when a value
// is added: a new case placed before the access-restriction check would let a
// held skill out.
func TestAccessRestrictionStillOutranksGenerated(t *testing.T) {
	hold := "license-review"
	reason, _ := gateFlags(&hold, RedistributionGenerated)
	if reason != BlockedLicenseHold {
		t.Fatalf("a hold must outrank generated, got %q", reason)
	}
}

// The dataset names that cannot become a direct child of data/. They used to be
// dropped from the package with nothing recording it — the only silent removal
// left in a file whose every other branch exists to say what was left out. Now
// the whole Test Case is excluded with a reason, so this predicate decides
// whether a case travels, which is why it is worth its own test.
func TestADatasetNameThatCannotBeWrittenExcludesTheCase(t *testing.T) {
	for _, name := range []string{
		"", "   ", ".", "..",
		"nested/file.csv", `windows\file.csv`, "C:file.csv",
	} {
		if !unsafeDatasetName([]testlab.Dataset{{FileName: name}}) {
			t.Errorf("%q must be refused as a data/ child", name)
		}
	}
	for _, name := range []string{"rows.csv", "input.json", ".hidden", "a b.txt", "資料.csv"} {
		if unsafeDatasetName([]testlab.Dataset{{FileName: name}}) {
			t.Errorf("%q is a perfectly good file name and must travel", name)
		}
	}
	// One bad name in a set of good ones still stops the case: a case.json that
	// lists fewer datasets than the case has is the quieter lie.
	mixed := []testlab.Dataset{{FileName: "rows.csv"}, {FileName: "../escape.csv"}}
	if !unsafeDatasetName(mixed) {
		t.Error("a single unusable name must exclude the whole case")
	}
	if unsafeDatasetName(nil) {
		t.Error("a case with no datasets has nothing unsafe about it")
	}
}

// The prose a reader has to comprehend is the product's, and this product's
// interface language is Traditional Chinese — `apps/web/index.html` declares
// `lang="zh-Hant"` and says in as many words that every string the app renders
// is Traditional Chinese.
//
// That claim was false here (04 丙-115). These fields are served verbatim to the
// packaging screen — the last screen of the whole journey — and copied verbatim
// into the INSTALL.md inside every download, and all of them were English. No
// test caught it because nothing in this repository read for language: the web
// fixtures faithfully contained the English, so even the doubles were honest.
//
// Both halves are asserted, because either alone permits the wrong thing:
// Chinese without the original silently discards a review of wording that was
// reviewed (`verified` and `unverified` mean exactly what these paragraphs say
// they mean, and nothing else), and the original without Chinese is where this
// started.
//
// Machine-facing strings are deliberately NOT here: `verification_prompt` and
// `snippet` are things a reader pastes into a tool rather than reads, the
// snippet's comments document behaviour bound to one SDK version, and paths and
// scopes are identifiers.
func TestShippedProfilesSpeakTheInterfaceLanguageAndKeepTheReviewedOriginal(t *testing.T) {
	const original = "（原文："

	hasHan := func(s string) bool {
		for _, r := range s {
			if unicode.Is(unicode.Han, r) {
				return true
			}
		}
		return false
	}

	for _, p := range loadRealProfiles(t).Ordered() {
		prose := map[string][]string{
			"verification_steps": p.VerificationSteps,
			"known_limitations":  p.KnownLimitations,
			"notes":              p.Notes,
		}
		for _, v := range p.EnvVars {
			prose["env_vars["+v.Name+"].description"] = []string{v.Description}
		}
		for _, loc := range p.Install.Locations {
			prose["install.locations["+loc.Scope+"].description"] = []string{loc.Description}
		}

		for field, items := range prose {
			for i, s := range items {
				if !hasHan(s) {
					t.Errorf("%s %s[%d] has no Chinese, so the packaging screen and the\n"+
						"downloaded INSTALL.md show a reader English on a zh-Hant page:\n%s", p.ID, field, i, s)
				}
				if !strings.Contains(s, original) {
					t.Errorf("%s %s[%d] dropped the reviewed original. The English wording is what\n"+
						"was reviewed; a translation that replaces it deletes that review:\n%s", p.ID, field, i, s)
				}
			}
		}
	}
}
