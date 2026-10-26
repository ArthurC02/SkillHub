package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func scrapedSeries(t *testing.T) map[string]string {
	t.Helper()
	rec := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics scrape returned %d: %s", rec.Code, rec.Body.String())
	}
	series := map[string]string{}
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if name, value, ok := strings.Cut(line, " "); ok && !strings.HasPrefix(line, "#") {
			series[name] = value
		}
	}
	return series
}

func TestEveryWorkerReplicaRefreshesItsGaugesFromTheDatabase(t *testing.T) {
	pool := requireDB(t)
	fake := providertest.New("fake_gauge_sandbox", "test-token")
	t.Cleanup(fake.Close)
	set, err := worker.BuildWorkers(pool, worker.Deps{Providers: run.NewRegistry(fake.Provider())})
	if err != nil {
		t.Fatal(err)
	}

	const staleValue = "987654"
	metrics.CleanupBacklog.WithLabelValues().Set(987654)
	metrics.OutboxDeadLetteredCurrent.WithLabelValues().Set(987654)
	metrics.OrphanPersistent.WithLabelValues("fake_gauge_sandbox").Set(987654)
	metrics.ObjectsMissing.WithLabelValues("dataset").Set(987654)
	metrics.BacklogOldestSeconds.WithLabelValues(metrics.BacklogEnrichment).Set(987654)
	metrics.DispatchHalted.WithLabelValues("lifted-elsewhere", "orphan_threshold").Set(987654)

	requireWorkerGaugeRefresh(t, set)

	scraped := scrapedSeries(t)
	for _, series := range []string{
		`skillhub_run_cleanup_backlog`,
		`skillhub_outbox_dead_lettered`,
		`skillhub_orphan_sandbox_persistent{provider="fake_gauge_sandbox"}`,
		`skillhub_storage_objects_missing{resource_kind="dataset"}`,
		`skillhub_backlog_oldest_seconds{backlog="enrichment"}`,
		`skillhub_dispatch_halted{source="orphan_threshold",target="lifted-elsewhere"}`,
	} {
		if scraped[series] == staleValue {
			t.Errorf("%s still reports %s, a value this replica never read from the database", series, staleValue)
		}
	}
}

