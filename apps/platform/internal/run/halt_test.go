package run

// The arithmetic and the reading of the SEC-012 / ADR-022 X-04 switch, with no
// database. The database-backed behaviour — both entry points, the preserved
// scene, the audit trail and the automatic recovery — is
// internal/identity/dispatch_halt_integration_test.go.

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// ADR-022 X-04 §6a, with the worked examples the ADR itself gives: 「單節點 2 slot
// × 50% ＝ 1 筆」, 「套進早期成長：20 slot × 25% ＝ 5 筆」, and the floor of 2 on the
// pool that exists because 「純 25% 在 4 slot 下等於 1 筆，一次洩漏就停掉整個平台
// ——那不是保守，是把一個容量事件升級成全平台事故」.
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
		// 25% of 4 is 1, and the floor is what stops one leak from taking the
		// platform down in the closed beta's own capacity.
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
	return gen.DispatchHalt{Provider: provider, Source: HaltSourceIncident}
}

func threshold(provider string) gen.DispatchHalt {
	return gen.DispatchHalt{Provider: provider, Source: HaltSourceOrphanThreshold}
}

// 02:SEC-010 action ③: a pool-wide P1 preserves every node's scene, a node-scoped
// one preserves only its own, and a capacity pause preserves nothing — cleanup is
// what clears the leaks that raised it.
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

// "All the nodes are drained" and "the pool is paused" are the same operational
// fact, and answering them differently is exactly the split view 03:SEC-012 is
// about.
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
		// A deployment with no sandbox at all is an existing state with existing
		// behaviour (the run is accepted and fails saying so). It is not this switch.
		{"no providers configured", haltsOf(), &Registry{}, false},
	} {
		if got := tc.state.dispatchPaused(tc.registry); got != tc.want {
			t.Errorf("%s: dispatchPaused = %v, want %v", tc.what, got, tc.want)
		}
	}
}
