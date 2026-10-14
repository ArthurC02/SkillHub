package halt

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestDispatchHaltViewCarriesOperatorFacingFacts(t *testing.T) {
	declared := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	row := gen.DispatchHalt{
		ID:          pgtype.UUID{Bytes: [16]byte{1}, Valid: true},
		Provider:    "sandbox",
		Source:      string(Incident),
		Reason:      "containment",
		DeclaredAt:  pgtype.Timestamptz{Time: declared, Valid: true},
		ClearRounds: 2,
	}

	got := haltOf(row)
	if got.ID != row.ID || got.Provider != "sandbox" || got.Source != Incident ||
		got.Reason != "containment" || got.DeclaredAt == nil || !got.DeclaredAt.Equal(declared) || got.ClearRounds != 2 {
		t.Errorf("dispatch halt = %+v", got)
	}
}

func haltsOf(entries ...gen.DispatchHalt) State {
	return StateOf(entries...)
}

func incident(provider string) gen.DispatchHalt {
	return gen.DispatchHalt{Provider: provider, Source: string(Incident)}
}

func threshold(provider string) gen.DispatchHalt {
	return gen.DispatchHalt{Provider: provider, Source: string(OrphanThreshold)}
}

func TestIncidentHeldCoversTheRightNodes(t *testing.T) {
	for _, tc := range []struct {
		what     string
		state    State
		provider string
		want     bool
	}{
		{"nothing halted", haltsOf(), "node_a", false},
		{"pool P1 covers every node", haltsOf(incident(Pool)), "node_a", true},
		{"pool P1 covers the pool itself", haltsOf(incident(Pool)), Pool, true},
		{"node P1 covers its own node", haltsOf(incident("node_a")), "node_a", true},
		{"node P1 leaves the others alone", haltsOf(incident("node_a")), "node_b", false},
		{"a node P1 is not a pool P1", haltsOf(incident("node_a")), Pool, false},
		{"a capacity pause never holds teardown", haltsOf(threshold(Pool)), "node_a", false},
	} {
		if got := tc.state.IncidentHeld(tc.provider); got != tc.want {
			t.Errorf("%s: incidentHeld(%q) = %v, want %v", tc.what, tc.provider, got, tc.want)
		}
	}
}

func TestIncidentPaused(t *testing.T) {
	two := []string{"node_a", "node_b"}
	for _, tc := range []struct {
		what      string
		state     State
		providers []string
		want      bool
	}{
		{"healthy fleet", haltsOf(), two, false},
		{"pool-wide incident", haltsOf(incident(Pool)), two, true},
		{"pool-wide threshold-only", haltsOf(threshold(Pool)), two, false},
		{"one node incident-held, one healthy", haltsOf(incident("node_a")), two, false},
		{"every node incident-held, alongside an unrelated threshold-held node", haltsOf(incident("node_a"), incident("node_b"), threshold("node_c")), two, true},
		{"one node threshold-held only, no incident anywhere", haltsOf(threshold("node_a")), two, false},
	} {
		if got := tc.state.IncidentPaused(tc.providers); got != tc.want {
			t.Errorf("%s: incidentPaused = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func TestDispatchPaused(t *testing.T) {
	two := []string{"node_a", "node_b"}
	for _, tc := range []struct {
		what      string
		state     State
		providers []string
		want      bool
	}{
		{"healthy fleet", haltsOf(), two, false},
		{"pool halt", haltsOf(threshold(Pool)), two, true},
		{"one node drained, one left", haltsOf(threshold("node_a")), two, false},
		{"every node drained", haltsOf(threshold("node_a"), incident("node_b")), two, true},

		{"no providers configured", haltsOf(), nil, false},
	} {
		if got := tc.state.DispatchPaused(tc.providers); got != tc.want {
			t.Errorf("%s: dispatchPaused = %v, want %v", tc.what, got, tc.want)
		}
	}
}

func TestAHaltDeclarationNeedsAKnownSourceAndAReason(t *testing.T) {
	for _, tc := range []struct {
		what   string
		source Source
		reason string
		want   error
	}{
		{"an incident with a reason", Incident, "masker stopped", nil},
		{"a capacity pause with a reason", OrphanThreshold, "leaks at threshold", nil},
		{"no reason", Incident, "", ErrReasonRequired},
		{"a reason of only whitespace", OrphanThreshold, " \t\n", ErrReasonRequired},
		{"a source nobody declared", Source("maintenance"), "planned work", ErrUnknownSource},
	} {
		if _, err := newDeclaration(tc.source, tc.reason, pgtype.UUID{}); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", tc.what, err, tc.want)
		}
	}
}

func TestARedeclarationNeverDowngradesAnIncident(t *testing.T) {
	operator := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	responder := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	active := func(source Source) gen.DispatchHalt {
		return gen.DispatchHalt{Provider: "node_a", Source: string(source), Reason: "first", DeclaredBy: operator, ClearRounds: 1}
	}
	for _, tc := range []struct {
		what        string
		active      Source
		declared    Source
		wantSource  Source
		wantReason  string
		wantDeclare pgtype.UUID
	}{
		{"an incident takes over a capacity pause", OrphanThreshold, Incident, Incident, "second", responder},
		{"a second incident restates the reason and names its declarer", Incident, Incident, Incident, "second", responder},
		{"a capacity pause cannot overwrite an incident", Incident, OrphanThreshold, Incident, "first", operator},
		{"a capacity pause restates its own reason and keeps its declarer", OrphanThreshold, OrphanThreshold, OrphanThreshold, "second", operator},
	} {
		declaration, err := newDeclaration(tc.declared, "second", responder)
		if err != nil {
			t.Fatal(err)
		}
		got := declaration.over(active(tc.active))
		if Source(got.Source) != tc.wantSource || got.Reason != tc.wantReason || got.DeclaredBy != tc.wantDeclare {
			t.Errorf("%s: got source=%s reason=%q declared_by=%v", tc.what, got.Source, got.Reason, got.DeclaredBy)
		}
		if got.ClearRounds != 0 {
			t.Errorf("%s: clear rounds = %d, want the recovery clock restarted", tc.what, got.ClearRounds)
		}
	}
}

func TestOnlyACapacityPauseRecoversAutomatically(t *testing.T) {
	if got := AutomaticallyRecoveringSources(); len(got) != 1 || got[0] != OrphanThreshold {
		t.Fatalf("automatically recovering sources = %v, want only the orphan threshold", got)
	}
	if Incident.RecoversAutomatically() {
		t.Fatal("a P1 incident must never lift itself")
	}
}

func TestHaltingOrResumingWithoutAReasonIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	var unwired Ledger
	if _, err := unwired.Declare(t.Context(), Pool, Incident, "  ", pgtype.UUID{}); !errors.Is(err, ErrReasonRequired) {
		t.Errorf("declaring without a reason: err = %v, want ErrReasonRequired", err)
	}
	if _, lifted, err := unwired.Lift(t.Context(), Pool, "", pgtype.UUID{}, AllSources()); lifted || !errors.Is(err, ErrReasonRequired) {
		t.Errorf("lifting without a reason: lifted=%v err=%v, want ErrReasonRequired", lifted, err)
	}
}
