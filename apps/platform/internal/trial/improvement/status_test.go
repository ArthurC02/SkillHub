package eval_test

import (
	"slices"
	"testing"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

var legalJudgements = map[eval.Status][]eval.Status{
	eval.StatusPending: {eval.StatusCompleted, eval.StatusFailed},
}

func TestAnEvaluationSettlesOnceAndOnlyFromPending(t *testing.T) {
	if len(eval.AllStatuses()) != 3 {
		t.Fatalf("an evaluation is pending, completed or failed, got %d states", len(eval.AllStatuses()))
	}
	for _, from := range eval.AllStatuses() {
		for _, to := range eval.AllStatuses() {
			if from == to {
				continue
			}
			want := slices.Contains(legalJudgements[from], to)
			if got := eval.CanTransitionStatus(from, to); got != want {
				t.Errorf("%s -> %s: table says %v, the queries say %v", from, to, got, want)
			}
		}
	}
}

func TestASettledEvaluationIsNotJudgedAgain(t *testing.T) {
	for _, settled := range []eval.Status{eval.StatusCompleted, eval.StatusFailed} {
		if settled.AwaitsTheJudge() {
			t.Errorf("%s has already been judged", settled)
		}
		for _, to := range eval.AllStatuses() {
			if to == settled {
				continue
			}
			if eval.CanTransitionStatus(settled, to) {
				t.Errorf("a superseding evaluation is a new row, so %s must not reach %s", settled, to)
			}
		}
	}
}

func TestOnlyAPendingEvaluationAwaitsTheJudge(t *testing.T) {
	for _, s := range eval.AllStatuses() {
		if want := s == eval.StatusPending; s.AwaitsTheJudge() != want {
			t.Errorf("%s: AwaitsTheJudge is %v, want %v", s, s.AwaitsTheJudge(), want)
		}
	}
}

func TestRewritingTheSameEvaluationStatusIsAllowed(t *testing.T) {
	for _, s := range eval.AllStatuses() {
		if !eval.CanTransitionStatus(s, s) {
			t.Errorf("%s: a rewrite that changes nothing must not be refused", s)
		}
	}
}

func TestAnEvaluationStatusNobodyDeclaredIsRefused(t *testing.T) {
	for _, bogus := range []string{"", "done", "Pending", "timed_out"} {
		if _, ok := eval.ParseStatus(bogus); ok {
			t.Errorf("%q is not an evaluation status", bogus)
		}
		if eval.CanTransitionStatus(eval.Status(bogus), eval.StatusCompleted) {
			t.Errorf("an evaluation sitting on %q must not be settled", bogus)
		}
	}
	for _, known := range eval.AllStatuses() {
		if _, ok := eval.ParseStatus(string(known)); !ok {
			t.Errorf("%s is declared but ParseStatus rejects it", known)
		}
	}
}
