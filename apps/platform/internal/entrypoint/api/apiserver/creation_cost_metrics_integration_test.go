package apiserver_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const knownCreationCost = `{"snapshot":{"spent_usd":0.02,"reserved_usd":0,"usage_unknown":false}}`

func creationCostPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := isolatedAPIPool(t)
	reset := func() {
		for _, gauge := range []*prometheus.GaugeVec{metrics.CreationCostSessions, metrics.CreationKnownSpend, metrics.CreationMaxKnownCost, metrics.CreationCostObservationSuccess, metrics.CreationCostObservedAt} {
			gauge.Reset()
		}
	}
	reset()
	t.Cleanup(reset)
	return pool
}

func observedCreationCost(t *testing.T, pool *pgxpool.Pool, workspace pgtype.UUID, snapshot string, at, expires time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `INSERT INTO creation_sessions (id,workspace_id,state,revision,snapshot,created_at,expires_at)
        VALUES (gen_random_uuid(),$1,'queued',1,$2,$3,$4) RETURNING id::text`, workspace, snapshot, at, expires).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func creationCostRows(t *testing.T, pool *pgxpool.Pool, since, until time.Time) []gen.ListCreationCostObservationsRow {
	t.Helper()
	rows, err := gen.New(pool).ListCreationCostObservations(context.Background(), gen.ListCreationCostObservationsParams{Since: pgconv.Timestamptz(since), Until: pgconv.Timestamptz(until)})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestCreationCostObservationUsesTheRootWindowAndLiveExpiry(t *testing.T) {
	pool := creationCostPool(t)
	ws := newCreationWorkspace(t, pool)
	until := time.Now().UTC().Truncate(time.Microsecond)
	since := until.Add(-24 * time.Hour)
	for _, at := range []time.Time{since.Add(-time.Microsecond), since, until.Add(-time.Microsecond), until, until.Add(time.Microsecond)} {
		observedCreationCost(t, pool, ws.ID, knownCreationCost, at, until.Add(time.Hour))
	}
	for _, expiry := range []time.Time{until.Add(-time.Microsecond), until, until.Add(time.Microsecond)} {
		observedCreationCost(t, pool, ws.ID, knownCreationCost, since.Add(time.Hour), expiry)
	}
	rows := creationCostRows(t, pool, since, until)
	if len(rows) != 3 {
		t.Fatalf("root/expiry rows = %d; want three sessions", len(rows))
	}
	for _, row := range rows {
		if string(row.SpentUsd) != "0.02" || string(row.ReservedUsd) != "0" || string(row.UsageUnknown) != "false" {
			t.Fatalf("cost projection = %+v; want only known cost metadata", row)
		}
	}
}

func TestCreationCostObservationIncludesEveryStateAndKeepsMalformedSnapshotsUnknown(t *testing.T) {
	pool := creationCostPool(t)
	ws := newCreationWorkspace(t, pool)
	for _, state := range creation.AllStates() {
		id := observedCreationCost(t, pool, ws.ID, strings.Replace(knownCreationCost, "0.02", "0.125", 1), time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
		mustExec(t, pool, `UPDATE creation_sessions SET state=$2 WHERE id=$1`, mustUUID(t, id), string(state))
	}
	for _, snapshot := range []string{`{}`, `"legacy"`, `{"snapshot":{"spent_usd":1e309,"reserved_usd":0,"usage_unknown":false,"messages":"private-cost-canary"}}`} {
		observedCreationCost(t, pool, ws.ID, snapshot, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	}
	if err := creation.PublishCostGauges(pool)(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "10", "0", "3", "1.25", "0.125", time.Now().Unix()-1)
}

func assertCreationCostObservation(t *testing.T, known, unsettled, unknown, sum, largest string, notBefore int64) {
	t.Helper()
	series := scrapedSeries(t)
	for name, want := range map[string]string{
		`skillhub_creation_cost_sessions{coverage="known"}`: known, `skillhub_creation_cost_sessions{coverage="unsettled"}`: unsettled,
		`skillhub_creation_cost_sessions{coverage="unknown"}`: unknown, "skillhub_creation_known_spend_usd": sum,
		"skillhub_creation_max_known_session_cost_usd": largest, "skillhub_creation_cost_observation_success": "1",
	} {
		value, exists := series[name]
		if (want == "" && exists) || (want != "" && (!exists || value != want)) {
			t.Fatalf("%s = %q, present=%t; want %q", name, value, exists, want)
		}
	}
	at, err := strconv.ParseFloat(series["skillhub_creation_cost_observed_at_seconds"], 64)
	if err != nil || int64(at) < notBefore || int64(at) > time.Now().Unix() {
		t.Fatalf("cost observation time = %v, %v; earliest %d", at, err, notBefore)
	}
	for name := range series {
		if strings.HasPrefix(name, "skillhub_creation_") && strings.Contains(name, "{") && !creationCostCoverageLabel(name) {
			t.Fatalf("cost observation has an unexpected label: %s", name)
		}
	}
}

func TestCreationCostObservationReadsRealStepsAcrossWorkersWithoutReplayOrCharges(t *testing.T) {
	pool := creationCostPool(t)
	a, service, calls := creationFixtureOnPool(t, pool, creationLimits())
	c := a.login(t, freshName("creation-cost-worker"))
	id := creationID(t)
	body := map[string]any{"id": id, "message": "整理資料", "budget_credits": 650}
	v := creationPost(t, c, "/creation-sessions", body, http.StatusOK)
	job := creationJobFromPool(t, pool, v.ID)
	if err := service.Step(context.Background(), job, nil); err != nil {
		t.Fatal(err)
	}
	creationPost(t, c, "/creation-sessions", body, http.StatusOK)
	if calls.Load() != 1 {
		t.Fatalf("replay called the model %d times; want one", calls.Load())
	}
	other := a.login(t, freshName("creation-cost-other"))
	observedCreationCost(t, pool, mustUUID(t, other.workspaceID), strings.Replace(knownCreationCost, "0.02", "0.03", 1), time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	var before, after string
	const unchanged = `SELECT (SELECT jsonb_agg(snapshot ORDER BY id)::text FROM creation_sessions) || ':' || (SELECT count(*) FROM cost_events)::text || ':' || (SELECT count(*) FROM credit_entries)::text`
	if err := pool.QueryRow(context.Background(), unchanged).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		set, err := worker.BuildWorkers(pool, worker.Deps{})
		if err != nil {
			t.Fatal(err)
		}
		for range 2 {
			now := time.Now().Unix()
			requireWorkerGaugeRefresh(t, set)
			assertCreationCostObservation(t, "2", "0", "0", "0.04", "0.03", now)
		}
	}
	if err := pool.QueryRow(context.Background(), unchanged).Scan(&after); err != nil || after != before {
		t.Fatalf("cost observation changed a snapshot or charged a fee: %v", err)
	}
}

func TestCreationCostObservationWithdrawsUnknownAmountsAndFailedReadsThenRecovers(t *testing.T) {
	pool := creationCostPool(t)
	ws := newCreationWorkspace(t, pool)
	publish := creation.PublishCostGauges(pool)
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "0", "0", "0", "", "", time.Now().Unix()-1)
	id := observedCreationCost(t, pool, ws.ID, knownCreationCost, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "1", "0", "0", "0.02", "0.02", time.Now().Unix()-1)
	mustExec(t, pool, `UPDATE creation_sessions SET snapshot='{"snapshot":{"spent_usd":0.02,"reserved_usd":0.1,"usage_unknown":false}}' WHERE id=$1`, mustUUID(t, id))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "0", "1", "0", "", "", time.Now().Unix()-1)
	mustExec(t, pool, `UPDATE creation_sessions SET snapshot='{"snapshot":{"spent_usd":0.02,"reserved_usd":0,"usage_unknown":true}}' WHERE id=$1`, mustUUID(t, id))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "0", "0", "1", "", "", time.Now().Unix()-1)
	mustExec(t, pool, `UPDATE creation_sessions SET snapshot='{"snapshot":{"spent_usd":0,"reserved_usd":0,"usage_unknown":false}}' WHERE id=$1`, mustUUID(t, id))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "1", "0", "0", "0", "0", time.Now().Unix()-1)
	closed, err := pgxpool.NewWithConfig(context.Background(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if err := creation.PublishCostGauges(closed)(context.Background()); err == nil {
		t.Fatal("closed database produced a successful cost observation")
	}
	series := scrapedSeries(t)
	for name := range series {
		if strings.HasPrefix(name, "skillhub_creation_") && name != "skillhub_creation_cost_observation_success" {
			t.Fatalf("unreadable cost observation retained %s", name)
		}
	}
	if series["skillhub_creation_cost_observation_success"] != "0" {
		t.Fatal("unreadable cost observation reports success")
	}
	observedCreationCost(t, pool, ws.ID, knownCreationCost, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "2", "0", "0", "0.02", "0.02", time.Now().Unix()-1)
}

func TestCreationCostObservationPublisherUsesTheFullDay(t *testing.T) {
	pool := creationCostPool(t)
	ws := newCreationWorkspace(t, pool)
	now := time.Now()
	for _, age := range []time.Duration{-25 * time.Hour, -23 * time.Hour, -time.Minute, time.Hour} {
		observedCreationCost(t, pool, ws.ID, knownCreationCost, now.Add(age), now.Add(time.Hour))
	}
	if err := creation.PublishCostGauges(pool)(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "2", "0", "0", "0.04", "0.02", now.Unix())
}

func TestCreationCostObservationReflectsExpiryAndActualAccountPurge(t *testing.T) {
	pool := creationCostPool(t)
	a := newAPI(t, pool)
	c := a.login(t, freshName("creation-cost-purge"))
	publish := creation.PublishCostGauges(pool)
	id := observedCreationCost(t, pool, mustUUID(t, c.workspaceID), knownCreationCost, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "1", "0", "0", "0.02", "0.02", time.Now().Unix()-1)
	mustExec(t, pool, `UPDATE creation_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, mustUUID(t, id))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "0", "0", "0", "", "", time.Now().Unix()-1)
	if count, err := creation.PurgeExpired(context.Background(), pool); err != nil || count != 1 {
		t.Fatalf("expiry purge = %d, %v; want one session", count, err)
	}
	observedCreationCost(t, pool, mustUUID(t, c.workspaceID), knownCreationCost, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	dueForPurge(t, pool, c)
	if count, err := a.auth.Service.PurgeExpiredAccounts(context.Background(), &recordingStore{}, 0, 100); err != nil || count != 1 {
		t.Fatalf("account purge = %d, %v; want one account", count, err)
	}
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "0", "0", "0", "", "", time.Now().Unix()-1)
}

func TestCreationCostObservationWithdrawsAnOverflowingAggregate(t *testing.T) {
	pool := creationCostPool(t)
	ws := newCreationWorkspace(t, pool)
	publish := creation.PublishCostGauges(pool)
	observedCreationCost(t, pool, ws.ID, knownCreationCost, time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	if err := publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertCreationCostObservation(t, "1", "0", "0", "0.02", "0.02", time.Now().Unix()-1)
	for range 2 {
		observedCreationCost(t, pool, ws.ID, strings.Replace(knownCreationCost, "0.02", "1.7e308", 1), time.Now().Add(-time.Minute), time.Now().Add(time.Hour))
	}
	if err := publish(context.Background()); err == nil {
		t.Fatal("overflowing cost sum produced a successful observation")
	}
	series := scrapedSeries(t)
	if series["skillhub_creation_cost_observation_success"] != "0" {
		t.Fatal("overflowing cost sum reports success")
	}
	for name := range series {
		if strings.HasPrefix(name, "skillhub_creation_") && name != "skillhub_creation_cost_observation_success" {
			t.Fatalf("overflowing cost sum retained %s", name)
		}
	}
}

func creationCostCoverageLabel(name string) bool {
	return name == `skillhub_creation_cost_sessions{coverage="known"}` || name == `skillhub_creation_cost_sessions{coverage="unsettled"}` || name == `skillhub_creation_cost_sessions{coverage="unknown"}`
}
