package eval

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
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

func TestATerminalRunAsksForABatchAdvanceOnlyWhereABatchIsLaunching(t *testing.T) {
	workspace := pgtype.UUID{Bytes: [16]byte{7}, Valid: true}
	for _, launching := range []bool{false, true} {
		var enqueued []BatchAdvanceArgs
		c := &BatchEventConsumer{
			HasLaunchingBatch: func(context.Context, pgtype.UUID) (bool, error) { return launching, nil },
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
		HasLaunchingBatch: func(context.Context, pgtype.UUID) (bool, error) { return false, errors.New("db down") },
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
