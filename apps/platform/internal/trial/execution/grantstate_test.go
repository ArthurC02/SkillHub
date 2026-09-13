package run_test

import (
	"slices"
	"testing"

	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

var legalGrantMoves = map[run.ObjectGrantState][]run.ObjectGrantState{
	run.ObjectGrantStateUnissued: {run.ObjectGrantStateRecorded, run.ObjectGrantStateClosed},
}

func TestGrantsAdmitExactlyTheMovesTheQueriesCanMake(t *testing.T) {
	if len(run.AllObjectGrantStates()) != 4 {
		t.Fatalf("the grant vocabulary is four states, got %d", len(run.AllObjectGrantStates()))
	}
	for _, from := range run.AllObjectGrantStates() {
		for _, to := range run.AllObjectGrantStates() {
			if from == to {
				continue
			}
			want := slices.Contains(legalGrantMoves[from], to)
			if got := run.CanTransitionObjectGrant(from, to); got != want {
				t.Errorf("%s -> %s: table says %v, the queries say %v", from, to, got, want)
			}
		}
	}
}

func TestOnlyAnUnissuedAttemptCanStillRecordAnExpiry(t *testing.T) {
	if !run.CanTransitionObjectGrant(run.ObjectGrantStateUnissued, run.ObjectGrantStateRecorded) {
		t.Fatal("recording an expiry is what an unissued attempt does next")
	}
	for _, from := range []run.ObjectGrantState{run.ObjectGrantStateRecorded, run.ObjectGrantStateClosed} {
		if run.CanTransitionObjectGrant(from, run.ObjectGrantStateRecorded) && from != run.ObjectGrantStateRecorded {
			t.Errorf("%s has already settled; recording an expiry over it would move the purge window", from)
		}
	}
}

func TestALegacyAttemptStaysWhereItIs(t *testing.T) {
	for _, to := range run.AllObjectGrantStates() {
		if to == run.ObjectGrantStateLegacyUnknown {
			continue
		}
		if run.CanTransitionObjectGrant(run.ObjectGrantStateLegacyUnknown, to) {
			t.Errorf("a legacy attempt fails closed and blocks purge; it must not reach %s", to)
		}
	}
}

func TestAClosedGrantIsADeadEnd(t *testing.T) {
	for _, to := range run.AllObjectGrantStates() {
		if to == run.ObjectGrantStateClosed {
			continue
		}
		if run.CanTransitionObjectGrant(run.ObjectGrantStateClosed, to) {
			t.Errorf("closed is the end of the grant lifecycle, yet the table lets it reach %s", to)
		}
	}
}

func TestRewritingTheSameGrantStateIsAllowed(t *testing.T) {
	for _, s := range run.AllObjectGrantStates() {
		if !run.CanTransitionObjectGrant(s, s) {
			t.Errorf("%s: a rewrite that changes nothing must not be refused", s)
		}
	}
}

func TestAGrantStateNobodyDeclaredIsRefused(t *testing.T) {
	for _, bogus := range []string{"", "issued", "Unissued", "legacy-unknown"} {
		if _, ok := run.ParseObjectGrantState(bogus); ok {
			t.Errorf("%q is not a grant state", bogus)
		}
		if run.CanTransitionObjectGrant(run.ObjectGrantState(bogus), run.ObjectGrantStateRecorded) {
			t.Errorf("an attempt sitting on %q must not be advanced", bogus)
		}
	}
	for _, known := range run.AllObjectGrantStates() {
		if _, ok := run.ParseObjectGrantState(string(known)); !ok {
			t.Errorf("%s is declared but ParseObjectGrantState rejects it", known)
		}
	}
}
