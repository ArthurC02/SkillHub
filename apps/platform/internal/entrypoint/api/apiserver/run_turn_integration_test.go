package apiserver_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestARunningRunDoesNotHoldAWorkerSoMoreRunsThanWorkersAllStart(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := newFixture(t, a, pool, "alice-no-held-worker")
	bob := newFixture(t, a, pool, "bob-no-held-worker")
	carol := newFixture(t, a, pool, "carol-no-held-worker")
	withProvider(t, a, pool, providertest.Plan{StuckRunning: true})

	started := []struct {
		f     fixture
		runID string
	}{
		{alice, alice.start(t).RunID},
		{bob, bob.start(t).RunID},
		{carol, carol.start(t).RunID},
	}
	for _, s := range started {
		waitForStatus(t, s.f.client, s.runID, string(gen.RunStatusRunning))
	}
}

type turnScene struct {
	ctx               context.Context
	pool              *pgxpool.Pool
	svc               run.Service
	fake              *providertest.Fake
	alice, bob, carol fixture
	bobsQueued        string
	carolsQueuedLater string
}

func newTurnScene(t *testing.T, name string) turnScene {
	t.Helper()
	pool := requireDB(t)
	a := newAPI(t, pool)
	s := turnScene{
		ctx:   context.Background(),
		pool:  pool,
		alice: newFixture(t, a, pool, "alice-"+name),
		bob:   newFixture(t, a, pool, "bob-"+name),
		carol: newFixture(t, a, pool, "carol-"+name),
	}
	clearRunBacklog(t, pool)
	s.fake = providertest.New("fake_sandbox", "test-token")
	t.Cleanup(s.fake.Close)
	s.fake.Plan = providertest.Plan{StuckRunning: true}
	s.svc = *a.runs
	s.svc.Providers = run.NewRegistry(s.fake.Provider())
	s.svc.Providers.TTL = time.Millisecond
	s.svc.Store = a.packages

	held := s.alice.start(t).RunID
	s.fake.SetFreeSlots(1)
	s.expectWaits(t, held, "the run that takes alice's first sandbox")
	if s.fake.Dispatches() != 1 {
		t.Fatalf("precondition: dispatches = %d, want alice holding one sandbox", s.fake.Dispatches())
	}
	s.bobsQueued = s.bob.start(t).RunID
	s.carolsQueuedLater = s.carol.start(t).RunID
	return s
}

func (s turnScene) expectWaits(t *testing.T, runID, what string) {
	t.Helper()
	ws := mustUUID(t, s.bob.workspaceID)
	if runID == s.carolsQueuedLater {
		ws = mustUUID(t, s.carol.workspaceID)
	}
	err := s.svc.Drive(s.ctx, ws, mustUUID(t, runID))
	if err != nil && !errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("driving %s returned %v", what, err)
	}
}

func TestEarlierQueuedRunWinsWhenWorkspacesHoldEqualSandboxes(t *testing.T) {
	s := newTurnScene(t, "fewer-held-first")
	s.fake.SetFreeSlots(1)

	s.expectWaits(t, s.bobsQueued, "bob's earlier run")
	if s.fake.Dispatches() != 2 {
		t.Fatalf("dispatches = %d, want bob's earlier run dispatched", s.fake.Dispatches())
	}

	s.expectWaits(t, s.carolsQueuedLater, "carol's later run")
	if s.fake.Dispatches() != 3 {
		t.Fatalf("dispatches = %d, want carol's run dispatched", s.fake.Dispatches())
	}

}

func TestARunDoesNotYieldWhenMoreSlotsAreFreeThanRunsAheadOfIt(t *testing.T) {
	s := newTurnScene(t, "enough-slots")
	s.fake.SetFreeSlots(2)

	s.expectWaits(t, s.bobsQueued, "bob's run with two slots free")
	if s.fake.Dispatches() != 2 {
		t.Errorf("dispatches = %d, want bob's run dispatched beside carol's waiting one", s.fake.Dispatches())
	}
}

func TestARunDoesNotYieldToARunThatCannotUseTheFreeSlot(t *testing.T) {
	s := newTurnScene(t, "unplaceable-ahead")
	s.fake.SetFreeSlots(1)
	if _, err := s.pool.Exec(s.ctx, `UPDATE runs SET policy_snapshot = jsonb_set(policy_snapshot,
		'{resource_limits,vcpu}', '999') WHERE id = $1`, mustUUID(t, s.bobsQueued)); err != nil {
		t.Fatal(err)
	}

	s.expectWaits(t, s.carolsQueuedLater, "carol's run behind one no provider can place")
	if s.fake.Dispatches() != 2 {
		t.Errorf("dispatches = %d, want carol's run dispatched: bob's needs more vCPU than any slot offers", s.fake.Dispatches())
	}
}
