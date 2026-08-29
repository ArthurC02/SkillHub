package main

import (
	"strings"
	"testing"
	"time"
)

// Pointed at the tree. RED while the two sides spell the budget differently —
// the Go markers say `LLM_TIMEOUT_SECONDS`, apps/llm says
// `evaluate.LLM_TIMEOUT_SECONDS`, and two Python modules define that constant,
// so the qualified name is the correct one and the Go side is the half to fix.
func TestTheRealTimeoutBudgetsPair(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if problems := timeoutBudgetProblems(root); len(problems) > 0 {
		t.Fatalf("%d timeout budget problem(s):\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// A check that found no markers at all would report success. The tree carries
// them, so their absence is a broken scan.
func TestTheTimeoutMarkerScanStillFindsMarkers(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	over, problems := scanTimeoutMarkers(root, timeoutGoRoots, ".go", timeoutOverMarker, parseGoDuration)
	if len(problems) > 0 {
		t.Fatalf("scanning the Go side reported problems: %v", problems)
	}
	if len(over) == 0 {
		t.Fatal("no `// budget-over:` marker found in apps/platform or apps/sandbox; " +
			"either every marker fell off or this scan is looking at the wrong thing")
	}
	ceiling, problems := scanTimeoutMarkers(root, timeoutPyRoots, ".py", timeoutCeilingMarker, parsePySeconds)
	if len(problems) > 0 {
		t.Fatalf("scanning the Python side reported problems: %v", problems)
	}
	if len(ceiling) == 0 {
		t.Fatal("no `# budget-ceiling:` marker found in apps/llm/src")
	}
}

func writeBudgetFixture(t *testing.T, goBody, pyBody string) string {
	t.Helper()
	root := t.TempDir()
	writeAt(t, root, "apps/platform/internal/trial/improvement/eval.go", goBody)
	writeAt(t, root, "apps/llm/src/skillhub_llm/evaluate.py", pyBody)
	return root
}

func TestTimeoutBudgetAcceptsAGoDeadlineWithEnoughSlack(t *testing.T) {
	t.Parallel()
	root := writeBudgetFixture(t,
		"package improvement\n\nconst judgeTimeout = 135 * time.Second // budget-over: evaluate.LLM_TIMEOUT_SECONDS\n",
		"# budget-ceiling: evaluate.LLM_TIMEOUT_SECONDS\nLLM_TIMEOUT_SECONDS = 120.0\n")
	if problems := timeoutBudgetProblems(root); len(problems) != 0 {
		t.Fatalf("135s over a 120s ceiling was rejected: %v", problems)
	}
}

func TestTimeoutBudgetRefusesTheThreeWaysThePairFails(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, goBody, pyBody, want string
	}{{
		// Exactly at the ceiling: Go and the gateway race, and the loser is the
		// user (Go records a timeout for a call that succeeded and was billed).
		name:   "Go equals Python, which is not a margin",
		goBody: "package improvement\n\nconst judgeTimeout = 120 * time.Second // budget-over: B\n",
		pyBody: "# budget-ceiling: B\nT = 120.0\n",
		want:   "Go must be at least 5s above Python",
	}, {
		name:   "Go is under Python",
		goBody: "package improvement\n\nconst judgeTimeout = 60 * time.Second // budget-over: B\n",
		pyBody: "# budget-ceiling: B\nT = 120.0\n",
		want:   "gives B 1m0s while",
	}, {
		name:   "one second short of the margin",
		goBody: "package improvement\n\nconst judgeTimeout = 124 * time.Second // budget-over: B\n",
		pyBody: "# budget-ceiling: B\nT = 120.0\n",
		want:   "Go must be at least 5s above Python",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := timeoutBudgetProblems(writeBudgetFixture(t, tc.goBody, tc.pyBody))
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// A marker with no partner is the failure this pairing exists for: it looks
// exactly like one that is doing its job.
func TestTimeoutBudgetRefusesAOneSidedMarker(t *testing.T) {
	t.Parallel()
	t.Run("Go alone", func(t *testing.T) {
		t.Parallel()
		root := writeBudgetFixture(t,
			"package improvement\n\nconst judgeTimeout = 135 * time.Second // budget-over: B\n",
			"LLM_TIMEOUT_SECONDS = 120.0\n")
		problems := timeoutBudgetProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "a one-sided marker protects nothing") {
			t.Fatalf("an unpaired Go marker was accepted: %v", problems)
		}
	})
	t.Run("Python alone", func(t *testing.T) {
		t.Parallel()
		root := writeBudgetFixture(t,
			"package improvement\n\nconst judgeTimeout = 135 * time.Second\n",
			"# budget-ceiling: B\nT = 120.0\n")
		problems := timeoutBudgetProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "nothing is holding the Go deadline above it") {
			t.Fatalf("an unpaired Python marker was accepted: %v", problems)
		}
	})
	t.Run("two ceilings for one budget", func(t *testing.T) {
		t.Parallel()
		root := writeBudgetFixture(t,
			"package improvement\n\nconst judgeTimeout = 135 * time.Second // budget-over: B\n",
			"# budget-ceiling: B\nT = 120.0\n\n# budget-ceiling: B\nU = 90.0\n")
		problems := timeoutBudgetProblems(root)
		if len(problems) != 1 || !strings.Contains(problems[0], "a budget has one ceiling") {
			t.Fatalf("two ceilings were accepted: %v", problems)
		}
	})
}

// A marker on a line with no readable duration must be loud, not skipped —
// silently dropping a pair is how a one-sided marker becomes invisible.
func TestTimeoutBudgetRefusesAMarkerWithNoNumberNearIt(t *testing.T) {
	t.Parallel()
	root := writeBudgetFixture(t,
		"package improvement\n\n// budget-over: B\n// still nothing here\n// nor here\nconst j = 135 * time.Second\n",
		"# budget-ceiling: B\nT = 120.0\n")
	problems := timeoutBudgetProblems(root)
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "carries a duration this check can read") {
		t.Fatalf("a marker with no number near it was skipped: %v", problems)
	}
}

// Both value parsers, including the shapes that must be refused rather than
// guessed at.
func TestTimeoutValueParsers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		line string
		want time.Duration
		ok   bool
	}{
		{"\tjudgeTimeout = 135 * time.Second // budget-over: B", 135 * time.Second, true},
		{"\tgrantFetchTimeout = 2 * time.Minute", 2 * time.Minute, true},
		{"\tt = 500 * time.Millisecond", 500 * time.Millisecond, true},
		// A duration built at runtime is not a constant this check can compare.
		{"\tt = time.Duration(envInt(\"X\", 30)) * time.Second", 0, false},
		{"\tt = someOther(30)", 0, false},
	} {
		got, ok := parseGoDuration(tc.line)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseGoDuration(%q) = %v,%v want %v,%v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
	for _, tc := range []struct {
		line string
		want time.Duration
		ok   bool
	}{
		{"LLM_TIMEOUT_SECONDS = 120.0", 120 * time.Second, true},
		{"EMBED_TIMEOUT_SECONDS = 20.0  # admission/enrich.go embedTimeout", 20 * time.Second, true},
		{"T = 8", 8 * time.Second, true},
		{"T = 0.5", 500 * time.Millisecond, true},
		{"client = _client(LLM_TIMEOUT_SECONDS)", 0, false},
	} {
		got, ok := parsePySeconds(tc.line)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parsePySeconds(%q) = %v,%v want %v,%v", tc.line, got, ok, tc.want, tc.ok)
		}
	}
}
