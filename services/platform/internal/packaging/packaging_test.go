package packaging

// The parts of the packager that need no database: what may travel, how the zip
// is written, what the two hashes cover, and what a profile is allowed to say.
//
// The database-backed half — the four gates, idempotency, and the promise that a
// produced package re-imports — is in
// services/platform/internal/identity/packaging_integration_test.go, because it
// runs through the real route table.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ArthurC02/skillhub/services/platform/internal/skillpkg"
)

// realProfilesDir is the shipped configuration, not a copy of it. Reading the
// real files is what makes these tests notice a profile edit that would change
// produced packages — a fixture copy would keep passing while the deployment
// changed underneath it.
const realProfilesDir = "../../../../contracts/packaging/profiles"

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
	files, err := collect(src)
	if err != nil {
		t.Fatal(err)
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
		if p.SupportStatus == "unverified" && !strings.Contains(out, "Support status: unverified") {
			t.Errorf("%s: an unverified target does not say so", p.ID)
		}
		if p.SupportStatus == "verified" && !strings.Contains(out, "Support status: verified") {
			t.Errorf("%s: support status missing", p.ID)
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
