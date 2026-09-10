package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
)

func creditTestLimits() creation.Limits {
	return creation.Limits{MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 8, MaxToolCalls: 3, CallTimeout: 2 * time.Second, SessionTimeout: time.Minute, Retention: time.Hour, MaxOutputTokens: 1000}
}

func seedCreationStep(t *testing.T, c *client, sessionID pgtype.UUID, usdMicros int64, source string, ago time.Duration) {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO cost_events (kind, model, usd_micros, cost_source, workspace_id, user_id, ref_type, ref_id, idempotency_key, created_at)
		VALUES ('creation_step', 'fixture-model', $1, $2, $3, $4, 'creation_session', $5, gen_random_uuid()::text, now() - $6::interval)`,
		usdMicros, source, mustUUID(t, c.workspaceID), mustUUID(t, c.userID), sessionID, ago); err != nil {
		t.Fatal(err)
	}
}

func sessionSummary(t *testing.T, sessionID pgtype.UUID) (rows int, usdMicros int64, steps int, estimated bool) {
	t.Helper()
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*), coalesce(max(usd_micros), 0), coalesce(max(steps), 0), coalesce(bool_or(estimated), false)
		FROM cost_session_summaries WHERE session_id = $1`, sessionID).Scan(&rows, &usdMicros, &steps, &estimated); err != nil {
		t.Fatal(err)
	}
	return rows, usdMicros, steps, estimated
}

func TestAnEndedCreationSessionLeavesExactlyOneCostSummary(t *testing.T) {
	a, _, _ := creationFixtureWithLimits(t, creditTestLimits())
	c := a.login(t, "summary-cancel")
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": 0.5}, 200)
	id := mustUUID(t, v.ID)
	seedCreationStep(t, c, id, 3000, "gateway", 0)
	seedCreationStep(t, c, id, 5000, "estimated", 0)

	creationAct(t, c, v, "cancel")

	rows, usd, steps, estimated := sessionSummary(t, id)
	if rows != 1 {
		t.Fatalf("cost summaries for the cancelled session = %d, want exactly 1", rows)
	}
	if usd != 8000 || steps != 2 || !estimated {
		t.Errorf("summary = %d micros / %d steps / estimated %v, want 8000 / 2 / true", usd, steps, estimated)
	}
}

func TestTheDailyJobSummarizesASessionThatStoppedWithoutEnding(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "summary-idle")
	id := creationID(t)
	seedCreationStep(t, c, id, 4000, "gateway", 2*time.Hour)

	svc := &credit.Service{Store: credit.NewPostgresStore(pool), Config: credit.Config{SessionIdle: time.Hour}}
	stats, err := svc.RecomputeStatistics(context.Background(), credit.KindCreationSession, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rows, usd, steps, _ := sessionSummary(t, id); rows != 1 || usd != 4000 || steps != 1 {
		t.Errorf("summary = %d rows / %d micros / %d steps, want 1 / 4000 / 1", rows, usd, steps)
	}
	if stats.SampleCount < 1 {
		t.Errorf("statistics sample count = %d, want the idle session counted", stats.SampleCount)
	}
}

func TestTheStartGateReadsWhatAWholeSessionCosts(t *testing.T) {
	a, _, _ := creationFixtureWithLimits(t, creditTestLimits())
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO cost_statistics (kind, window_start, window_end, sample_count, p50_usd_micros, p90_usd_micros, p95_usd_micros, max_usd_micros)
		VALUES ('creation_session', now() - interval '1 day', now() + interval '1 day', 20, 1000000, 1000000, 1000000, 1000000)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM cost_statistics WHERE kind = 'creation_session' AND window_end > now()`)
	})
	a.startingCredits = 1000
	c := a.login(t, "summary-gate")

	creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_usd": 0.5}, 422)
}
