package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func namedSkillMD(name string) string {
	return "---\nname: " + name + "\ndescription: Work with things and more things.\nlicense: MIT\n---\n# " + name + "\n"
}

func pluginManifest(name string) string {
	return `{"$schema":"` + skillpkg.PluginSchemaID + `","name":"` + name + `","version":"2.1.0",` +
		`"repository":"https://example.invalid/r"}`
}

func planOf(t *testing.T, files map[string]string) importPlan {
	t.Helper()
	plan, err := planImport(zipBytes(t, files))
	if err != nil {
		t.Fatalf("planImport: %v", err)
	}
	return plan
}

func admittedPaths(plan importPlan) []string {
	out := make([]string, 0, len(plan.admitted))
	for _, s := range plan.admitted {
		out = append(out, s.path)
	}
	return out
}

func TestPlanImportKeepsThePackageDigestForASkillThatIsThePackage(t *testing.T) {
	data := zipBytes(t, map[string]string{"SKILL.md": skillMD})
	sum := sha256.Sum256(data)

	plan, err := planImport(data)
	if err != nil {
		t.Fatal(err)
	}

	if plan.shape != skillpkg.ShapeSkill || len(plan.admitted) != 1 {
		t.Fatalf("shape = %q, admitted = %v", plan.shape, admittedPaths(plan))
	}
	if got := plan.admitted[0].pkg.contentHash; got != hex.EncodeToString(sum[:]) {
		t.Fatalf("content hash = %q, want the package digest %q; changing it would make every "+
			"already-imported package look like new content", got, hex.EncodeToString(sum[:]))
	}
}

func TestPlanImportHashesEachSkillOfAPluginOverItsOwnFiles(t *testing.T) {
	data := zipBytes(t, map[string]string{
		"plugin.json":           pluginManifest("my-plugin"),
		"skills/alpha/SKILL.md": namedSkillMD("alpha"),
		"skills/beta/SKILL.md":  namedSkillMD("beta"),
	})
	sum := sha256.Sum256(data)
	packageHash := hex.EncodeToString(sum[:])

	plan, err := planImport(data)
	if err != nil {
		t.Fatal(err)
	}

	if plan.shape != skillpkg.ShapePlugin {
		t.Fatalf("shape = %q, want %q", plan.shape, skillpkg.ShapePlugin)
	}
	if got := strings.Join(admittedPaths(plan), ","); got != "skills/alpha,skills/beta" {
		t.Fatalf("admitted = %q", got)
	}
	if plan.plugin == nil || plan.plugin.Name != "my-plugin" || plan.plugin.Version != "2.1.0" {
		t.Fatalf("plugin facts = %+v", plan.plugin)
	}
	seen := map[string]bool{}
	for _, s := range plan.admitted {
		if s.pkg.objectKey != plan.objectKey {
			t.Errorf("%s points at %q, want the one shared object %q", s.path, s.pkg.objectKey, plan.objectKey)
		}
		if s.pkg.contentHash == packageHash {
			t.Errorf("%s was hashed over the whole plugin, so two skills of one plugin would collide", s.path)
		}
		if seen[s.pkg.contentHash] {
			t.Errorf("%s repeats a content hash already used in this plan", s.path)
		}
		seen[s.pkg.contentHash] = true
	}
}

func TestPlanImportGivesTheSameContentTheSameHashWhereverItSits(t *testing.T) {
	body := namedSkillMD("alpha")
	first := planOf(t, map[string]string{
		"plugin.json": pluginManifest("one"), "skills/alpha/SKILL.md": body})
	second := planOf(t, map[string]string{
		"plugin.json": pluginManifest("two"), "skills/alpha/SKILL.md": body, "README.md": "# unrelated"})

	if first.admitted[0].pkg.contentHash != second.admitted[0].pkg.contentHash {
		t.Fatalf("identical skill content hashed differently (%s vs %s); duplicate detection would miss it",
			first.admitted[0].pkg.contentHash, second.admitted[0].pkg.contentHash)
	}
}

func TestPlanImportFindsSkillsByWalkingASourceWithNoManifest(t *testing.T) {
	plan := planOf(t, map[string]string{
		"README.md":                      "# repo",
		"packages/one/SKILL.md":          namedSkillMD("one"),
		"vendor-dir/skills/two/SKILL.md": namedSkillMD("two"),
	})

	if plan.shape != skillpkg.ShapeTree {
		t.Fatalf("shape = %q, want %q", plan.shape, skillpkg.ShapeTree)
	}
	if got := strings.Join(admittedPaths(plan), ","); got != "packages/one,vendor-dir/skills/two" {
		t.Fatalf("admitted = %q", got)
	}
}

func TestPlanImportStopsAtTheCeilingAndSaysBothNumbers(t *testing.T) {
	build := func(count int) map[string]string {
		files := map[string]string{"plugin.json": pluginManifest("big")}
		for i := 0; i < count; i++ {
			name := fmt.Sprintf("skill-%03d", i)
			files["skills/"+name+"/SKILL.md"] = namedSkillMD(name)
		}
		return files
	}

	if plan := planOf(t, build(MaxSkillsPerImport)); len(plan.admitted) != MaxSkillsPerImport {
		t.Fatalf("exactly the ceiling was refused: %d admitted", len(plan.admitted))
	}

	_, err := planImport(zipBytes(t, build(MaxSkillsPerImport+1)))
	if !errors.Is(err, ErrTooManySkills) {
		t.Fatalf("one over the ceiling: err = %v, want ErrTooManySkills", err)
	}
	for _, want := range []string{fmt.Sprint(MaxSkillsPerImport + 1), fmt.Sprint(MaxSkillsPerImport)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not say %q, so the reader cannot tell how far over they are: %v", want, err)
		}
	}
}

