package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTally lays out the owner document (checkboxes plus the §19 header
// sentence derived from them) and any satellite that also talks about M5.
func writeTally(t *testing.T, owner string, satellites map[string]string) string {
	t.Helper()
	root := t.TempDir()
	write := func(relative, contents string) {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(tallyOwner, owner)
	for relative, contents := range satellites {
		write(relative, contents)
	}
	return root
}

// Three ticked, two open, and the header says so.
const threeTwo = "## 19. M5\n\n本節共 3 項已勾、2 項 ◐。\n\n" +
	"- [x] GEN-001 描述\n- [x] GEN-002 描述\n- [x] GEN-003 描述\n- [ ] GEN-008 描述\n- [ ] GEN-009 描述\n"

func TestMilestoneTallyAcceptsAHeaderThatMatchesItsBoxes(t *testing.T) {
	t.Parallel()
	root := writeTally(t, threeTwo, map[string]string{
		// A satellite may carry the narrative, which is the useful half.
		"AGENTS.md": "M5 的 ◐ 是 `GEN-008` 與 `GEN-009`；勾選數以 `03` §19 為準，本檔不複述。\n",
	})
	if problems := milestoneTallyProblems(root); len(problems) != 0 {
		t.Fatalf("a header that matches its own boxes was rejected: %v", problems)
	}
}

func TestMilestoneTallyRejectsTheThreeWaysTheNumberHasHadFiveAuthors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		owner      string
		satellites map[string]string
		want       string
	}{{
		// The failure three of six review rounds found: the header states 8/3
		// while its own items say 9/2.
		name: "the owner's header disagrees with its own boxes",
		owner: "## 19. M5\n\n本節共 2 項已勾、3 項 ◐。\n\n" +
			"- [x] GEN-001\n- [x] GEN-002\n- [x] GEN-003\n- [ ] GEN-008\n- [ ] GEN-009\n",
		want: `must say "3 項已勾、2 項 ◐" and does not`,
	}, {
		// A satellite still saying 8/3 a day after the other four were corrected.
		name:       "a satellite states the tally at all",
		owner:      threeTwo,
		satellites: map[string]string{"docs/plans/01-goals-and-plan.md": "M5 目前 8 項已勾，其餘為 ◐。\n"},
		want:       "states M5's tally",
	}, {
		// Same sentence, same file, but about M4 - whose 49 items are defined in
		// prose no machine can count. Flagging what cannot be checked is how a
		// check loses its readers, so this one must stay quiet.
		name:       "a tally about something this check cannot count is left alone",
		owner:      threeTwo,
		satellites: map[string]string{"docs/plans/01-goals-and-plan.md": "M4 的 release-checklist 49 項中 16 勾。\n"},
		want:       "",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			problems := milestoneTallyProblems(writeTally(t, tc.owner, tc.satellites))
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %v", problems)
				}
				return
			}
			if !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// The tally is derived from the checkboxes, so a document with no checkboxes has
// nothing to derive from. Both of these are green under a naive implementation.
func TestMilestoneTallySaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("no GEN items", func(t *testing.T) {
		root := writeTally(t, "## 19. M5\n\n本節共 3 項已勾、2 項 ◐。\n", nil)
		if problems := milestoneTallyProblems(root); len(problems) == 0 {
			t.Fatal("an owner document with no GEN checkboxes was accepted")
		}
	})
	t.Run("no owner document", func(t *testing.T) {
		if problems := milestoneTallyProblems(t.TempDir()); len(problems) == 0 {
			t.Fatal("a missing owner document was accepted")
		}
	})
}
