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
	// Both subjects live in the same owner document, so a fixture that carries
	// only one of them makes the other report a lost subject. Callers who are
	// testing RELEASE pass their own §18 block in `owner`; everyone else gets
	// this one, which is simply "not the thing under test".
	if !strings.Contains(owner, "RELEASE-") {
		owner += "\n## 18. 封測准入\n\n- [x] RELEASE-007 描述\n- [ ] RELEASE-008 描述\n"
	}
	if !strings.Contains(owner, "GEN-") {
		owner += "\n## 19. M5\n\n本節共 1 項已勾、1 項 ◐。\n\n- [x] GEN-001 描述\n- [ ] GEN-009 描述\n"
	}
	if !strings.Contains(owner, "PORT-") {
		owner += "\n## 20. M6（完成 1 項、撤回 1 項、剩 1 項）\n\n" +
			"- [x] PORT-001 描述\n- [~] ~~PORT-002 描述~~\n- [ ] PORT-007 描述\n"
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

// M4's RELEASE-001～010: ten literal checkboxes in `03` §18, and `01` §10 has
// been contradicting them since 2026-08-28 (「十項全部不勾」 against two ticked).
// The M5 rule generalised, with the two things that make M4 different: the count
// is written in Chinese numerals, and the same satellite line carries a SECOND
// count — 「49 項中 16 勾」 — about a set defined in prose that no machine can
// confirm. Flagging that one is how a check loses its readers, so it must stay
// quiet about it while speaking about the other.
func TestMilestoneTallyCoversTheReleaseCheckboxesToo(t *testing.T) {
	t.Parallel()
	const owner = "## 18. 封測准入\n\n- [x] RELEASE-007 描述\n- [x] RELEASE-008 描述\n" +
		"- [ ] RELEASE-009 描述\n- [ ] RELEASE-010 描述\n"
	for _, tc := range []struct {
		name, satellite, want string
	}{{
		name:      "a satellite states the tally in Chinese numerals",
		satellite: "| M4 | **封測未開始**。`RELEASE-001`～`010` **十項全部不勾** |\n",
		want:      `states M4 的封測准入（RELEASE-001～010）'s tally ("十項全部不勾") while docs/plans/03-work-items.md counts 2 ticked and 2 open`,
	}, {
		name:      "a satellite states it in digits",
		satellite: "| M4 | `RELEASE-001`～`010` 目前 3 項已勾 |\n",
		want:      "states M4 的封測准入",
	}, {
		name:      "a satellite that carries the narrative and points at the owner",
		satellite: "| M4 | **封測未開始**。`RELEASE-001`～`010` 的勾選數以 `03` §18 為準；共同的阻擋是甲類四項 |\n",
		want:      "",
	}, {
		name: "the 49-item M4 count, which no machine can confirm, is left alone",
		// Far enough from any RELEASE mention that the neighbourhood test does
		// not claim it — the same distance the real 01 §10 row has.
		satellite: "| M4 打包與封閉測試 | **已收斂**（m4 對帳，49 項中 16 勾／33 誠實不勾）。" +
			strings.Repeat("補述。", 120) + " |\n",
		want: "",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeTally(t, owner, map[string]string{
				"docs/plans/01-goals-and-plan.md": tc.satellite,
			})
			problems := milestoneTallyProblems(root)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %v", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// M6 is the third subject and the first with a retracted state. The count it
// has to get right is 「完成 N 項、撤回 N 項、剩 N 項」, and the real defect this
// subject was added for is the middle row of this table: a satellite that kept
// counting after the owner un-ticked an item.
func TestMilestoneTallyCoversTheM6CheckboxesToo(t *testing.T) {
	t.Parallel()
	const boxes = "- [x] PORT-001 描述\n- [~] ~~PORT-002 描述~~\n- [~] ~~PORT-006 描述~~\n" +
		"- [ ] PORT-007 描述\n- [ ] PORT-009 描述\n"
	const goodOwner = "## 20. 可攜執行（M6，完成 1 項、撤回 2 項、剩 2 項）\n\n" + boxes
	for _, tc := range []struct {
		name, owner, satellite, want string
	}{{
		name:      "the owner's header counts the retracted items too",
		owner:     goodOwner,
		satellite: "M6 的三條軸見 `PORT-003`；勾選數以 `03` §20 為準，本檔不複述。\n",
		want:      "",
	}, {
		name:      "a satellite keeps its own copy of the count",
		owner:     goodOwner,
		satellite: "| M6 | **進行中：完成七項、撤回兩項、剩兩項**，`PORT-009` 見下 |\n",
		want:      `states M6's tally ("完成七項")`,
	}, {
		name:      "the owner's header forgets that an item was retracted",
		owner:     "## 20. 可攜執行（M6，完成 1 項、撤回 0 項、剩 2 項）\n\n" + boxes,
		satellite: "M6 的勾選數以 `03` §20 為準。\n",
		want:      `must say "完成 1 項、撤回 2 項、剩 2 項" and does not`,
	}, {
		name:      "a count far from any M6 mention is not claimed",
		owner:     goodOwner,
		satellite: "| M2 | 策展完成 45 項 |" + strings.Repeat("補述。", 120) + "\n",
		want:      "",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := writeTally(t, tc.owner, map[string]string{
				"docs/plans/mvp/m6/README.md": tc.satellite,
			})
			problems := milestoneTallyProblems(root)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("expected no problems, got %v", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("want exactly one problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

// The tally is derived from the checkboxes, so a document with no checkboxes has
// nothing to derive from. Both of these are green under a naive implementation.
func TestMilestoneTallySaysSoWhenItHasLostItsSubject(t *testing.T) {
	t.Parallel()
	t.Run("no GEN items", func(t *testing.T) {
		root := writeTally(t, "## 19. M5\n\n本節共 3 項已勾、2 項 ◐。GEN- 的清單搬走了。\n", nil)
		if problems := milestoneTallyProblems(root); len(problems) == 0 {
			t.Fatal("an owner document with no GEN checkboxes was accepted")
		}
	})
	t.Run("no RELEASE items", func(t *testing.T) {
		root := writeTally(t, "## 18. 封測准入\n\n十項全部不勾。RELEASE-001～010 的清單搬走了。\n", nil)
		problems := milestoneTallyProblems(root)
		if len(problems) == 0 || !strings.Contains(strings.Join(problems, "\n"), "RELEASE-") {
			t.Fatalf("an owner document with no RELEASE checkboxes was accepted: %v", problems)
		}
	})
	t.Run("no owner document", func(t *testing.T) {
		if problems := milestoneTallyProblems(t.TempDir()); len(problems) == 0 {
			t.Fatal("a missing owner document was accepted")
		}
	})
}
