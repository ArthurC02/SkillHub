package run_test

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/run"
)

// The Run aggregate's rules, tested without a database: statemachine.go's top half
// is pure so this file can be, too. Everything below is the transition table
// written out a second time, by hand — the aggregate's invariants are worth two
// independent statements of.
//
// legal is that second statement, as pairs. It is deliberately not derived from
// run.successors: a test that reads the table it is checking proves only that the
// table equals itself. Every pair here is one line of ADR-004 / RUN-002 / RUN-004,
// and adding a state to the enum without adding its row makes
// TestTransitionsAreExactlyTheAllowedSet fail.
var legal = map[gen.RunStatus][]gen.RunStatus{
	// The happy path, one step at a time...
	gen.RunStatusQueued:       {gen.RunStatusProvisioning},
	gen.RunStatusProvisioning: {gen.RunStatusPreparing},
	gen.RunStatusPreparing:    {gen.RunStatusRunning},
	gen.RunStatusRunning:      {gen.RunStatusEvaluating},
	// ...and the only way to succeed: after evaluation, never before.
	gen.RunStatusEvaluating: {gen.RunStatusSucceeded},
}

// unhappyTerminals may be reached from any non-terminal state: failure can happen
// anywhere, RUN-004 names all five states as cancellable, and the wall clock
// (PDM-005 §5.2) covers queue wait as well as execution.
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

// RUN-002/RUN-004: the full cross product, so an illegal transition cannot pass
// by being one nobody thought to write a case for. In particular this pins the
// three things the state machine exists to prevent: skipping a state (queued →
// running), rewinding one (running → preparing), and leaving a terminal state
// at all (failed → running, succeeded → cancelled).
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

// Every test here walks run.AllStatuses, so a status missing from that list is a
// status nothing above checks. sqlc generates the run_status constants but no list
// of them, so the size is pinned by hand: growing the enum in db/migrations without
// growing AllStatuses fails here rather than passing everywhere.
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
	// Each status is classified by the transition table one way or the other. A
	// status added to the enum and to AllStatuses but forgotten in the table would
	// silently become terminal - a run could enter it and never leave.
	for _, s := range run.AllStatuses {
		if run.IsTerminal(s) {
			continue
		}
		if len(successorsOf(s)) == 0 {
			t.Errorf("%s is not terminal but has no successors", s)
		}
	}
}

// successorsOf asks the exported rule, not the unexported table.
func successorsOf(from gen.RunStatus) []gen.RunStatus {
	var out []gen.RunStatus
	for _, to := range run.AllStatuses {
		if run.CanTransition(from, to) {
			out = append(out, to)
		}
	}
	return out
}

// A run may not transition to the state it is already in. Re-applying a state
// change is caught by the expected-from-status guard in SQL, where it is a
// conflict; accepting it here would append a phantom row to the run's history.
func TestSelfTransitionsAreIllegal(t *testing.T) {
	for _, s := range run.AllStatuses {
		if run.CanTransition(s, s) {
			t.Errorf("CanTransition(%s, %s) = true, want false", s, s)
		}
	}
}

// Terminal states are terminal: nothing leaves them, and the 0005 trigger says
// the same thing in the database.
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

// Every non-terminal state must be able to reach a terminal one, or a run could
// get stuck forever with no way for cancel or timeout to end it (RUN-004).
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
