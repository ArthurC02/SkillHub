package sandbox

import (
	"context"
	"testing"
)

func TestASandboxThatStartsIsReportedRunningWithItsStartTime(t *testing.T) {
	drv := newP02Driver()
	m := p02Manager(drv)
	t.Cleanup(func() {
		close(drv.done)
		m.Wait()
	})

	run, created, err := m.Create(context.Background(), p02Request())
	if err != nil || !created {
		t.Fatalf("Create = created %v, err %v; want a new run", created, err)
	}
	if run.State != StateRunning || run.StartedAt.IsZero() {
		t.Fatalf("Create returned state %q started at %v, want running with a start time", run.State, run.StartedAt)
	}
	stored, err := m.Get(run.ProviderRunID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StateRunning || !stored.StartedAt.Equal(run.StartedAt) {
		t.Fatalf("stored run is %q started at %v, want running started at %v", stored.State, stored.StartedAt, run.StartedAt)
	}
}
