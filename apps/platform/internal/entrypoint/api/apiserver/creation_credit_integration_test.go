package apiserver_test

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
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
	v := creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_credits": 650}, 200)
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

	creationPost(t, c, "/creation-sessions", map[string]any{"id": creationID(t), "message": "", "budget_credits": 650}, 422)

	var credits map[string]any
	if code := getJSON(t, c.Client, c.base+"/me/credits", &credits); code != 200 {
		t.Fatalf("GET /me/credits: got %d", code)
	}
	if credits["can_start"] != false {
		t.Errorf("the balance screen says can_start = %v while the gate refuses; both must read what a whole session costs", credits["can_start"])
	}
}

func creationDomain(t *testing.T, s *creation.Service, c *client, v creation.View) creation.View {
	t.Helper()
	out, err := s.Get(context.Background(), identity.Workspace{ID: mustUUID(t, c.workspaceID)}, mustUUID(t, v.ID))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheCreationScreenIsQuotedInCredits(t *testing.T) {
	a, _, _ := creationFixtureWithLimits(t, creditTestLimits())
	c := a.login(t, "creation-in-credits")

	var limits map[string]any
	if code := getJSON(t, c.Client, c.base+"/creation-sessions/limits", &limits); code != 200 {
		t.Fatalf("limits: got %d", code)
	}
	if limits["min_budget_credits"] != float64(130) || limits["max_budget_credits"] != float64(1300) {
		t.Errorf("limits = %v, want 130..1300 credits", limits)
	}

	id := creationID(t)
	creationPost(t, c, "/creation-sessions", map[string]any{"id": id, "message": "", "budget_credits": 650}, 200)
	var raw map[string]any
	if code := getJSON(t, c.Client, c.base+"/creation-sessions/"+uuidText(id), &raw); code != 200 {
		t.Fatalf("get: got %d", code)
	}
	snap, _ := raw["snapshot"].(map[string]any)
	if snap["budget_credits"] != float64(650) || snap["reserved_credits"] != float64(0) {
		t.Errorf("snapshot budget = %v reserved = %v, want 650 / 0", snap["budget_credits"], snap["reserved_credits"])
	}
	for _, dollars := range []string{"budget_usd", "reserved_usd", "spent_usd"} {
		if _, shown := snap[dollars]; shown {
			t.Errorf("the snapshot still shows %s", dollars)
		}
	}
}

var dollarField = regexp.MustCompile(`^\s+((?:[a-z0-9_]+_)?usd):\s*$`)

func TestNoFieldInThePublicContractIsPricedInDollars(t *testing.T) {
	b, err := os.ReadFile("../../../../../../contracts/openapi/public.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		if m := dollarField.FindStringSubmatch(line); m != nil {
			t.Errorf("public.yaml:%d names the field %q; users see credits, only the ledger holds dollars", i+1, m[1])
		}
	}
}

func TestTheBalanceScreenShowsTheConfiguredDebtFloor(t *testing.T) {
	t.Setenv("CREDIT_DEBT_FLOOR", "-80")
	a := newAPI(t, requireDB(t))
	c := a.login(t, "credit-floor-shown")

	var credits map[string]any
	if code := getJSON(t, c.Client, c.base+"/me/credits", &credits); code != 200 {
		t.Fatalf("GET /me/credits: got %d", code)
	}
	if credits["debt_floor_credits"] != float64(-80) {
		t.Errorf("debt_floor_credits = %v, want the configured -80", credits["debt_floor_credits"])
	}
}

func forgetStatistics(t *testing.T, windowEnd time.Time) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM cost_statistics WHERE window_end = $1`, windowEnd)
	})
}

func seedCostEvent(t *testing.T, kind string, usdMicros int64, source string, at time.Time) {
	t.Helper()
	key := "statistics-fixture:" + uuid.NewString()
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO cost_events (kind, model, usd_micros, cost_source, idempotency_key, created_at)
		VALUES ($1, 'fixture-model', $2, $3, $4, $5)`, kind, usdMicros, source, key, at); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		tx, err := testPool.Begin(ctx)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		if _, err := tx.Exec(ctx, `DELETE FROM cost_events WHERE idempotency_key = $1`, key); err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
}

func TestTheDailyStatisticsSurviveAWindowWithNoEvents(t *testing.T) {
	store := credit.NewPostgresStore(requireDB(t))
	start := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Second)
	forgetStatistics(t, end)

	for _, kind := range []string{credit.KindSuggestion, credit.KindCreationSession} {
		stats, err := store.RecomputeStatistics(context.Background(), kind, start, end)
		if err != nil {
			t.Fatalf("%s over a window with no events: %v", kind, err)
		}
		if stats.SampleCount != 0 {
			t.Errorf("%s sample count = %d, want 0", kind, stats.SampleCount)
		}
	}
}

