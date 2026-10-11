package batch

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func distinctTestCases(n int) []pgtype.UUID {
	ids := make([]pgtype.UUID, n)
	for i := range ids {
		ids[i] = pgtype.UUID{Bytes: [16]byte{byte(i), byte(i >> 8), 1}, Valid: true}
	}
	return ids
}

func TestABatchShapeKeepsTrialsAndTestCasesWithinTheirLimits(t *testing.T) {
	duplicate := distinctTestCases(2)
	duplicate[1] = duplicate[0]
	cases := []struct {
		name   string
		batch  NewBatch
		accept bool
	}{
		{"one trial", NewBatch{TestCaseIDs: distinctTestCases(1), Trials: 1}, true},
		{"ten trials", NewBatch{TestCaseIDs: distinctTestCases(1), Trials: 10}, true},
		{"zero trials", NewBatch{TestCaseIDs: distinctTestCases(1), Trials: 0}, false},
		{"eleven trials", NewBatch{TestCaseIDs: distinctTestCases(1), Trials: 11}, false},
		{"fifty test cases", NewBatch{TestCaseIDs: distinctTestCases(50), Trials: 1}, true},
		{"fifty-one test cases", NewBatch{TestCaseIDs: distinctTestCases(51), Trials: 1}, false},
		{"no test case", NewBatch{Trials: 1}, false},
		{"the same test case twice", NewBatch{TestCaseIDs: duplicate, Trials: 1}, false},
	}
	for _, c := range cases {
		if got := c.batch.validShape(); got != c.accept {
			t.Errorf("%s: accepted = %v, want %v", c.name, got, c.accept)
		}
	}
}

func TestThePassRateCarriesItsWilsonInterval(t *testing.T) {
	if rate, interval := passRate(0, 0); rate != nil || interval != nil {
		t.Errorf("nothing judged: rate %v interval %v, want neither", rate, interval)
	}
	cases := []struct {
		name            string
		passed, judged  int
		rate, low, high float64
	}{
		{"every trial passed", 10, 10, 1, 0.7225, 1},
		{"none passed", 0, 10, 0, 0, 0.2775},
		{"one of two", 1, 2, 0.5, 0.0945, 0.9055},
	}
	for _, c := range cases {
		rate, interval := passRate(c.passed, c.judged)
		if math.Abs(*rate-c.rate) > 1e-9 || math.Abs(interval.Low-c.low) > 5e-4 || math.Abs(interval.High-c.high) > 5e-4 {
			t.Errorf("%s: rate %v interval %+v, want %v [%v, %v]", c.name, *rate, *interval, c.rate, c.low, c.high)
		}
	}
}

func TestPassAtKAndPassAllKAreUnknownUntilTheTrialsDecideThem(t *testing.T) {
	cases := []struct {
		name                   string
		passed, judged, trials int
		atK, allK              *bool
	}{
		{"every trial passed", 3, 3, 3, new(true), new(true)},
		{"one passed, the rest still running", 1, 1, 3, new(true), nil},
		{"one failed, the rest still running", 0, 1, 3, nil, new(false)},
		{"every trial failed", 0, 3, 3, new(false), new(false)},
		{"nothing judged yet", 0, 0, 3, nil, nil},
	}
	for _, c := range cases {
		if got := passAtK(c.passed, c.judged, c.trials); !sameBool(got, c.atK) {
			t.Errorf("%s: pass@k = %v, want %v", c.name, show(got), show(c.atK))
		}
		if got := passAllK(c.passed, c.judged, c.trials); !sameBool(got, c.allK) {
			t.Errorf("%s: pass^k = %v, want %v", c.name, show(got), show(c.allK))
		}
	}
}

func sameBool(a, b *bool) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }

func show(b *bool) any {
	if b == nil {
		return "unknown"
	}
	return *b
}

