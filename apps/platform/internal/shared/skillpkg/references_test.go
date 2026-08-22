package skillpkg

import (
	"testing"
	"testing/fstest"
)

// The reference check used to read markdown links only, which verifies the
// Skills written as documents and silently skips the ones written as
// instructions — and the second kind is the kind with scripts.
func TestABarePathIsAReferenceWhenThePackageHasThatDirectory(t *testing.T) {
	src := fstest.MapFS{
		"SKILL.md": &fstest.MapFile{Data: []byte("---\nname: s\ndescription: d\n---\n" +
			"Run scripts/analyze.py first, then see references/passes.md.\n" +
			"Unrelated prose about requirements.txt in some other repo.\n")},
		"scripts/analyze.py":   &fstest.MapFile{Data: []byte("print(1)\n")},
		"references/passes.md": &fstest.MapFile{Data: []byte("# passes\n")},
	}
	got := map[string]bool{}
	for _, ref := range SkillMDReferences(src) {
		got[ref] = true
	}
	for _, want := range []string{"scripts/analyze.py", "references/passes.md"} {
		if !got[want] {
			t.Errorf("%s was not read as a reference: %v", want, got)
		}
	}
	// The anchor. Without it every mention of another project's file becomes a
	// missing file in this one, and a checker that cries wolf is turned off.
	if got["requirements.txt"] {
		t.Error("a path with no matching directory in the package was read as a reference")
	}
}

func TestABareReferenceToAMissingFileIsReportedAndStillShips(t *testing.T) {
	src := fstest.MapFS{
		"SKILL.md": &fstest.MapFile{Data: []byte("---\nname: s\ndescription: d\n---\n" +
			"Run scripts/missing.py.\n")},
		"scripts/present.py": &fstest.MapFile{Data: []byte("print(1)\n")},
	}
	r := Validate(src)
	found := false
	for _, f := range r.Findings {
		if f.Code == "file-ref-missing" && f.Path == "scripts/missing.py" {
			found = true
			if f.Severity != SeverityWarning {
				t.Errorf("severity = %v, want warning: a package that arrived incomplete is the author's, "+
					"and 02:SKILL-002 asks for the severities to be shown apart, not for this one to block", f.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("a bare reference to a missing file was not reported: %+v", r.Findings)
	}
	if r.Blocked {
		t.Error("a dangling reference blocked the package; only the packager removing a needed file does that")
	}
}
