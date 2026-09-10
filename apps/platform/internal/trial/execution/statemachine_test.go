package run_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

var legal = map[gen.RunStatus][]gen.RunStatus{

	gen.RunStatusQueued:       {gen.RunStatusProvisioning},
	gen.RunStatusProvisioning: {gen.RunStatusPreparing},
	gen.RunStatusPreparing:    {gen.RunStatusRunning},
	gen.RunStatusRunning:      {gen.RunStatusEvaluating},

	gen.RunStatusEvaluating: {gen.RunStatusSucceeded},
}

var unhappyTerminals = []gen.RunStatus{
	gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
}

func allowed() map[[2]gen.RunStatus]bool {
	set := make(map[[2]gen.RunStatus]bool)
	for from, tos := range legal {
		for _, to := range tos {
			set[[2]gen.RunStatus{from, to}] = true
		}
		for _, to := range unhappyTerminals {
			set[[2]gen.RunStatus{from, to}] = true
		}
	}
	return set
}

func TestTransitionsAreExactlyTheAllowedSet(t *testing.T) {
	want := allowed()
	if len(want) != 20 {
		t.Fatalf("expected 20 legal transitions (5 non-terminal states x 4 exits), got %d", len(want))
	}

	for _, from := range run.AllStatuses {
		for _, to := range run.AllStatuses {
			got := run.CanTransition(from, to)
			if expect := want[[2]gen.RunStatus{from, to}]; got != expect {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, expect)
			}
		}
	}
}

func TestAllStatusesCoversTheEnum(t *testing.T) {
	if len(run.AllStatuses) != 9 {
		t.Fatalf("run.AllStatuses has %d entries, want the 9 values of the run_status enum; "+
			"a new status must be added there, to the transition table, and to `legal` below",
			len(run.AllStatuses))
	}
	seen := map[gen.RunStatus]bool{}
	for _, s := range run.AllStatuses {
		if s == "" {
			t.Error("run.AllStatuses contains the zero status")
		}
		if seen[s] {
			t.Errorf("run.AllStatuses lists %s twice, so the cross product below is not one", s)
		}
		seen[s] = true
	}

	for _, s := range run.AllStatuses {
		if run.IsTerminal(s) {
			continue
		}
		if len(successorsOf(s)) == 0 {
			t.Errorf("%s is not terminal but has no successors", s)
		}
	}
}

func successorsOf(from gen.RunStatus) []gen.RunStatus {
	var out []gen.RunStatus
	for _, to := range run.AllStatuses {
		if run.CanTransition(from, to) {
			out = append(out, to)
		}
	}
	return out
}

func TestSelfTransitionsAreIllegal(t *testing.T) {
	for _, s := range run.AllStatuses {
		if run.CanTransition(s, s) {
			t.Errorf("CanTransition(%s, %s) = true, want false", s, s)
		}
	}
}

func TestTerminalStatesAreDeadEnds(t *testing.T) {
	terminals := map[gen.RunStatus]bool{
		gen.RunStatusSucceeded: true, gen.RunStatusFailed: true,
		gen.RunStatusCancelled: true, gen.RunStatusTimedOut: true,
	}
	for _, s := range run.AllStatuses {
		if run.IsTerminal(s) != terminals[s] {
			t.Errorf("IsTerminal(%s) = %v, want %v", s, run.IsTerminal(s), terminals[s])
		}
		if !terminals[s] {
			continue
		}
		for _, to := range run.AllStatuses {
			if run.CanTransition(s, to) {
				t.Errorf("terminal %s still allows a move to %s", s, to)
			}
		}
	}
}

func TestEveryNonTerminalStateCanBeEnded(t *testing.T) {
	for _, s := range run.AllStatuses {
		if run.IsTerminal(s) {
			continue
		}
		for _, end := range unhappyTerminals {
			if !run.CanTransition(s, end) {
				t.Errorf("%s cannot be ended with %s", s, end)
			}
		}
	}
}

func TestHappyPathIsTheWholeLifecycle(t *testing.T) {
	want := []gen.RunStatus{
		gen.RunStatusProvisioning, gen.RunStatusPreparing,
		gen.RunStatusRunning, gen.RunStatusEvaluating, gen.RunStatusSucceeded,
	}
	got, err := run.HappyPath(gen.RunStatusQueued)
	if err != nil {
		t.Fatalf("HappyPath(queued): %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("HappyPath(queued) = %v, want %v", got, want)
	}

	for i, from := range append([]gen.RunStatus{gen.RunStatusQueued}, want[:len(want)-1]...) {
		got, err := run.HappyPath(from)
		if err != nil {
			t.Fatalf("HappyPath(%s): %v", from, err)
		}
		if !slices.Equal(got, want[i:]) {
			t.Errorf("HappyPath(%s) = %v, want %v", from, got, want[i:])
		}
	}
}

func TestExactlyOneSuccessorIsTheHappyOne(t *testing.T) {
	unhappy := map[gen.RunStatus]bool{}
	for _, s := range unhappyTerminals {
		unhappy[s] = true
	}
	for _, from := range run.AllStatuses {
		next, ok := run.NextOnSuccess(from)
		if run.IsTerminal(from) {
			if ok {
				t.Errorf("NextOnSuccess(%s) = %s, want none: terminal states go nowhere", from, next)
			}
			continue
		}
		var happy []gen.RunStatus
		for _, to := range successorsOf(from) {
			if !unhappy[to] {
				happy = append(happy, to)
			}
		}
		if len(happy) != 1 {
			t.Fatalf("%s has %d non-failure successors (%v), want exactly 1", from, len(happy), happy)
		}
		if !ok || next != happy[0] {
			t.Errorf("NextOnSuccess(%s) = %s/%v, want %s", from, next, ok, happy[0])
		}
	}
}

func TestHappyPathFromATerminalStateIsAnError(t *testing.T) {
	for _, s := range unhappyTerminals {
		path, err := run.HappyPath(s)
		if !errors.Is(err, run.ErrNoHappyPath) {
			t.Errorf("HappyPath(%s) = %v, %v; want ErrNoHappyPath", s, path, err)
		}
	}

	path, err := run.HappyPath(gen.RunStatusSucceeded)
	if err != nil || len(path) != 0 {
		t.Errorf("HappyPath(succeeded) = %v, %v; want no steps and no error", path, err)
	}
}