func TestATerminalRunAsksForABatchAdvanceOnlyWhereABatchIsUnsettled(t *testing.T) {
	workspace := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	for _, launching := range []bool{false, true} {
		var enqueued []BatchAdvanceArgs
		c := &BatchEventConsumer{
			HasUnsettledBatch: func(context.Context, pgtype.UUID) (bool, error) { return launching, nil },
			Enqueue: func(_ context.Context, args BatchAdvanceArgs) error {
				enqueued = append(enqueued, args)
				return nil
			},
		}
		if err := c.Deliver(context.Background(), outbox.Event{WorkspaceID: workspace, EventType: outbox.RunCancelled}); err != nil {
			t.Fatal(err)
		}
		want := 0
		if launching {
			want = 1
		}
		if len(enqueued) != want || (want == 1 && enqueued[0].WorkspaceID != pgconv.UUIDString(workspace)) {
			t.Errorf("launching=%v: enqueued %+v, want %d advance for the event's workspace", launching, enqueued, want)
		}
	}
	if err := (&BatchEventConsumer{}).Deliver(context.Background(), outbox.Event{}); err == nil {
		t.Error("an unwired consumer delivered silently, want an error")
	}
	failing := &BatchEventConsumer{
		HasUnsettledBatch: func(context.Context, pgtype.UUID) (bool, error) { return false, errors.New("db down") },
		Enqueue:           func(context.Context, BatchAdvanceArgs) error { return nil },
	}
	if err := failing.Deliver(context.Background(), outbox.Event{}); err == nil {
		t.Error("a failed lookup was swallowed, want it returned so the event is retried")
	}
}

func TestThePairedDifferenceCarriesItsStandardErrorInterval(t *testing.T) {
	if mean, interval := pairedDifference(nil); mean != nil || interval != nil {
		t.Errorf("no pair: %v %v, want neither", mean, interval)
	}
	if mean, interval := pairedDifference([]float64{0.5}); mean == nil || *mean != 0.5 || interval != nil {
		t.Errorf("one pair: %v %v, want the difference without an interval", mean, interval)
	}
	mean, interval := pairedDifference([]float64{1, 0, 0.5, -0.5})
	if math.Abs(*mean-0.25) > 1e-9 || math.Abs(interval.Low+0.3826) > 5e-4 || math.Abs(interval.High-0.8826) > 5e-4 {
		t.Errorf("four pairs: mean %v interval %+v, want 0.25 [-0.3826, 0.8826]", *mean, *interval)
	}
}

func TestTheSkillEffectPairsEachTestCaseWithItsOwnTrialsWithoutTheSkill(t *testing.T) {
	cases := distinctTestCases(3)
	outcomes := map[pgtype.UUID]resolvedTrial{}
	var items []gen.EvaluationBatchItem
	trial := func(testCase pgtype.UUID, n int32, withheld bool, outcome TrialOutcome) {
		run := pgtype.UUID{Bytes: [16]byte{byte(len(items) + 1), 9}, Valid: true}
		items = append(items, gen.EvaluationBatchItem{TestCaseID: testCase, Trial: n, SkillWithheld: withheld, RunID: run})
		outcomes[run] = resolvedTrial{end: ChainEnd{RunID: run}, outcome: outcome}
	}
	trial(cases[0], 1, false, TrialMet)
	trial(cases[0], 2, false, TrialMet)
	trial(cases[1], 1, false, TrialNotMet)
	trial(cases[1], 2, false, TrialMet)
	trial(cases[2], 1, false, TrialMet)
	trial(cases[2], 2, false, TrialMet)
	trial(cases[0], 1, true, TrialMet)
	trial(cases[0], 2, true, TrialNotMet)
	trial(cases[1], 1, true, TrialNotMet)
	trial(cases[1], 2, true, TrialNotMet)
	trial(cases[2], 1, true, TrialRunning)
	trial(cases[2], 2, true, TrialPending)

	withSkill, withheld := splitByArm(items)
	with, without := armReport(withSkill, outcomes, 2), armReport(withheld, outcomes, 2)
	if with.Passed != 5 || with.Judged != 6 || without.Passed != 1 || without.Judged != 4 {
		t.Fatalf("arms = %d/%d with and %d/%d without, want 5/6 and 1/4", with.Passed, with.Judged, without.Passed, without.Judged)
	}
	effect := pairArms(with, without)
	if effect.Paired != 2 || effect.Unpaired != 1 || effect.Better != 2 || effect.Worse != 0 || effect.Tied != 0 {
		t.Errorf("effect = %+v, want two pairs both better and the unjudged test case unpaired", effect)
	}
	if effect.Difference == nil || math.Abs(*effect.Difference-0.5) > 1e-9 {
		t.Errorf("difference = %v, want 0.5", effect.Difference)
	}
}

