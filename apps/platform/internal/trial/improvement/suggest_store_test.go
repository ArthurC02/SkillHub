package eval

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

const citableExcerpt = "bash exited 1 while writing output.xlsx"

type countingSuggester struct{ calls int }

func (c *countingSuggester) SuggestImprovements(
	context.Context, modelport.ImprovementRequest,
) (*modelport.Improvements, error) {
	c.calls++
	return &modelport.Improvements{}, nil
}

func TestNoSuggesterCallWithoutCitableEvidence(t *testing.T) {
	suggester := &countingSuggester{}
	s := &Service{Suggester: suggester}
	uncited := assessment{overall: verdict.OverallNotMet, findings: []verdict.Finding{{
		Category: verdict.CategoryEffect, Severity: verdict.SeverityWarning, Message: "needs work",
	}}}

	s.suggest(context.Background(), facts.Material{}, gen.Evaluation{}, uncited)

	if suggester.calls != 0 {
		t.Fatalf("suggester called %d times for a verdict with nothing to cite, want 0", suggester.calls)
	}
}

func TestTheDigestLeavesOutPassedCriteriaAndInfoFindings(t *testing.T) {
	const passedExcerpt = "the passed criterion quoted this line"
	const infoExcerpt = "an informational finding quoted this one"
	v := assessment{overall: verdict.OverallNotMet,
		results: []verdict.CriterionResult{
			{CriterionID: "c1", Text: "met", Result: verdict.ResultPassed, Reason: "fine",
				Evidence: []verdict.EvidenceRef{{Kind: verdict.KindAgentOutput, Excerpt: passedExcerpt, Available: true}}},
			{CriterionID: "c2", Text: "not met", Result: verdict.ResultFailed, Reason: "missing",
				Evidence: []verdict.EvidenceRef{{Kind: verdict.KindAgentOutput, Excerpt: citableExcerpt, Available: true}}},
		},
		findings: []verdict.Finding{{
			Category: verdict.CategoryEffect, Severity: verdict.SeverityInfo, Message: "disclosure",
			Evidence: []verdict.EvidenceRef{{Kind: verdict.KindAgentOutput, Excerpt: infoExcerpt, Available: true}},
		}},
	}

	digest, refs := suggestionDigest(facts.Material{}, v)

	if len(refs) != 1 || refs[0].Excerpt != citableExcerpt {
		t.Fatalf("allowlist = %+v, want only the failed criterion's excerpt", refs)
	}
	for _, excluded := range []string{passedExcerpt, infoExcerpt} {
		if strings.Contains(digest, excluded) {
			t.Errorf("the digest shows %q, which a suggestion must not cite", excluded)
		}
	}
}

func TestAtMostTenSuggestionsAreStoredPerEvaluation(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ev := beginAndComplete(t, s, m, aVerdict("not met", verdict.OverallNotMet))
	refs := []verdict.EvidenceRef{{Kind: verdict.KindAgentOutput, Excerpt: citableExcerpt, Available: true}}
	var proposals []modelport.ImprovementProposal
	for i := range maxSuggestionsStore + 2 {
		proposals = append(proposals, modelport.ImprovementProposal{
			Category: "skill", Problem: fmt.Sprintf("problem %d", i), Evidence: citableExcerpt,
			TargetPath: "SKILL.md", ProposedContent: fmt.Sprintf("content %d", i), ExpectedImpact: "better",
		})
	}

	s.storeProposals(context.Background(), ev, proposals, refs)

	var stored int
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM evaluation_suggestions WHERE evaluation_id = $1`, ev.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != maxSuggestionsStore {
		t.Fatalf("stored %d of %d suggestions, want %d", stored, len(proposals), maxSuggestionsStore)
	}
}
