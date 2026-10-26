package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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
	metrics.CleanupBacklog.Set(987654)
	metrics.OutboxDeadLetteredCurrent.Set(987654)
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
