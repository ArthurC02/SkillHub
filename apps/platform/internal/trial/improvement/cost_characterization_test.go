package eval

import (
	"context"
	"testing"
)

func TestAStoredModelUsageNamesTheModelAndPromptThatAnswered(t *testing.T) {
	cost := 0.0022
	s := &Service{
		Pool:   requireEvalDB(t),
		Credit: &fakeLedger{},
		Suggester: stubSuggester{resp: Improvements{
			Model: "suggest-model", PromptVersion: "suggest/v2",
			Usage: &ModelUsage{PromptTokens: 3000, CompletionTokens: 900, CostUSD: &cost, CostReported: true},
		}},
	}
	m := seedRun(t, s.Pool)
	ev := beginAndComplete(t, s, m, aVerdict("not met", OverallNotMet))

	s.suggest(context.Background(), m, ev, costTestVerdict())

	var model, promptVersion string
	var promptTokens, completionTokens int64
	if err := s.Pool.QueryRow(context.Background(), `
		SELECT model, prompt_version, prompt_tokens, completion_tokens FROM evaluation_model_usage
		WHERE evaluation_id = $1 AND operation = 'suggest'`, ev.ID).Scan(&model, &promptVersion, &promptTokens, &completionTokens); err != nil {
		t.Fatalf("the suggestion's model usage was not stored: %v", err)
	}
	if model != "suggest-model" || promptVersion != "suggest/v2" || promptTokens != 3000 || completionTokens != 900 {
		t.Errorf("usage = %s %s %d/%d, want suggest-model suggest/v2 3000/900", model, promptVersion, promptTokens, completionTokens)
	}
}