func TestWorkerBacklogObservationsDisappearAfterDatabaseFailureAndRecover(t *testing.T) {
	pool := requireDB(t)
	metrics.BacklogOldestSeconds.Reset()
	t.Cleanup(metrics.BacklogOldestSeconds.Reset)
	ctx := context.Background()
	failedPool, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(failedPool.Close)
	fake := providertest.New("backlog_freshness_sandbox", "test-token")
	t.Cleanup(fake.Close)
	build := func(pool *pgxpool.Pool) *worker.Set {
		set, err := worker.BuildWorkers(pool, worker.Deps{Providers: run.NewRegistry(fake.Provider())})
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	set := build(failedPool)
	requireWorkerGaugeRefresh(t, set)
	series := []string{
		`skillhub_backlog_oldest_seconds{backlog="orphan_objects"}`,
		`skillhub_backlog_oldest_seconds{backlog="source_checks"}`,
		`skillhub_backlog_oldest_seconds{backlog="enrichment"}`,
	}
	initial := scrapedSeries(t)
	for _, name := range series {
		if _, present := initial[name]; !present {
			t.Fatalf("initial observation missing %s", name)
		}
	}
	failedPool.Close()
	failures := 0
	for _, publish := range set.Gauges {
		if err := publish(ctx); err != nil {
			failures++
		}
	}
	if failures == 0 {
		t.Fatal("closed database pool produced no observation errors")
	}
	failed := scrapedSeries(t)
	for _, name := range series {
		if value, present := failed[name]; present {
			t.Fatalf("unreadable observation %s still publishes %s", name, value)
		}
	}
	requireWorkerGaugeRefresh(t, build(pool))
	recovered := scrapedSeries(t)
	for _, name := range series {
		if _, present := recovered[name]; !present {
			t.Fatalf("recovered observation missing %s", name)
		}
	}
}

func requireWorkerGaugeRefresh(t *testing.T, set *worker.Set) {
	t.Helper()
	for _, publish := range set.Gauges {
		if err := publish(context.Background()); err != nil {
			t.Fatalf("refreshing a gauge failed: %v", err)
		}
	}
}

func resetOperationalGaugeObservations(t *testing.T) {
	t.Helper()
	for _, gauge := range []*prometheus.GaugeVec{
		metrics.CleanupBacklog, metrics.OutboxDeadLetteredCurrent,
		metrics.OrphanPersistent, metrics.ObjectsMissing,
		metrics.PersistentOrphanObservationSuccess, metrics.DispatchHaltObservationSuccess,
		metrics.DispatchHalted,
	} {
		gauge.Reset()
		t.Cleanup(gauge.Reset)
	}
}

func TestOperationalObservationsWithdrawUnreadableCountsAndRecover(t *testing.T) {
	pool := requireDB(t)
	for _, tc := range []struct {
		name  string
		value float64
	}{{"cached zero", 0}, {"cached positive", 7}} {
		t.Run(tc.name, func(t *testing.T) {
			testOperationalObservationRecovery(t, pool, tc.value)
		})
	}
}

func testOperationalObservationRecovery(t *testing.T, pool *pgxpool.Pool, cached float64) {
	t.Helper()
	resetOperationalGaugeObservations(t)
	ctx := context.Background()
	failedPool, err := pgxpool.NewWithConfig(ctx, pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(failedPool.Close)
	fake := providertest.New("observation_sandbox", "test-token")
	t.Cleanup(fake.Close)
	build := func(pool *pgxpool.Pool) *worker.Set {
		set, err := worker.BuildWorkers(pool, worker.Deps{Providers: run.NewRegistry(fake.Provider())})
		if err != nil {
			t.Fatal(err)
		}
		return set
	}
	set := build(failedPool)
	requireWorkerGaugeRefresh(t, set)
	names := []string{
		`skillhub_run_cleanup_backlog`, `skillhub_outbox_dead_lettered`,
		`skillhub_orphan_sandbox_persistent{provider="observation_sandbox"}`,
		`skillhub_storage_objects_missing{resource_kind="dataset"}`,
		`skillhub_storage_objects_missing{resource_kind="artifact"}`,
	}
	initial := scrapedSeries(t)
	for _, name := range names {
		if _, present := initial[name]; !present {
			t.Fatalf("initial observation missing %s", name)
		}
	}
	metrics.CleanupBacklog.WithLabelValues().Set(cached)
	metrics.OutboxDeadLetteredCurrent.WithLabelValues().Set(cached)
	metrics.OrphanPersistent.WithLabelValues("observation_sandbox").Set(cached)
	metrics.ObjectsMissing.WithLabelValues("dataset").Set(cached)
	metrics.ObjectsMissing.WithLabelValues("artifact").Set(cached)
	metrics.OrphanPersistent.WithLabelValues("other_sandbox").Set(9)
	metrics.PersistentOrphanObservationSuccess.WithLabelValues("other_sandbox").Set(1)
	metrics.DispatchHalted.WithLabelValues("last_known_target", "p1_incident").Set(1)
	failedPool.Close()
	for round := 0; round < 2; round++ {
		requireUnreadableOperationalObservations(t, set, names)
	}
	requireWorkerGaugeRefresh(t, build(pool))
	recovered := scrapedSeries(t)
	for _, name := range names {
		if value, present := recovered[name]; !present || value != initial[name] {
			t.Fatalf("recovered %s = %q, present %t; initial %q", name, value, present, initial[name])
		}
	}
	requireOperationalObservationFlags(t, recovered, "1")
	if _, present := recovered[`skillhub_dispatch_halted{source="p1_incident",target="last_known_target"}`]; present {
		t.Fatal("successful halt read retained an incident absent from the database")
	}
}

func requireUnreadableOperationalObservations(t *testing.T, set *worker.Set, names []string) {
	t.Helper()
	for _, publish := range set.Gauges {
		if err := publish(context.Background()); err == nil {
			t.Fatal("closed pool produced a successful gauge read")
		}
	}
	failed := scrapedSeries(t)
	for _, name := range names {
		if value, present := failed[name]; present {
			t.Fatalf("unreadable %s still publishes %s", name, value)
		}
	}
	requireOperationalObservationFlags(t, failed, "0")
	if failed[`skillhub_dispatch_halted{source="p1_incident",target="last_known_target"}`] != "1" {
		t.Fatal("failed halt read cleared the last known incident")
	}
}

func requireOperationalObservationFlags(t *testing.T, series map[string]string, success string) {
	t.Helper()
	for name, want := range map[string]string{
		`skillhub_dispatch_halt_observation_success`:                          success,
		`skillhub_orphan_observation_success{provider="observation_sandbox"}`: success,
		`skillhub_orphan_sandbox_persistent{provider="other_sandbox"}`:        "9",
		`skillhub_orphan_observation_success{provider="other_sandbox"}`:       "1",
	} {
		if value, present := series[name]; !present || value != want {
			t.Fatalf("%s = %q, present %t; want %s", name, value, present, want)
		}
	}
}

type cleanupObservationKey struct{}

type cancelAfterCleanupObservation struct{ cancel context.CancelFunc }

func (c cancelAfterCleanupObservation) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "cleanup_status <> 'cleaned'") {
		return context.WithValue(ctx, cleanupObservationKey{}, true)
	}
	return ctx
}

func (c cancelAfterCleanupObservation) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(cleanupObservationKey{}) == true {
		c.cancel()
	}
}

func TestAReadFailureAfterCleanupStillFailsTheRunObservation(t *testing.T) {
	pool := requireDB(t)
	resetOperationalGaugeObservations(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	config := pool.Config()
	config.ConnConfig.Tracer = cancelAfterCleanupObservation{cancel: cancel}
	observedPool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(observedPool.Close)
	service := &run.Service{Pool: observedPool, Providers: run.NewRegistry()}
	if err := service.PublishGauges(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("want the canceled halt read, got %v", err)
	}
	series := scrapedSeries(t)
	if _, present := series["skillhub_run_cleanup_backlog"]; !present {
		t.Fatal("the completed cleanup observation was discarded")
	}
	if series["skillhub_dispatch_halt_observation_success"] != "0" {
		t.Fatal("the unreadable halt inventory was reported as observed")
	}
}
