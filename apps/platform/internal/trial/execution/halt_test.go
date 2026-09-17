package run

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestNodeAndPoolThresholds(t *testing.T) {
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{
		{"closed beta node, 2 slots at 50%", 2, 1},
		{"4 slots at 50%", 4, 2},
		{"early growth node, 8 slots at 50%", 8, 4},
		{"an undeclared or unreachable node falls back to the floor", 0, 1},

		{"3 slots at 50% rounds up, and the floor of 1 cannot hide it", 3, 2},
	} {
		if got := haltThreshold(tc.slots, 1, 2, 1); got != tc.want {
			t.Errorf("node threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
	for _, tc := range []struct {
		what  string
		slots int
		want  int64
	}{

		{"closed beta pool, 4 slots at 25% under the floor", 4, 2},
		{"early growth pool, 20 slots at 25%", 20, 5},
		{"6 slots at 25% rounds up to a whole resource", 6, 2},
		{"no slots declared anywhere", 0, 2},
	} {
		if got := haltThreshold(tc.slots, 1, 4, 2); got != tc.want {
			t.Errorf("pool threshold, %s: got %d, want %d", tc.what, got, tc.want)
		}
	}
}

func haltsOf(entries ...gen.DispatchHalt) haltState {
	state := haltState{byTarget: map[string]gen.DispatchHalt{}}
	for _, e := range entries {
		state.byTarget[e.Provider] = e
	}
	return state
}

func incident(provider string) gen.DispatchHalt {
	return gen.DispatchHalt{Provider: provider, Source: string(HaltSourceIncident)}
}

func threshold(provider string) gen.DispatchHalt {
	return gen.DispatchHalt{Provider: provider, Source: string(HaltSourceOrphanThreshold)}
}

func TestIncidentHeldCoversTheRightNodes(t *testing.T) {
	for _, tc := range []struct {
		what     string
		state    haltState
		provider string
		want     bool
	}{
		{"nothing halted", haltsOf(), "node_a", false},
		{"pool P1 covers every node", haltsOf(incident(haltPool)), "node_a", true},
		{"pool P1 covers the pool itself", haltsOf(incident(haltPool)), haltPool, true},
		{"node P1 covers its own node", haltsOf(incident("node_a")), "node_a", true},
		{"node P1 leaves the others alone", haltsOf(incident("node_a")), "node_b", false},
		{"a node P1 is not a pool P1", haltsOf(incident("node_a")), haltPool, false},
		{"a capacity pause never holds teardown", haltsOf(threshold(haltPool)), "node_a", false},
	} {
		if got := tc.state.incidentHeld(tc.provider); got != tc.want {
			t.Errorf("%s: incidentHeld(%q) = %v, want %v", tc.what, tc.provider, got, tc.want)
		}
	}
}

func TestIncidentPaused(t *testing.T) {
	two := &Registry{Providers: []*Provider{
		NewProvider("node_a", "http://a", ""),
		NewProvider("node_b", "http://b", ""),
	}}
	for _, tc := range []struct {
		what     string
		state    haltState
		registry *Registry
		want     bool
	}{
		{"healthy fleet", haltsOf(), two, false},
		{"pool-wide incident", haltsOf(incident(haltPool)), two, true},
		{"pool-wide threshold-only", haltsOf(threshold(haltPool)), two, false},
		{"one node incident-held, one healthy", haltsOf(incident("node_a")), two, false},
		{"every node incident-held, alongside an unrelated threshold-held node", haltsOf(incident("node_a"), incident("node_b"), threshold("node_c")), two, true},
		{"one node threshold-held only, no incident anywhere", haltsOf(threshold("node_a")), two, false},
	} {
		if got := tc.state.incidentPaused(tc.registry); got != tc.want {
			t.Errorf("%s: incidentPaused = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func TestDispatchPaused(t *testing.T) {
	two := &Registry{Providers: []*Provider{
		NewProvider("node_a", "http://a", ""),
		NewProvider("node_b", "http://b", ""),
	}}
	for _, tc := range []struct {
		what     string
		state    haltState
		registry *Registry
		want     bool
	}{
		{"healthy fleet", haltsOf(), two, false},
		{"pool halt", haltsOf(threshold(haltPool)), two, true},
		{"one node drained, one left", haltsOf(threshold("node_a")), two, false},
		{"every node drained", haltsOf(threshold("node_a"), incident("node_b")), two, true},

		{"no providers configured", haltsOf(), &Registry{}, false},
	} {
		if got := tc.state.dispatchPaused(tc.registry); got != tc.want {
			t.Errorf("%s: dispatchPaused = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func TestAHaltDeclarationNeedsAKnownSourceAndAReason(t *testing.T) {
	for _, tc := range []struct {
		what   string
		source HaltSource
		reason string
		want   error
	}{
		{"an incident with a reason", HaltSourceIncident, "masker stopped", nil},
		{"a capacity pause with a reason", HaltSourceOrphanThreshold, "leaks at threshold", nil},
		{"no reason", HaltSourceIncident, "", ErrHaltReasonRequired},
		{"a reason of only whitespace", HaltSourceOrphanThreshold, " \t\n", ErrHaltReasonRequired},
		{"a source nobody declared", HaltSource("maintenance"), "planned work", ErrUnknownHaltSource},
	} {
		if _, err := newHaltDeclaration(tc.source, tc.reason, pgtype.UUID{}); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.what, err, tc.want)
		}
	}
}

func TestARedeclarationNeverDowngradesAnIncident(t *testing.T) {
	operator := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	responder := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	active := func(source HaltSource) gen.DispatchHalt {
		return gen.DispatchHalt{Provider: "node_a", Source: string(source), Reason: "first", DeclaredBy: operator, ClearRounds: 1}
	}
	for _, tc := range []struct {
		what        string
		active      HaltSource
		declared    HaltSource
		wantSource  HaltSource
		wantReason  string
		wantDeclare pgtype.UUID
	}{
		{"an incident takes over a capacity pause", HaltSourceOrphanThreshold, HaltSourceIncident, HaltSourceIncident, "second", responder},
		{"a second incident restates the reason and names its declarer", HaltSourceIncident, HaltSourceIncident, HaltSourceIncident, "second", responder},
		{"a capacity pause cannot overwrite an incident", HaltSourceIncident, HaltSourceOrphanThreshold, HaltSourceIncident, "first", operator},
		{"a capacity pause restates its own reason and keeps its declarer", HaltSourceOrphanThreshold, HaltSourceOrphanThreshold, HaltSourceOrphanThreshold, "second", operator},
	} {
		declaration, err := newHaltDeclaration(tc.declared, "second", responder)
		if err != nil {
			t.Fatal(err)
		}
		got := declaration.over(active(tc.active))
		if HaltSource(got.Source) != tc.wantSource || got.Reason != tc.wantReason || got.DeclaredBy != tc.wantDeclare {
			t.Errorf("%s: got source=%s reason=%q declared_by=%v", tc.what, got.Source, got.Reason, got.DeclaredBy)
		}
		if got.ClearRounds != 0 {
			t.Errorf("%s: clear rounds = %d, want the recovery clock restarted", tc.what, got.ClearRounds)
		}
	}
}

func TestOnlyACapacityPauseRecoversAutomatically(t *testing.T) {
	if got := automaticallyRecoveringSources(); len(got) != 1 || got[0] != HaltSourceOrphanThreshold {
		t.Fatalf("automatically recovering sources = %v, want only the orphan threshold", got)
	}
	if HaltSourceIncident.RecoversAutomatically() {
		t.Fatal("a P1 incident must never lift itself")
	}
}

func TestHaltingOrResumingWithoutAReasonIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	var unwired Service
	if _, err := unwired.DeclareHalt(t.Context(), haltPool, HaltSourceIncident, "  ", pgtype.UUID{}); !errors.Is(err, ErrHaltReasonRequired) {
		t.Errorf("declaring without a reason: err = %v, want ErrHaltReasonRequired", err)
	}
	if _, lifted, err := unwired.LiftHalt(t.Context(), haltPool, "", pgtype.UUID{}, AllHaltSources()); lifted || !errors.Is(err, ErrHaltReasonRequired) {
		t.Errorf("lifting without a reason: lifted=%v err=%v, want ErrHaltReasonRequired", lifted, err)
	}
}

func DefaultRequirements() Requirements {
	return requirementsFromPolicy(defaultPolicy())
}
