package run

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func reasonOf(t *testing.T, err error) string {
	t.Helper()
	var r refusal
	if !errors.As(err, &r) {
		t.Fatalf("%v is not a refusal, so nothing counts it or records why", err)
	}
	return r.reason
}

func blockedReport(codes ...string) skillpkg.Report {
	report := skillpkg.Report{Blocked: true}
	for _, code := range codes {
		report.Findings = append(report.Findings,
			skillpkg.Finding{Severity: skillpkg.SeverityError, Code: code})
	}
	return report
}

func TestAPackageThatCouldNotBeScannedIsNotTreatedAsACleanOne(t *testing.T) {
	reason, err := scanVerdict(skillpkg.Report{}, false)
	if reason != ReasonScanUnavailable {
		t.Errorf("reason = %q, want %q", reason, ReasonScanUnavailable)
	}
	if !errors.Is(err, ErrScanBlocked) {
		t.Errorf("an unscanned package must refuse through the scan sentinel: %v", err)
	}
}

func TestAScannedPackageWithNothingBlockingRuns(t *testing.T) {
	clean := skillpkg.Report{Findings: []skillpkg.Finding{
		{Severity: skillpkg.SeverityWarning, Code: "PKG-W01"},
	}}
	if _, err := scanVerdict(clean, true); err != nil {
		t.Errorf("a scanned package the scanner did not block was refused: %v", err)
	}
}

func TestABlockedPackageIsRefusedAndNamesWhatBlockedIt(t *testing.T) {
	reason, err := scanVerdict(blockedReport("PKG-E02", "PKG-E01"), true)
	if reason != ReasonScanBlocked {
		t.Errorf("reason = %q, want %q", reason, ReasonScanBlocked)
	}
	if !strings.Contains(err.Error(), "PKG-E01, PKG-E02") {
		t.Errorf("the refusal does not name the blocking codes in a stable order: %v", err)
	}
}

func TestABlockingCodeIsNamedOnceHoweverManyFilesCarryIt(t *testing.T) {
	report := blockedReport("PKG-E01", "PKG-E01", "PKG-E01")
	report.Findings = append(report.Findings,
		skillpkg.Finding{Severity: skillpkg.SeverityWarning, Code: "PKG-W09"})

	_, err := scanVerdict(report, true)
	if got := strings.Count(err.Error(), "PKG-E01"); got != 1 {
		t.Errorf("PKG-E01 is named %d times, want 1: %v", got, err)
	}
	if strings.Contains(err.Error(), "PKG-W09") {
		t.Errorf("a warning was reported as a reason the run is blocked: %v", err)
	}
}

func TestAWorkspaceLeavesOneNodeSlotForAnotherWorkspace(t *testing.T) {
	if MaxConcurrentRunsPerWorkspace != 1 {
		t.Fatalf("per-workspace concurrency = %d, want 1", MaxConcurrentRunsPerWorkspace)
	}
	if err := runSlotVerdict(0); err != nil {
		t.Fatalf("an idle workspace was refused: %v", err)
	}
	err := runSlotVerdict(1)
	if got := reasonOf(t, err); got != ReasonWorkspaceConcurrency {
		t.Errorf("one run in progress: reason = %q, want %q", got, ReasonWorkspaceConcurrency)
	}
	if !strings.Contains(err.Error(), strconv.FormatInt(1, 10)) {
		t.Errorf("the refusal does not say how many are already running: %v", err)
	}
}

func TestAnAccessRestrictedSkillIsRefusedAndSaysWhichRestriction(t *testing.T) {
	hold := "a licence review is open"
	err := (&Service{}).requireNotAccessRestricted(SkillFacts{AccessRestricted: true, AccessRestrictionReason: hold})
	if got := reasonOf(t, err); got != ReasonAccessRestricted {
		t.Errorf("reason = %q, want %q", got, ReasonAccessRestricted)
	}
	if !strings.Contains(err.Error(), hold) {
		t.Errorf("the refusal does not name the restriction: %v", err)
	}
	if err := (&Service{}).requireNotAccessRestricted(SkillFacts{}); err != nil {
		t.Errorf("a skill under no restriction was refused: %v", err)
	}
}

func TestEveryDeclaredReasonIsInTheRoster(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "specification.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	roster := RefusalReasons()
	declared := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || !strings.HasPrefix(value.Names[0].Name, "Reason") {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok {
				t.Fatalf("%s is not a plain string constant", value.Names[0].Name)
			}
			reason, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			declared++
			if !slices.Contains(roster, reason) {
				t.Errorf("%s refuses with %q, which RefusalReasons does not list; "+
					"a reason nobody lists is one nobody can chart", value.Names[0].Name, reason)
			}
		}
	}
	if declared == 0 {
		t.Fatal("specification.go declares no Reason constants; this test read nothing")
	}
}

func packageSourceFiles(t *testing.T) (*token.FileSet, []*ast.File) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		t.Fatal("read no source file in this package")
	}
	return fset, files
}

func TestNoGateInventsAReasonInline(t *testing.T) {
	fset, files := packageSourceFiles(t)
	calls := 0
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name, ok := call.Fun.(*ast.Ident)
			if !ok || name.Name != "refused" || len(call.Args) == 0 {
				return true
			}
			calls++
			if literal, ok := call.Args[0].(*ast.BasicLit); ok {
				t.Errorf("%s: refused(%s, ...) spells its reason inline; "+
					"declare it beside the others so the roster stays the whole vocabulary",
					fset.Position(call.Pos()), literal.Value)
			}
			return true
		})
	}
	if calls == 0 {
		t.Fatal("found no refused() call at all; this test read nothing")
	}
}