func TestPlanImportAdmitsTheGoodSkillsAndListsTheRefusedOnes(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":            pluginManifest("mixed"),
		"skills/alpha/SKILL.md":  namedSkillMD("alpha"),
		"skills/broken/SKILL.md": "no frontmatter here at all\n",
	})

	if got := strings.Join(admittedPaths(plan), ","); got != "skills/alpha" {
		t.Fatalf("admitted = %q, want only the valid skill", got)
	}
	if len(plan.refused) != 1 || plan.refused[0].path != "skills/broken" {
		t.Fatalf("refused = %+v, want the one broken skill named", plan.refused)
	}
	if plan.blocked() {
		t.Error("one broken skill blocked the whole import; the others were valid")
	}
}

func TestPlanImportIsBlockedOnlyWhenNothingSurvives(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":            pluginManifest("all-bad"),
		"skills/broken/SKILL.md": "no frontmatter here at all\n",
	})
	if !plan.blocked() || len(plan.refused) != 1 {
		t.Fatalf("a plugin whose every skill failed was not blocked: %+v", plan)
	}

	empty := planOf(t, map[string]string{"README.md": "# nothing"})
	if !empty.blocked() || len(empty.refused) != 1 || !empty.refused[0].pkg.report.Blocked {
		t.Fatalf("a source with no SKILL.md was not blocked: %+v", empty)
	}
}

func TestPlanImportCarriesSourceLevelFindingsOntoEverySkillBelowTheRoot(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":             pluginManifest("nested"),
		"skills/alpha/SKILL.md":   namedSkillMD("alpha"),
		"skills/alpha/bundle.zip": "PK\x03\x04 not really",
	})

	if len(plan.admitted) != 1 {
		t.Fatalf("admitted = %v", admittedPaths(plan))
	}
	for _, f := range plan.admitted[0].pkg.report.Findings {
		if f.Code == skillpkg.CodeNestedArchive {
			return
		}
	}
	t.Fatalf("the archive reader's finding never reached the skill's report: %+v",
		plan.admitted[0].pkg.report.Findings)
}

func TestPlanImportRefusesASecondSkillClaimingANameAlreadyTaken(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":           pluginManifest("twins"),
		"skills/alpha/SKILL.md": namedSkillMD("alpha"),
		"skills/beta/SKILL.md":  namedSkillMD("alpha"),
	})

	if got := strings.Join(admittedPaths(plan), ","); got != "skills/alpha" {
		t.Fatalf("admitted = %q; a repeated name would become the first skill's version 2", got)
	}
	if plan.blocked() {
		t.Error("the whole import was blocked; the first skill was fine")
	}
	if len(plan.refused) != 1 || plan.refused[0].path != "skills/beta" {
		t.Fatalf("refused = %+v, want only skills/beta", plan.refused)
	}
	report := plan.refused[0].pkg.report
	if !report.Blocked {
		t.Error("the refused skill's report is not blocked, so it would still be imported")
	}
	var found *skillpkg.Finding
	for i, f := range report.Findings {
		if f.Code == skillpkg.CodeDuplicateSkillName {
			found = &report.Findings[i]
		}
	}
	if found == nil {
		t.Fatalf("no %s finding: %+v", skillpkg.CodeDuplicateSkillName, report.Findings)
	}
	for _, want := range []string{"skills/alpha", "alpha"} {
		if !strings.Contains(found.Message, want) {
			t.Errorf("the refusal does not say %q, so the reader cannot find the other one: %s", want, found.Message)
		}
	}
}

func TestPlanImportAdmitsEveryDistinctNameInOneSource(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":           pluginManifest("triplets"),
		"skills/alpha/SKILL.md": namedSkillMD("alpha"),
		"skills/beta/SKILL.md":  namedSkillMD("beta"),
		"skills/gamma/SKILL.md": namedSkillMD("gamma"),
	})

	if got := strings.Join(admittedPaths(plan), ","); got != "skills/alpha,skills/beta,skills/gamma" {
		t.Fatalf("admitted = %q, want all three", got)
	}
	if len(plan.refused) != 0 {
		t.Fatalf("refused = %+v, want none", plan.refused)
	}
}

func TestEachSkillOfAPluginRecordsTheDirectoryItWasValidatedFrom(t *testing.T) {
	plan := planOf(t, map[string]string{
		"plugin.json":            pluginManifest("routed"),
		"skills/first/SKILL.md":  namedSkillMD("first"),
		"skills/second/SKILL.md": namedSkillMD("second"),
	})

	got := map[string]string{}
	for _, planned := range plan.admitted {
		got[planned.path] = planned.pkg.sourcePath
	}
	want := map[string]string{"skills/first": "skills/first", "skills/second": "skills/second"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source paths = %v, want %v; a wrong path sends every later reader at the wrong directory", got, want)
	}
}

func TestASkillThatIsTheWholePackageRecordsNoSubdirectory(t *testing.T) {
	plan := planOf(t, map[string]string{"SKILL.md": skillMD})

	if len(plan.admitted) != 1 {
		t.Fatalf("admitted = %v", admittedPaths(plan))
	}
	if got := plan.admitted[0].pkg.sourcePath; got != "" {
		t.Fatalf("source path = %q, want empty; the package root is already this skill's root", got)
	}
}
