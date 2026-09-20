package sandbox

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

type reclaimDriver struct {
	Driver
	removed []string
}

func (d *reclaimDriver) Remove(_ context.Context, id string) error {
	d.removed = append(d.removed, id)
	return nil
}

func (d *reclaimDriver) Isolation() IsolationStrength   { return IsolationNone }
func (d *reclaimDriver) InjectsFromGrant() []string     { return nil }
func (d *reclaimDriver) Rootless() bool                 { return true }
func (d *reclaimDriver) DedicatedWorkspacePerRun() bool { return true }
func (d *reclaimDriver) Healthy(context.Context) bool   { return true }

const testRetention = 30 * time.Minute

func managerAt(t *testing.T, now time.Time) (*Manager, *reclaimDriver) {
	t.Helper()
	drv := &reclaimDriver{}
	m := NewManager(drv, Config{Provider: "test", Slots: 1, ResultRetention: testRetention},
		slog.New(slog.DiscardHandler))
	m.now = func() time.Time { return now }
	return m, drv
}

func TestANodeTakesBackASlotNobodyReleased(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		state       RunState
		finishedAgo time.Duration
		wantRemoved bool
	}{
		{"running, however long it has been", StateRunning, 10 * time.Hour, false},
		{"finished a moment ago", StateCompleted, time.Second, false},
		{"finished exactly at the retention", StateCompleted, testRetention, false},
		{"finished one tick past the retention", StateCompleted, testRetention + time.Nanosecond, true},
		{"cancelled long past the retention", StateCancelled, 10 * time.Hour, true},
		{"failed long past the retention", StateFailed, 10 * time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, drv := managerAt(t, now)
			e := &entry{run: ProviderRun{ProviderRunID: "run-1", State: tc.state}}
			if tc.state.Terminal() {
				e.run.FinishedAt = now.Add(-tc.finishedAgo)
			}
			m.runs["run-1"] = e

			m.ReclaimStale()

			removed := len(drv.removed) == 1
			if removed != tc.wantRemoved {
				t.Fatalf("driver removals = %v, want removed=%v", drv.removed, tc.wantRemoved)
			}
			wantFree := 0
			if tc.wantRemoved {
				wantFree = 1
			}
			if got := m.Capability(context.Background()).Availability.ConcurrentRunSlots; got != wantFree {
				t.Errorf("free slots = %d, want %d: a slot the node took back must be offered again", got, wantFree)
			}
		})
	}
}

func TestATerminalRunWithNoFinishTimeIsLeftAlone(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	m, drv := managerAt(t, now)
	m.runs["run-1"] = &entry{run: ProviderRun{ProviderRunID: "run-1", State: StateCompleted}}

	m.ReclaimStale()

	if len(drv.removed) != 0 {
		t.Fatalf("driver removals = %v, want none: without a finish time there is no age to judge", drv.removed)
	}
}
