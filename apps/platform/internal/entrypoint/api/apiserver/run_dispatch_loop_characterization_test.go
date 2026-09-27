package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestADispatchThatOutlivesItsWaitLimitBetweenAttemptsTimesOut(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-late-between-attempts")
	ctx := context.Background()
	created := f.start(t)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	var late atomic.Bool
	fake.DispatchStatuses = []int{http.StatusServiceUnavailable, http.StatusServiceUnavailable, http.StatusServiceUnavailable}
	fake.OnDispatch = func(string, int) { late.Store(true) }
	svc := *a.runs
	svc.Providers = run.NewRegistry(fake.Provider())
	svc.Store = a.packages
	svc.MaxAttempts = 3
	svc.Now = func() time.Time {
		if late.Load() {
			return time.Now().Add(2 * time.Hour)
		}
		return time.Now()
	}

	if err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("drive: %v", err)
	}
	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusTimedOut) || view.FailureClass.Value != "timeout" || !strings.Contains(view.StatusReason, "排隊") {
		t.Fatalf("run is %q/%q (%s), want timed_out on the queue wait limit", view.Status, view.FailureClass.Value, view.StatusReason)
	}
	if len(view.Attempts) != 1 {
		t.Errorf("attempts = %d, want 1: no attempt is started once the wait limit has passed", len(view.Attempts))
	}
}

func TestARunEveryProviderRefusedForCapacityGoesBackToWaitForASlot(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-all-refused-capacity")
	ctx := context.Background()
	created := f.start(t)
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	fake.DispatchStatuses = []int{http.StatusTooManyRequests}
	svc := *a.runs
	svc.Providers = run.NewRegistry(fake.Provider())
	svc.Store = a.packages

	err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID))
	if !errors.Is(err, run.ErrTryAgainLater) {
		t.Fatalf("drive returned %v, want the run sent back to wait for a slot", err)
	}
	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusQueued) || len(view.Attempts) != 1 {
		t.Errorf("run is %q with %d attempts, want queued with the one refused attempt", view.Status, len(view.Attempts))
	}
}

func TestTheWallClockStartsAtTheDispatchThisDriveMade(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-clock-from-dispatch")
	ctx := context.Background()
	created := f.start(t)
	if _, err := pool.Exec(ctx, `UPDATE runs SET policy_snapshot = jsonb_set(policy_snapshot,
		'{resource_limits,wall_clock_hard_seconds}', '1') WHERE id = $1`, mustUUID(t, created.RunID)); err != nil {
		t.Fatal(err)
	}
	fake := providertest.New("fake_sandbox", "test-token")
	t.Cleanup(fake.Close)
	fake.Plan = providertest.Plan{StuckRunning: true}
	svc := *a.runs
	svc.Providers = run.NewRegistry(fake.Provider())
	svc.Store = a.packages
	svc.Now = func() time.Time { return time.Now().Add(time.Minute) }

	if err := svc.Drive(ctx, mustUUID(t, f.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("drive: %v", err)
	}
	_, view := f.getRun(t, created.RunID)
	if view.Status != string(gen.RunStatusTimedOut) || !strings.Contains(view.StatusReason, "硬性時間上限") {
		t.Fatalf("run is %q (%s), want timed_out on the 1s wall clock counted from its dispatch", view.Status, view.StatusReason)
	}
}

func TestAProviderThatRefusedForCapacityIsAskedAfreshForItsCapacity(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	first := newFixture(t, a, pool, "alice-capacity-refetch")
	second := newFixture(t, a, pool, "bob-capacity-refetch")
	clearRunBacklog(t, pool)
	ctx := context.Background()
	busy, spare := providertest.New("busy_sandbox", "test-token"), providertest.New("spare_sandbox", "test-token")
	t.Cleanup(busy.Close)
	t.Cleanup(spare.Close)
	busy.SetFreeSlots(4)
	spare.SetFreeSlots(4)
	busy.DispatchStatuses = []int{http.StatusTooManyRequests}
	target, err := url.Parse(busy.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var capabilityReads atomic.Int32
	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/capability" {
			capabilityReads.Add(1)
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(front.Close)
	registry := run.NewRegistry(run.NewProvider(busy.Name, front.URL, busy.Token), spare.Provider())
	registry.TTL = time.Hour
	a.runs.Providers = registry
	svc := *a.runs
	svc.Providers = registry
	svc.Store = a.packages

	firstRun := first.start(t).RunID
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, first.workspaceID), mustUUID(t, firstRun)); err != nil {
		t.Fatal(err)
	}
	readsAfterRefusal := capabilityReads.Load()
	secondRun := second.start(t).RunID
	if err := driveThroughPolls(ctx, svc.Drive, mustUUID(t, second.workspaceID), mustUUID(t, secondRun)); err != nil {
		t.Fatal(err)
	}
	if got := capabilityReads.Load(); got <= readsAfterRefusal {
		t.Errorf("busy_sandbox's capability was read %d times before and %d after the next placement; "+
			"a provider that refused for capacity must not be trusted from cache", readsAfterRefusal, got)
	}
}
