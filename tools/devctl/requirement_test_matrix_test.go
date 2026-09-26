package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const matrixFixtureSpec = "#### DISC-001：搜尋\n\n#### DISC-002：結果\n\n#### TEST-003：後 MVP 的東西\n"

func writeMatrixFixture(t *testing.T, matrix string) string {
	t.Helper()
	root := t.TempDir()
	for path, body := range map[string]string{
		filepath.FromSlash(requirementSpecPath):   matrixFixtureSpec,
		filepath.FromSlash(requirementMatrixPath): matrix,
		filepath.Join("apps", "x", "x_test.go"):   "func TestSomethingReal(t *testing.T) {}\n",
	} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const matrixBothRows = "| DISC-001 | 有測試 | `TestSomethingReal` |  |\n" +
	"| DISC-002 | 部分 | `TestSomethingReal` | 第二條沒有 |\n"

func TestRequirementTestMatrixAcceptsACompleteTable(t *testing.T) {
	t.Parallel()
	if problems := requirementTestMatrixProblems(writeMatrixFixture(t, matrixBothRows)); len(problems) != 0 {
		t.Fatalf("a complete matrix was rejected: %v", problems)
	}
}

func TestRequirementTestMatrixRejectsDrift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		matrix string
		want   string
	}{{
		name:   "an MVP requirement with no row",
		matrix: "| DISC-001 | 有測試 | `TestSomethingReal` |  |\n",
		want:   "DISC-002 is MVP-required",
	}, {
		name:   "a test name nothing declares",
		matrix: matrixBothRows + "| DISC-001 | 有測試 | `TestNobodyWrote` |  |\n",
		want:   `names "TestNobodyWrote"`,
	}, {
		name:   "a post-MVP requirement smuggled in",
		matrix: matrixBothRows + "| TEST-003 | 有測試 | `TestSomethingReal` |  |\n",
		want:   "which docs/plans/02-specifications-and-acceptance-criteria.md marks 後 MVP",
	}, {
		name:   "an id the spec has no heading for",
		matrix: matrixBothRows + "| MADE-999 | 有測試 | `TestSomethingReal` |  |\n",
		want:   "is not a requirement heading",
	}, {
		name:   "a status outside the five",
		matrix: "| DISC-001 | 應該可以 | `TestSomethingReal` |  |\n| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n",
		want:   "is not one of the five",
	}, {
		name:   "有測試 with nothing named",
		matrix: "| DISC-001 | 有測試 |  |  |\n| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n",
		want:   "but names no test",
	}, {
		name:   "有測試 that still states a gap",
		matrix: "| DISC-001 | 有測試 | `TestSomethingReal` | 其實還缺一條 |\n| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n",
		want:   "yet states a gap",
	}, {
		name:   "部分 that never says what is missing",
		matrix: "| DISC-001 | 部分 | `TestSomethingReal` |  |\n| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n",
		want:   "without saying what is missing",
	}, {
		name:   "the same requirement twice",
		matrix: matrixBothRows + "| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n",
		want:   "appears more than once",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := requirementTestMatrixProblems(writeMatrixFixture(t, tc.matrix))
			if len(problems) == 0 {
				t.Fatalf("drift was accepted")
			}
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("expected a problem naming %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestRequirementTestMatrixAcceptsAFilePathAsTheProof(t *testing.T) {
	t.Parallel()
	root := writeMatrixFixture(t, "| DISC-001 | 有測試 | `apps/x/x_test.go` |  |\n"+
		"| DISC-002 | 部分 | `TestSomethingReal` | 缺 |\n")
	if problems := requirementTestMatrixProblems(root); len(problems) != 0 {
		t.Fatalf("a real file path was rejected as proof: %v", problems)
	}
}

func TestRequirementTestMatrixReadsTheLiveTable(t *testing.T) {
	t.Parallel()
	required, postMVP, err := specRequirementIDs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(required) != 94 || len(postMVP) != 8 {
		t.Fatalf("the live spec reads as %d MVP-required and %d 後 MVP; the table is built for 94 and 8",
			len(required), len(postMVP))
	}
	if problems := requirementTestMatrixProblems("../.."); len(problems) != 0 {
		t.Fatalf("the live matrix has drifted: %v", problems)
	}
}

func TestRequirementTestMatrixReadsRuntimeScriptTests(t *testing.T) {
	t.Parallel()
	root := writeMatrixFixture(t, matrixBothRows)
	script := filepath.Join(root, "infra", "images", "runtime", "run.test.mjs")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("test(\"a declared directory installs one skill\", () => {});\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matrix := "| DISC-001 | 有測試 | `a declared directory installs one skill` |  |\n" +
		"| DISC-002 | 部分 | `TestSomethingReal` | 第二條沒有 |\n"
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(requirementMatrixPath)), []byte(matrix), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := requirementTestMatrixProblems(root); len(problems) != 0 {
		t.Fatalf("a test named in a .test.mjs suite was not found: %v", problems)
	}
}