func TestAnEstimatedCallCostStaysOutOfTheStatistics(t *testing.T) {
	store := credit.NewPostgresStore(requireDB(t))
	start := time.Date(2002, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	forgetStatistics(t, end)
	seedCostEvent(t, credit.KindSuggestion, 1_000, "gateway", start.Add(time.Minute))
	seedCostEvent(t, credit.KindSuggestion, 999_000, "estimated", start.Add(time.Minute))

	stats, err := store.RecomputeStatistics(context.Background(), credit.KindSuggestion, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SampleCount != 1 || stats.MaxUsdMicros != 1_000 {
		t.Errorf("statistics = %d samples, max %d micros; want 1 sample, max 1000 — only the gateway-priced call counts", stats.SampleCount, stats.MaxUsdMicros)
	}
}

func TestASessionWithAnEstimatedStepStaysOutOfTheStatistics(t *testing.T) {
	pool := requireDB(t)
	store := credit.NewPostgresStore(pool)
	start := time.Date(2003, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	forgetStatistics(t, end)
	for _, row := range []struct {
		usdMicros int64
		estimated bool
	}{{2_000, false}, {888_000, true}} {
		id := uuid.NewString()
		if _, err := pool.Exec(context.Background(), `
			INSERT INTO cost_session_summaries (session_id, usd_micros, steps, estimated, last_step_at)
			VALUES ($1, $2, 1, $3, $4)`, id, row.usdMicros, row.estimated, start.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM cost_session_summaries WHERE session_id = $1`, id)
		})
	}

	stats, err := store.RecomputeStatistics(context.Background(), credit.KindCreationSession, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if stats.SampleCount != 1 || stats.MaxUsdMicros != 2_000 {
		t.Errorf("statistics = %d samples, max %d micros; want 1 sample, max 2000 — a session priced by a guess is not a sample", stats.SampleCount, stats.MaxUsdMicros)
	}
}

func TestAccountDeletionLeavesTheCreditLedgerAlone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "credit-kept-after-deletion")
	ctx := context.Background()
	user := mustUUID(t, c.userID)
	ledger, err := worker.NewCreditService(pool)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Grant(ctx, tx, credit.GrantInput{
		UserID: user, EntryKind: credit.EntryGrant, Credits: 100, Reason: "fixture", OperatorID: user,
		IdempotencyKey: "kept-after-deletion:" + uuid.NewString(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_events (kind, model, usd_micros, cost_source, user_id, idempotency_key)
		VALUES ('suggestion', 'fixture-model', 1000, 'gateway', $1, $2)`, user, "kept-after-deletion:"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cost_session_summaries (session_id, user_id, usd_micros, steps, estimated, last_step_at)
		VALUES ($1, $2, 1000, 1, false, now())`, uuid.NewString(), user); err != nil {
		t.Fatal(err)
	}
	tables := []string{"credit_accounts", "credit_entries", "cost_events", "cost_session_summaries"}
	before := map[string]int{}
	for _, table := range tables {
		before[table] = countRow(t, pool, "SELECT count(*) FROM "+table+" WHERE user_id = $1", user)
		if before[table] == 0 {
			t.Fatalf("the fixture left no %s rows to keep", table)
		}
	}

	if code := c.status(t, "DELETE", "/me"); code != 200 {
		t.Fatalf("DELETE /me: %d", code)
	}
	if _, err := a.auth.Service.PurgeExpiredAccounts(ctx, a.packages, 0, 10); err != nil {
		t.Fatal(err)
	}
	if n := countRow(t, pool, "SELECT count(*) FROM users WHERE id = $1 AND deleted_at IS NOT NULL", user); n != 1 {
		t.Fatal("the account purge did not finish, so nothing here was tested")
	}
	for _, table := range tables {
		if got := countRow(t, pool, "SELECT count(*) FROM "+table+" WHERE user_id = $1", user); got != before[table] {
			t.Errorf("%s rows for the deleted account = %d, want the %d it had", table, got, before[table])
		}
	}
}

func TestTheLedgerAndItsStatisticsAcceptTheMatchReasonsKind(t *testing.T) {
	store := credit.NewPostgresStore(requireDB(t))
	start := time.Date(2004, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	forgetStatistics(t, end)
	seedCostEvent(t, credit.KindMatchReasons, 420, "gateway", start.Add(time.Minute))

	stats, err := store.RecomputeStatistics(context.Background(), credit.KindMatchReasons, start, end)
	if err != nil {
		t.Fatalf("recomputing match_reasons statistics: %v", err)
	}
	if stats.SampleCount != 1 || stats.MaxUsdMicros != 420 {
		t.Errorf("statistics = %d samples, max %d micros; want 1 sample, max 420", stats.SampleCount, stats.MaxUsdMicros)
	}
}
