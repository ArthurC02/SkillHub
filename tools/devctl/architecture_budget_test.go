package main

import (
	"fmt"
	"strings"
	"testing"
)

type budgetFixture struct {
	t    *testing.T
	root string
}

func newBudgetFixture(t *testing.T, baseline string) budgetFixture {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, architectureBudgetFile, baseline)
	return budgetFixture{t: t, root: root}
}

func (f budgetFixture) files(pkg string, n int) {
	for i := range n {
		writeAt(f.t, f.root, fmt.Sprintf("%s/%s/f%d.go", platformInternalDir, pkg, i), "package p\n")
	}
}

func (f budgetFixture) typeWith(pkg, name string, fields, methods int) {
	var b strings.Builder
	fmt.Fprintf(&b, "package p\n\ntype %s struct {\n", name)
	for i := range fields {
		fmt.Fprintf(&b, "\tF%d int\n", i)
	}
	b.WriteString("}\n")
	for i := range methods {
		fmt.Fprintf(&b, "func (s *%s) M%d() {}\n", name, i)
	}
	writeAt(f.t, f.root, fmt.Sprintf("%s/%s/%s.go", platformInternalDir, pkg, strings.ToLower(name)), b.String())
}

func (f budgetFixture) problems() []string { return architectureBudgetProblems(f.root) }

const emptyBudget = `{"packages": {}, "types": {}}`

func TestAPackageAtTheFileLimitPassesAndOneMoreFileFails(t *testing.T) {
	t.Parallel()
	at := newBudgetFixture(t, emptyBudget)
	at.files("ctx/a", maxPackageFiles)
	if p := at.problems(); len(p) != 0 {
		t.Errorf("%d files: %v, want none", maxPackageFiles, p)
	}
	over := newBudgetFixture(t, emptyBudget)
	over.files("ctx/a", maxPackageFiles+1)
	if p := over.problems(); len(p) != 1 || !strings.Contains(p[0], "ctx/a has 26 non-test files, over the limit of 25") {
		t.Errorf("%d files: %v, want the package named over the limit", maxPackageFiles+1, p)
	}
}

func TestTestFilesAndGeneratedPackagesAreNotCounted(t *testing.T) {
	t.Parallel()
	f := newBudgetFixture(t, emptyBudget)
	f.files("ctx/a", maxPackageFiles)
	writeAt(t, f.root, platformInternalDir+"/ctx/a/extra_test.go", "package p\n")
	f.files("ctx/db/gen", maxPackageFiles+5)
	if p := f.problems(); len(p) != 0 {
		t.Errorf("tests and generated code were counted: %v", p)
	}
}

func TestTheBaselineOnlyLetsAPackageShrink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		files int
		want  string
	}{
		{"held at its baseline", 30, ""},
		{"grew past its baseline", 31, "ctx/a grew to 31 non-test files; " + architectureBudgetFile + " holds it at 30"},
		{"shrank below its baseline", 29, "lower its entry from 30 to 29"},
		{"shrank within the limit", maxPackageFiles, "within the limit of 25; remove its entry"},
	}
	for _, c := range cases {
		f := newBudgetFixture(t, `{"packages": {"ctx/a": 30}, "types": {}}`)
		f.files("ctx/a", c.files)
		p := f.problems()
		switch {
		case c.want == "" && len(p) != 0:
			t.Errorf("%s: %v, want none", c.name, p)
		case c.want != "" && (len(p) != 1 || !strings.Contains(p[0], c.want)):
			t.Errorf("%s: %v, want %q", c.name, p, c.want)
		}
	}
}

func TestABaselineEntryForAPackageThatIsGoneMustBeRemoved(t *testing.T) {
	t.Parallel()
	f := newBudgetFixture(t, `{"packages": {"ctx/gone": 30}, "types": {}}`)
	f.files("ctx/a", 1)
	if p := f.problems(); len(p) != 1 || !strings.Contains(p[0], "ctx/gone no longer exists") {
		t.Errorf("a stale entry: %v, want it named for removal", p)
	}
}

func TestATypeAtTheMethodLimitPassesAndOneMoreMethodFails(t *testing.T) {
	t.Parallel()
	at := newBudgetFixture(t, emptyBudget)
	at.typeWith("ctx/a", "Service", 1, maxTypeMethods)
	if p := at.problems(); len(p) != 0 {
		t.Errorf("%d methods: %v, want none", maxTypeMethods, p)
	}
	over := newBudgetFixture(t, emptyBudget)
	over.typeWith("ctx/a", "Service", 1, maxTypeMethods+1)
	if p := over.problems(); len(p) != 1 || !strings.Contains(p[0], "ctx/a.Service has 31 methods, over the limit of 30") {
		t.Errorf("%d methods: %v, want the type named over the limit", maxTypeMethods+1, p)
	}
}

func TestOnlyABehaviourBearingTypeIsHeldToTheFieldLimit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		fields, methods int
		over            bool
	}{
		{"a value object with many fields", maxBehaviourFields + 1, behaviourMethods, false},
		{"a service at the field limit", maxBehaviourFields, behaviourMethods + 1, false},
		{"a service one field over", maxBehaviourFields + 1, behaviourMethods + 1, true},
	}
	for _, c := range cases {
		f := newBudgetFixture(t, emptyBudget)
		f.typeWith("ctx/a", "Thing", c.fields, c.methods)
		p := f.problems()
		if got := len(p) == 1 && strings.Contains(p[0], "ctx/a.Thing has 13 fields, over the limit of 12"); got != c.over || (!c.over && len(p) != 0) {
			t.Errorf("%s: %v, want over the limit = %v", c.name, p, c.over)
		}
	}
}

func TestTheRepositoryStaysWithinItsArchitectureBudget(t *testing.T) {
	t.Parallel()
	if p := architectureBudgetProblems("../.."); len(p) != 0 {
		t.Errorf("architecture budget: %v", p)
	}
}
