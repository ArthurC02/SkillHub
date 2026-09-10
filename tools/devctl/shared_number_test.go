package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSites(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestSharedNumberAcceptsCopiesThatAgree(t *testing.T) {
	t.Parallel()
	root := writeSites(t, map[string]string{
		"apps/platform/judge.go":       "const maxDigestEntry = 8000 // one-number: maxDigestEntry\n",
		"contracts/openapi/llm.yaml":   "          maxLength: 8000  # one-number: maxDigestEntry\n",
		"tools/eval-regression/j.py":   "MAX_DIGEST_ENTRY = 8_000  # one-number: maxDigestEntry - raised 2026-08-23\n",
		"apps/llm/src/evaluate.py":     "    excerpt: str = Field(..., max_length=8000)  # one-number: maxDigestEntry\n",
		"apps/web/src/components/a.ts": "const MAX = 8000; // one-number: maxDigestEntry\n",
	})
	if problems := sharedNumberProblemsFor(root, []string{"maxDigestEntry"}); len(problems) != 0 {
		t.Fatalf("five agreeing sites were rejected: %v", problems)
	}
}

func TestSharedNumberScansInfrastructureSources(t *testing.T) {
	t.Parallel()
	root := writeSites(t, map[string]string{
		"apps/platform/archive.go": "const maxEntries = 2000 // one-number: maxSkillPackageEntries\n",
		"infra/runtime/run.mjs":    "const MAX_ENTRIES = 2000; // one-number: maxSkillPackageEntries\n",
	})
	if problems := sharedNumberProblemsFor(root, []string{"maxSkillPackageEntries"}); len(problems) != 0 {
		t.Fatalf("an infrastructure copy was not compared: %v", problems)
	}
}

func TestSharedNumberRejectsTheWaysACopyStopsBeingCompared(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		roster []string
		files  map[string]string
		want   string
	}{{

		name:   "one copy was raised and the others were not",
		roster: []string{"maxDigestEntry"},
		files: map[string]string{
			"apps/platform/judge.go":     "const maxDigestEntry = 8000 // one-number: maxDigestEntry\n",
			"contracts/openapi/llm.yaml": "          maxLength: 2000  # one-number: maxDigestEntry\n",
		},
		want: "disagrees across 2 sites",
	}, {

		name:   "only one site still carries a marker",
		roster: []string{"maxDigestEntry"},
		files: map[string]string{
			"apps/platform/judge.go":     "const maxDigestEntry = 8000 // one-number: maxDigestEntry\n",
			"contracts/openapi/llm.yaml": "          maxLength: 8000\n",
		},
		want: "has only one marked site",
	}, {

		name:   "every marker for a rostered invariant is gone",
		roster: []string{"maxDigestEntry"},
		files: map[string]string{
			"apps/platform/judge.go":     "const maxDigestEntry = 8000\n",
			"contracts/openapi/llm.yaml": "          maxLength: 8000\n",
		},
		want: "is on the roster in tools/devctl/shared_number.go but no marked site was found",
	}, {

		name:   "a marked invariant that nobody rostered",
		roster: nil,
		files: map[string]string{
			"apps/platform/judge.go":     "const maxDigestEntry = 8000 // one-number: maxDigestEntry\n",
			"contracts/openapi/llm.yaml": "          maxLength: 8000  # one-number: maxDigestEntry\n",
		},
		want: "is not on the roster in tools/devctl/shared_number.go",
	}, {
		name:   "a marker on a line with no number",
		roster: []string{"maxDigestEntry"},
		files: map[string]string{
			"apps/platform/judge.go":     "var x = f() // one-number: maxDigestEntry\n",
			"contracts/openapi/llm.yaml": "          maxLength: 8000  # one-number: maxDigestEntry\n",
		},
		want: "marks a line with no number on it",
	}, {

		name:   "a marker that does not open its comment does not count",
		roster: []string{"maxDigestEntry"},
		files: map[string]string{
			"apps/platform/judge.go":     "const maxDigestEntry = 8000 // one-number: maxDigestEntry\n",
			"tools/eval-regression/j.py": "MAX = 2000  # raised 2026-08-23; one-number: maxDigestEntry\n",
		},
		want: "has only one marked site",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeSites(t, tc.files)
			problems := sharedNumberProblemsFor(root, tc.roster)
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestSharedNumberReadsUnderscoredIntegers(t *testing.T) {
	t.Parallel()
	root := writeSites(t, map[string]string{
		"apps/platform/judge.go":   "const maxFinalOutput = 40000 // one-number: maxFinalOutput\n",
		"apps/llm/src/evaluate.py": "    final_output: str = Field(..., max_length=40_000)  # one-number: maxFinalOutput\n",
	})
	if problems := sharedNumberProblemsFor(root, []string{"maxFinalOutput"}); len(problems) != 0 {
		t.Fatalf("40_000 and 40000 were reported as different: %v", problems)
	}
}

func TestSharedNumberSkipsGeneratedAndVendoredTrees(t *testing.T) {
	t.Parallel()
	root := writeSites(t, map[string]string{
		"apps/platform/judge.go":                 "const maxCriteria = 20 // one-number: maxCriteria\n",
		"contracts/openapi/llm.yaml":             "          maxItems: 20  # one-number: maxCriteria\n",
		"apps/platform/internal/db/gen/q.sql.go": "const x = 99 // one-number: maxCriteria\n",
		"apps/llm/.venv/lib/site.py":             "X = 99  # one-number: maxCriteria\n",
		"tools/devctl/shared_number_test.go":     "const fixture = 99 // one-number: maxCriteria\n",
	})
	if problems := sharedNumberProblemsFor(root, []string{"maxCriteria"}); len(problems) != 0 {
		t.Fatalf("generated or vendored copies voted: %v", problems)
	}
}

func TestSharedNumberFixturesDoNotLeakIntoTheRealScan(t *testing.T) {
	t.Parallel()
	found, problems := sharedNumberScan("../..")
	if len(found) == 0 {
		t.Fatal("the real tree yielded no marked sites at all; this test has lost its subject")
	}
	for _, name := range sharedNumberRoster {
		for _, site := range found[name] {
			if strings.HasPrefix(filepath.ToSlash(site.file), "tools/devctl/") {
				t.Errorf("one-number: %s counts %s:%d - this package's own fixture is being read as a production copy",
					name, site.file, site.line)
			}
		}
	}
	for _, problem := range problems {
		if strings.Contains(filepath.ToSlash(problem), "tools/devctl/") {
			t.Errorf("the scan complained about its own package: %s", problem)
		}
	}
}

func TestSharedNumberOwnTestIsExactlyThisPackage(t *testing.T) {
	t.Parallel()
	for relative, want := range map[string]bool{
		"tools/devctl/shared_number_test.go":        true,
		"tools/devctl/query_owners_test.go":         true,
		"tools/devctl/shared_number.go":             false,
		"tools/eval-regression/judge_regression.py": false,
		"tools/eval-regression/harness_test.go":     false,
		"tools/devctl/fixtures/nested_test.go":      false,
		"apps/platform/internal/eval/judge_test.go": false,
	} {
		if got := sharedNumberOwnTest(relative); got != want {
			t.Errorf("sharedNumberOwnTest(%q) = %v, want %v", relative, got, want)
		}
	}
}

func TestSharedNumberRosterIsWellFormed(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for i, name := range sharedNumberRoster {
		if seen[name] {
			t.Errorf("roster lists %q twice", name)
		}
		seen[name] = true
		if i > 0 && sharedNumberRoster[i-1] > name {
			t.Errorf("roster is not sorted: %q comes after %q", name, sharedNumberRoster[i-1])
		}
		if m := sharedNumberMarker.FindStringSubmatch("// one-number: " + name); m == nil || m[1] != name {
			t.Errorf("roster entry %q is not a name the marker pattern reads back", name)
		}
	}
}
