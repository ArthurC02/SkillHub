package eval_test

import (
	"testing"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func TestOnlyAPendingEvaluationAwaitsTheJudge(t *testing.T) {
	for _, s := range eval.AllStatuses() {
		if want := s == eval.StatusPending; s.AwaitsTheJudge() != want {
			t.Errorf("%s: AwaitsTheJudge is %v, want %v", s, s.AwaitsTheJudge(), want)
		}
	}
}