func TestATrialReadsAsItsVerdictOnceJudgedAndOtherwiseByHowItsRunEnded(t *testing.T) {
	asked := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	s := &BatchService{Ports: BatchPorts{ReadQuestionCount: func(_ context.Context, _, runID pgtype.UUID) (int, error) {
		if runID == asked {
			return 1, nil
		}
		return 0, nil
	}}}
	succeeded := ChainEnd{Terminal: true, Succeeded: true}
	cases := []struct {
		name      string
		end       ChainEnd
		verdict   Verdict
		evaluated bool
		want      TrialOutcome
	}{
		{"the run is still going", ChainEnd{}, Verdict{}, false, TrialRunning},
		{"the judge decided", succeeded, Verdict{Judged: true, Overall: "partially_met"}, true, TrialPartiallyMet},
		{"the judging failed", succeeded, Verdict{Failed: true}, true, TrialNotEvaluated},
		{"the judging is under way", succeeded, Verdict{}, true, TrialRunning},
		{"a finished run that asked a question", ChainEnd{Terminal: true, Succeeded: true, RunID: asked}, Verdict{}, false, TrialAwaitingAnswer},
		{"a finished run that asked nothing waits for its judging", succeeded, Verdict{}, false, TrialRunning},
		{"a failed run waits for its judging", ChainEnd{Terminal: true, Failed: true}, Verdict{}, false, TrialRunning},
		{"a cancelled run is never judged", ChainEnd{Terminal: true}, Verdict{}, false, TrialNotEvaluated},
	}
	for _, c := range cases {
		got, err := s.trialOutcome(context.Background(), pgtype.UUID{}, c.end, c.verdict, c.evaluated)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q %v, want %q", c.name, got, err, c.want)
		}
	}
}

func TestEachPairedTestCaseCountsOnceAsBetterWorseOrTied(t *testing.T) {
	cases := distinctTestCases(4)
	candidate := ArmReport{TestCases: []TestCaseResult{
		{TestCaseID: cases[0], Passed: 2, Judged: 2},
		{TestCaseID: cases[1], Passed: 0, Judged: 2},
		{TestCaseID: cases[2], Passed: 1, Judged: 2},
		{TestCaseID: cases[3], Passed: 1, Judged: 1},
	}}
	reference := ArmReport{TestCases: []TestCaseResult{
		{TestCaseID: cases[0], Passed: 1, Judged: 2},
		{TestCaseID: cases[1], Passed: 2, Judged: 2},
		{TestCaseID: cases[2], Passed: 1, Judged: 2},
	}}
	p := pairArms(candidate, reference)
	if p.Paired != 3 || p.Unpaired != 1 || p.Better != 1 || p.Worse != 1 || p.Tied != 1 {
		t.Errorf("pairing = %+v, want one better, one worse, one tied and one unpaired", p)
	}
	if p.Difference == nil || math.Abs(*p.Difference+0.1666667) > 1e-6 {
		t.Errorf("difference = %v, want -0.1667 (the reference wins by more than it loses)", p.Difference)
	}
}
