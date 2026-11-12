package apiserver_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func continuationObservationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := isolatedAPIPool(t)
	reset := func() {
		for _, gauge := range []*prometheus.GaugeVec{metrics.ContinuationChains, metrics.ContinuationRounds, metrics.ContinuationLongest, metrics.ContinuationObservedAt, metrics.ContinuationObservationSuccess} {
			gauge.Reset()
		}
	}
	reset()
	t.Cleanup(reset)
	return pool
}

func observedContinuationRun(t *testing.T, pool *pgxpool.Pool, f fixture, at time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `WITH snap AS (
        INSERT INTO test_case_snapshots (workspace_id,test_case_id,user_prompt,acceptance_criteria,content_hash)
        VALUES ($1,$2,'fixture','[]','chain-fixture') RETURNING id)
        INSERT INTO runs (workspace_id,skill_version_id,test_case_snapshot_id,provider,created_at)
        SELECT $1,$3,snap.id,'fake_sandbox',$4 FROM snap RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID), mustUUID(t, f.versionID), at).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func observedContinuationLink(t *testing.T, pool *pgxpool.Pool, f fixture, parent, child string, at time.Time) {
	t.Helper()
	mustExec(t, pool, `INSERT INTO run_continuations (run_id, workspace_id, continues_run_id, questions, answers, created_at)
        VALUES ($1,$2,$3,'[{"question":"q","why":"w"}]','["a"]',$4)`,
		mustUUID(t, child), mustUUID(t, f.workspaceID), mustUUID(t, parent), at)
}

func continuationSnapshot(t *testing.T, pool *pgxpool.Pool, since, until time.Time) gen.SummarizeContinuationChainsRow {
	t.Helper()
	stats, err := gen.New(pool).SummarizeContinuationChains(context.Background(), gen.SummarizeContinuationChainsParams{
		Since: pgconv.Timestamptz(since), Until: pgconv.Timestamptz(until),
	})
	if err != nil {
		t.Fatal(err)
	}
	return stats
}

func TestContinuationObservationCountsEachRootOnceAcrossWorkspaces(t *testing.T) {
	pool := continuationObservationPool(t)
	a := newAPI(t, pool)
	mine := newFixture(t, a, pool, freshName("chain-owner"))
	other := newFixture(t, a, pool, freshName("chain-other"))
	until := time.Now().Add(time.Minute)
	at := until.Add(-time.Hour)
	seedFinishedRun(t, pool, mine, finishedRun{testCaseID: mine.testCaseID, criteria: "[]", contentHash: "chain-zero"})
	root := observedContinuationRun(t, pool, mine, at)
	child := observedContinuationRun(t, pool, mine, at.Add(time.Minute))
	end := observedContinuationRun(t, pool, mine, at.Add(2*time.Minute))
	observedContinuationLink(t, pool, mine, root, child, at.Add(time.Minute))
	observedContinuationLink(t, pool, mine, child, end, at.Add(2*time.Minute))
	theirRoot := observedContinuationRun(t, pool, other, at)
	theirEnd := observedContinuationRun(t, pool, other, at.Add(time.Minute))
	observedContinuationLink(t, pool, other, theirRoot, theirEnd, at.Add(time.Minute))
	stats := continuationSnapshot(t, pool, until.Add(-24*time.Hour), until)
	if stats.Chains != 3 || stats.Rounds != 3 || stats.Longest != 2 {
		t.Fatalf("chain snapshot = %+v; want three roots, three edges, longest two", stats)
	}
}

func TestContinuationObservationRootWindowIncludesLowerAndExcludesUpper(t *testing.T) {
	pool := continuationObservationPool(t)
	f := newFixture(t, newAPI(t, pool), pool, freshName("chain-boundaries"))
	until := time.Now().UTC().Truncate(time.Microsecond)
	since := until.Add(-24 * time.Hour)
	for _, at := range []time.Time{since.Add(-time.Microsecond), since, until.Add(-time.Microsecond), until, until.Add(time.Microsecond)} {
		observedContinuationRun(t, pool, f, at)
	}
	stats := continuationSnapshot(t, pool, since, until)
	if stats.Chains != 2 || stats.Rounds != 0 || stats.Longest != 0 {
		t.Fatalf("root-window snapshot = %+v; want two isolated roots", stats)
	}
}

func TestContinuationObservationExcludesOldRootsAndFutureEdges(t *testing.T) {
	pool := continuationObservationPool(t)
	f := newFixture(t, newAPI(t, pool), pool, freshName("chain-edge-time"))
	until := time.Now().UTC().Truncate(time.Microsecond)
	at := until.Add(-time.Hour)
	old := observedContinuationRun(t, pool, f, until.Add(-25*time.Hour))
	recent := observedContinuationRun(t, pool, f, at)
	observedContinuationLink(t, pool, f, old, recent, at)
	for i := range 3 {
		root := observedContinuationRun(t, pool, f, at)
		childAt, linkAt := at.Add(time.Minute), at.Add(time.Minute)
		if i == 0 {
			childAt = until
		}
		if i == 1 {
			linkAt = until
		}
		child := observedContinuationRun(t, pool, f, childAt)
		observedContinuationLink(t, pool, f, root, child, linkAt)
	}
	stats := continuationSnapshot(t, pool, until.Add(-24*time.Hour), until)
	if stats.Chains != 3 || stats.Rounds != 1 || stats.Longest != 1 {
		t.Fatalf("edge-time snapshot = %+v; want three recent roots with one observed edge", stats)
	}
}

func assertContinuationObservation(t *testing.T, chains, rounds, longest string, at int64) {
	t.Helper()
	series := scrapedSeries(t)
	for name, want := range map[string]string{
		"skillhub_continuation_chains": chains, "skillhub_continuation_rounds": rounds,
		"skillhub_continuation_longest_rounds": longest, "skillhub_continuation_observation_success": "1",
	} {
		if series[name] != want {
			t.Fatalf("%s = %q; want %s", name, series[name], want)
		}
	}
	observedAt, err := strconv.ParseFloat(series["skillhub_continuation_observed_at_seconds"], 64)
	if err != nil || int64(observedAt) != at {
		t.Fatalf("chain observation time = %v, %v; want %d", observedAt, err, at)
	}
	for name := range series {
		if strings.HasPrefix(name, "skillhub_continuation_") && strings.Contains(name, "{") {
			t.Fatalf("continuation observation exposes dynamic labels: %s", name)
		}
	}
}

func TestContinuationObservationUsesActualWorkerAndDoesNotAccumulateOnReplay(t *testing.T) {
	pool := continuationObservationPool(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, freshName("chain-worker"))
	asking := seedAskingRun(t, pool, f, "Which date?")
	if code, _ := f.postJSON(t, "/runs/"+asking+"/continuations", `{"answers":["Tomorrow"]}`); code != http.StatusCreated {
		t.Fatalf("continue = %d; want created", code)
	}
	if code, _ := f.postJSON(t, "/runs/"+asking+"/continuations", `{"answers":["Tomorrow"]}`); code != http.StatusConflict {
		t.Fatalf("replay = %d; want conflict", code)
	}
	until := time.Now().UTC().Add(time.Second)
	for range 2 {
		set, err := worker.BuildWorkers(pool, worker.Deps{})
		if err != nil {
			t.Fatal(err)
		}
		set.Runs.Now = func() time.Time { return until }
		for range 2 {
			requireWorkerGaugeRefresh(t, set)
			assertContinuationObservation(t, "1", "1", "1", until.Unix())
		}
	}
}

func TestContinuationObservationWithdrawsFailedReadsAndRecovers(t *testing.T) {
	pool := continuationObservationPool(t)
	until := time.Now().UTC().Add(time.Minute)
	service := &run.Service{Pool: pool, Now: func() time.Time { return until }}
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "0", "0", "0", until.Unix())
	f := newFixture(t, newAPI(t, pool), pool, freshName("chain-recovery"))
	root := observedContinuationRun(t, pool, f, until.Add(-time.Hour))
	child := observedContinuationRun(t, pool, f, until.Add(-time.Minute))
	observedContinuationLink(t, pool, f, root, child, until.Add(-time.Minute))
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "1", "1", "1", until.Unix())
	closed, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if err := (&run.Service{Pool: closed}).PublishGauges(context.Background()); err == nil {
		t.Fatal("closed database produced a successful chain snapshot")
	}
	series := scrapedSeries(t)
	for _, name := range []string{"skillhub_continuation_chains", "skillhub_continuation_rounds", "skillhub_continuation_longest_rounds", "skillhub_continuation_observed_at_seconds"} {
		if _, present := series[name]; present {
			t.Fatalf("unreadable chain snapshot retained %s", name)
		}
	}
	if series["skillhub_continuation_observation_success"] != "0" {
		t.Fatal("unreadable chain snapshot reports success")
	}
	observedContinuationRun(t, pool, f, until.Add(-time.Minute))
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "2", "1", "1", until.Unix())
}

func TestContinuationObservationPublisherUsesTheFullDay(t *testing.T) {
	pool := continuationObservationPool(t)
	f := newFixture(t, newAPI(t, pool), pool, freshName("chain-day"))
	until := time.Now().UTC()
	for _, offset := range []time.Duration{-25 * time.Hour, -23 * time.Hour, -time.Minute, time.Hour} {
		observedContinuationRun(t, pool, f, until.Add(offset))
	}
	service := &run.Service{Pool: pool, Now: func() time.Time { return until }}
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "2", "0", "0", until.Unix())
}

func TestContinuationObservationUsesRetainedStructureAfterAccountPurge(t *testing.T) {
	pool := continuationObservationPool(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, freshName("chain-purge"))
	root := seedAskingRun(t, pool, f, "Which date?")
	seedAnsweredRound(t, pool, f, root, "Which time?")
	mustExec(t, pool, `UPDATE runs SET cleanup_status='cleaned' WHERE workspace_id=$1`, mustUUID(t, f.workspaceID))
	until := time.Now().UTC().Add(time.Minute)
	service := &run.Service{Pool: pool, Now: func() time.Time { return until }}
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "1", "1", "1", until.Unix())
	dueForPurge(t, pool, f.client)
	if count, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), &recordingStore{}, 0, 100); err != nil || count != 1 {
		t.Fatalf("purge = %d, %v; want one account", count, err)
	}
	if got := countRow(t, pool, `SELECT count(*) FROM users WHERE id=$1 AND deleted_at IS NOT NULL`, mustUUID(t, f.userID)); got != 1 {
		t.Fatal("account purge did not finish")
	}
	if err := service.PublishGauges(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertContinuationObservation(t, "1", "1", "1", until.Unix())
}

func TestContinuationObservationCohortIndexIsPresent(t *testing.T) {
	pool := continuationObservationPool(t)
	var indexed bool
	err := pool.QueryRow(context.Background(), `SELECT EXISTS (
        SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
        WHERE c.relname='runs_created_at_idx' AND i.indrelid='runs'::regclass
          AND i.indisvalid AND i.indpred IS NULL AND pg_get_indexdef(i.indexrelid,1,true)='created_at')`).Scan(&indexed)
	if err != nil || !indexed {
		t.Fatalf("all-status root cohort index = %t, %v; want valid created_at index", indexed, err)
	}
}
