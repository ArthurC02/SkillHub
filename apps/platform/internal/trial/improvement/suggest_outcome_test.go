package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/verdict"
)

type failingSuggester struct{}

func (failingSuggester) SuggestImprovements(
	context.Context, modelport.ImprovementRequest,
) (*modelport.Improvements, error) {
	return nil, errors.New("gateway timed out")
}

func proposalQuoting(quote string) modelport.ImprovementProposal {
	return modelport.ImprovementProposal{
		Category: "skill", Problem: "the description does not mention xlsx",
		Evidence: quote, TargetPath: "SKILL.md", ProposedContent: "new",
		ExpectedImpact: "activation improves",
	}
}

func verdictWithoutEvidence() assessment {
	return assessment{overall: verdict.OverallNotMet, findings: []verdict.Finding{{
		Category: verdict.CategoryEffect, Severity: verdict.SeverityWarning, Message: "needs work",
		Evidence: []verdict.EvidenceRef{},
	}}}
}

func storedSuggestionOutcome(t *testing.T, s *Service, ev gen.Evaluation) string {
	t.Helper()
	var outcome *string
	if err := s.Pool.QueryRow(context.Background(),
		`SELECT suggestion_outcome FROM evaluations WHERE id = $1`, ev.ID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	if outcome == nil {
		return "<null>"
	}
	return *outcome
}

func TestAnEvaluationRecordsWhyItHasTheSuggestionsItHas(t *testing.T) {
	pool := requireEvalDB(t)
	cases := []struct {
		name      string
		suggester modelport.Suggester
		held      bool
		judged    assessment
		want      string
	}{
		{"no suggestion model in this deployment", nil, false, costTestVerdict(), "<null>"},
		{"a skill under a licence hold", stubSuggester{}, true, costTestVerdict(), "restricted"},
		{"a met verdict with only info findings", stubSuggester{}, false, aVerdict("fine", verdict.OverallMet), "not_needed"},
		{"a verdict citing nothing", stubSuggester{}, false, verdictWithoutEvidence(), "no_evidence"},
		{"the suggestion model failing", failingSuggester{}, false, costTestVerdict(), "unavailable"},
		{"the model answering with no proposals", stubSuggester{}, false, costTestVerdict(), "none_proposed"},
		{"every proposal quoting nothing the verdict cited", stubSuggester{resp: modelport.Improvements{
			Proposals: []modelport.ImprovementProposal{proposalQuoting("words the judge never saw")},
		}}, false, costTestVerdict(), "none_kept"},
		{"a proposal quoting the verdict's evidence", stubSuggester{resp: modelport.Improvements{
			Proposals: []modelport.ImprovementProposal{proposalQuoting("wrote report.md")},
		}}, false, costTestVerdict(), "kept"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &Service{Pool: pool, Suggester: c.suggester}
			m := seedRun(t, pool)
			m.Skill.AccessRestricted = c.held
			ev, err := s.begin(context.Background(), m)
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			if err := s.completeAndSuggest(context.Background(), m, ev, c.judged); err != nil {
				t.Fatalf("complete and suggest: %v", err)
			}
			if got := storedSuggestionOutcome(t, s, ev); got != c.want {
				t.Errorf("suggestion outcome = %s, want %s", got, c.want)
			}
		})
	}
}

func TestASuggestionOutcomeIsWrittenOnce(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ev := beginAndComplete(t, s, m, aVerdict("not met", verdict.OverallNotMet))

	recordSuggestionOutcome(context.Background(), s.queries(), ev, SuggestionsUnavailable)
	recordSuggestionOutcome(context.Background(), s.queries(), ev, SuggestionsKept)

	if got := storedSuggestionOutcome(t, s, ev); got != "unavailable" {
		t.Errorf("suggestion outcome after a second write = %s, want the first, unavailable", got)
	}
}
