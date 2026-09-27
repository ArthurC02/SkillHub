package apiserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func acceptAll(t *testing.T, c *client, runID string) ([]suggestionBody, string) {
	t.Helper()
	_, suggestions, evaluationID := c.listSuggestions(t, runID)
	for _, s := range suggestions {
		if code, _ := c.decide(t, s.SuggestionID, "accepted"); code != http.StatusOK {
			t.Fatalf("accept %s: got %d", s.SuggestionID, code)
		}
	}
	return suggestions, evaluationID
}

func versionsOf(t *testing.T, skillID string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM skill_versions WHERE skill_id = $1`,
		mustUUID(t, skillID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func byTarget(t *testing.T, suggestions []suggestionBody, target string, marker string) string {
	t.Helper()
	for _, s := range suggestions {
		if s.TargetPath == target && strings.Contains(s.Problem, marker) {
			return s.SuggestionID
		}
	}
	t.Fatalf("no suggestion on %s about %q among %+v", target, marker, suggestions)
	return ""
}

func TestASuggestionFromAnotherEvaluationOfTheSameWorkspaceIsNotFound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("sugg-other-eval"))
	proposal := func(name string) []llmclient.ImprovementProposal {
		return []llmclient.ImprovementProposal{{
			Category: "skill", Problem: "the description is thin", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: packagedSkillMD(name) + "\nMore detail.\n",
			ExpectedImpact: "clearer",
		}}
	}
	mine := evaluateWithSuggestions(t, a, pool, c, "sugg-other-eval-mine", proposal("sugg-other-eval-mine"))
	other := evaluateWithSuggestions(t, a, pool, c, "sugg-other-eval-theirs", proposal("sugg-other-eval-theirs"))
	_, mineEvaluation := acceptAll(t, c, mine.runID)
	otherSuggestions, _ := acceptAll(t, c, other.runID)

	code, applied := c.applySuggestions(t, mine.skillID, mineEvaluation, otherSuggestions[0].SuggestionID)
	if code != http.StatusNotFound || applied.VersionID != "" {
		t.Fatalf("applying another evaluation's suggestion: got %d %+v, want 404 and no version", code, applied)
	}
	if n := versionsOf(t, mine.skillID); n != 1 {
		t.Errorf("the skill has %d versions, want 1", n)
	}
}

func TestASuggestionWhoseFileHasMovedOnIsRefusedWhileTheRestOfTheBatchApplies(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("sugg-moved-on"))
	const name = "sugg-moved-on-skill"
	base := packagedSkillMD(name)
	seed := evaluateWithSuggestions(t, a, pool, c, name, []llmclient.ImprovementProposal{
		{Category: "skill", Problem: "no mention of deduplication", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: base + "\nIt deduplicates rows.\n", ExpectedImpact: "better activation"},
		{Category: "skill", Problem: "no mention of the output format", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: base + "\nIt writes an xlsx file.\n", ExpectedImpact: "better activation"},
		{Category: "skill", Problem: "no notes on edge cases", Evidence: suggestionQuote,
			TargetPath: "references/notes.md", ProposedContent: "Empty rows are kept.\n", ExpectedImpact: "fewer surprises"},
	})
	suggestions, evaluationID := acceptAll(t, c, seed.runID)
	dedupe := byTarget(t, suggestions, "SKILL.md", "deduplication")
	format := byTarget(t, suggestions, "SKILL.md", "output format")
	notes := byTarget(t, suggestions, "references/notes.md", "edge cases")

	if code, first := c.applySuggestions(t, seed.skillID, evaluationID, dedupe); code != http.StatusCreated {
		t.Fatalf("the first change: got %d (%s)", code, first.Error)
	}
	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, format, notes)
	if code != http.StatusCreated {
		t.Fatalf("the mixed batch: got %d (%s)", code, applied.Error)
	}
	if len(applied.AppliedSuggestionIDs) != 1 || applied.AppliedSuggestionIDs[0] != notes {
		t.Errorf("applied %v, want only the notes suggestion %s", applied.AppliedSuggestionIDs, notes)
	}
	if len(applied.RejectedSuggestions) != 1 || applied.RejectedSuggestions[0].SuggestionID != format ||
		applied.RejectedSuggestions[0].BlockedReason != "target_changed" {
		t.Errorf("rejected %+v, want %s as target_changed", applied.RejectedSuggestions, format)
	}
	if applied.VersionNumber != 3 {
		t.Errorf("version number %d, want 3", applied.VersionNumber)
	}
	_, _, key, _ := latestVersionOf(t, pool, seed.skillID)
	if got := storedFile(t, a, key, "references/notes.md"); got != "Empty rows are kept.\n" {
		t.Errorf("notes in the new version = %q", got)
	}
	if got := storedFile(t, a, key, "SKILL.md"); !strings.Contains(got, "deduplicates") || strings.Contains(got, "xlsx") {
		t.Errorf("SKILL.md in the new version = %q, want the first change and not the refused one", got)
	}
}

func TestAppliedSuggestionsAreReportedInTheOrderOfTheFilesTheyChange(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("sugg-file-order"))
	const name = "sugg-file-order-skill"
	seed := evaluateWithSuggestions(t, a, pool, c, name, []llmclient.ImprovementProposal{
		{Category: "skill", Problem: "no mention of deduplication", Evidence: suggestionQuote,
			TargetPath: "SKILL.md", ProposedContent: packagedSkillMD(name) + "\nIt deduplicates rows.\n", ExpectedImpact: "better activation"},
		{Category: "skill", Problem: "no notes on edge cases", Evidence: suggestionQuote,
			TargetPath: "references/notes.md", ProposedContent: "Empty rows are kept.\n", ExpectedImpact: "fewer surprises"},
	})
	suggestions, evaluationID := acceptAll(t, c, seed.runID)
	skill := byTarget(t, suggestions, "SKILL.md", "deduplication")
	notes := byTarget(t, suggestions, "references/notes.md", "edge cases")

	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, notes, skill)
	if code != http.StatusCreated {
		t.Fatalf("got %d (%s)", code, applied.Error)
	}
	if len(applied.AppliedSuggestionIDs) != 2 || applied.AppliedSuggestionIDs[0] != skill || applied.AppliedSuggestionIDs[1] != notes {
		t.Errorf("applied %v, want [%s %s]: SKILL.md sorts before references/notes.md", applied.AppliedSuggestionIDs, skill, notes)
	}
}

func TestAValidationRefusalNamesWhatTheImportCheckFound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("sugg-refusal-codes"))
	seed := evaluateWithSuggestions(t, a, pool, c, "sugg-refusal-codes-skill", []llmclient.ImprovementProposal{{
		Category: "skill", Problem: "the document is too long", Evidence: suggestionQuote,
		TargetPath: "SKILL.md", ProposedContent: "just prose, no frontmatter at all\n", ExpectedImpact: "shorter",
	}})
	suggestions, evaluationID := acceptAll(t, c, seed.runID)

	code, applied := c.applySuggestions(t, seed.skillID, evaluationID, suggestions[0].SuggestionID)
	if code != http.StatusUnprocessableEntity || len(applied.RejectedSuggestions) != 1 {
		t.Fatalf("got %d %+v, want one refusal", code, applied)
	}
	msg := applied.RejectedSuggestions[0].Message
	if !strings.HasPrefix(msg, "with this change applied the package no longer passes the validation an import has to pass (") ||
		!strings.HasSuffix(msg, ")") {
		t.Errorf("message = %q, want the refusal to name the failing checks", msg)
	}
}
