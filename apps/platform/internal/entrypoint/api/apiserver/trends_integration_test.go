package apiserver_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type trendKey struct{ day, key string }

func getTrend(t *testing.T, c *client, path string) (map[string]any, map[trendKey]map[string]any) {
	t.Helper()
	code, body := getAdmin(t, c, path)
	if code != http.StatusOK {
		t.Fatalf("GET %s: got %d (%v)", path, code, body)
	}
	buckets := map[trendKey]map[string]any{}
	for _, b := range objects(t, body["buckets"]) {
		day, _ := b["day"].(string)
		key, _ := b["key"].(string)
		buckets[trendKey{day, key}] = b
	}
	return body, buckets
}

func trendNumber(v map[string]any, field string) int64 {
	n, _ := v[field].(float64)
	return int64(n)
}

func trendDay(t *testing.T, v any) time.Time {
	t.Helper()
	s, _ := v.(string)
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatalf("want a YYYY-MM-DD day, got %v", v)
	}
	return d
}

func trendOperator(t *testing.T, a *api, name string) *client {
	t.Helper()
	operator := a.login(t, name)
	a.auth.Operators = map[string]bool{operator.userID: true}
	return operator
}

func assertTrendDelta(t *testing.T, before, after map[trendKey]map[string]any, day time.Time, key string, field string, want int64) {
	t.Helper()
	k := trendKey{day.Format(time.DateOnly), key}
	if got := trendNumber(after[k], field) - trendNumber(before[k], field); got != want {
		t.Errorf("%s %s on %s: grew by %d, want %d", key, field, k.day, got, want)
	}
}

func TestTrendRangeIsTheLastNUTCDaysEndingToday(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := trendOperator(t, a, "trend-range-operator")

	today := time.Now().UTC().Format(time.DateOnly)
	for _, tc := range []struct {
		name  string
		query string
		span  int
	}{
		{"no days means 30", "", 30},
		{"7", "?days=7", 7},
		{"30", "?days=30", 30},
		{"90", "?days=90", 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := getTrend(t, operator, "/admin/trends/runs"+tc.query)
			if body["to"] != today {
				t.Errorf("to = %v, want today %s", body["to"], today)
			}
			from, to := trendDay(t, body["from"]), trendDay(t, body["to"])
			if got := int(to.Sub(from).Hours()/24) + 1; got != tc.span {
				t.Errorf("the range covers %d days, want %d", got, tc.span)
			}
		})
	}

	for _, bad := range []string{"?days=6", "?days=8", "?days=31", "?days=91", "?days=0", "?days=abc", "?days="} {
		for _, path := range []string{
			"/admin/trends/cost", "/admin/trends/credits", "/admin/trends/runs", "/admin/trends/operator-actions",
		} {
			if code, _ := getAdmin(t, operator, path+bad); code != http.StatusBadRequest {
				t.Errorf("GET %s%s: got %d, want 400", path, bad, code)
			}
		}
	}
}

func TestCostTrendSumsEachKindPerUTCDayFromTheFirstInstantOfTheRange(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := trendOperator(t, a, "trend-cost-operator")

	body, before := getTrend(t, operator, "/admin/trends/cost?days=7")
	from, to := trendDay(t, body["from"]), trendDay(t, body["to"])
	seedCostEvent(t, "review", 1000, "gateway", from)
	seedCostEvent(t, "review", 250, "estimated", from.Add(time.Hour))
	seedCostEvent(t, "review", 500, "gateway", to.Add(time.Minute))
	seedCostEvent(t, "generate", 7, "gateway", from.Add(-time.Second))

	_, after := getTrend(t, operator, "/admin/trends/cost?days=7")
	assertTrendDelta(t, before, after, from, "review", "count", 2)
	assertTrendDelta(t, before, after, from, "review", "total", 1250)
	assertTrendDelta(t, before, after, to, "review", "count", 1)
	assertTrendDelta(t, before, after, to, "review", "total", 500)
	assertTrendDelta(t, before, after, from, "generate", "count", 0)
	if _, ok := after[trendKey{from.AddDate(0, 0, -1).Format(time.DateOnly), "generate"}]; ok {
		t.Error("a day before the range came back as a bucket")
	}
}

func TestCreditTrendNetsEachEntryKindAndTotalsEveryBalance(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := trendOperator(t, a, "trend-credit-operator")
	owner := a.login(t, "trend-credit-owner")

	body, before := getTrend(t, operator, "/admin/trends/credits?days=7")
	to := trendDay(t, body["to"])
	if code, out := operatorCall(t, operator, http.MethodPost, "/admin/credits/"+owner.workspaceID+"/grants",
		`{"amount_credits":150,"reason":"trend fixture"}`); code != http.StatusOK {
		t.Fatalf("grant: got %d (%v)", code, out)
	}

	next, after := getTrend(t, operator, "/admin/trends/credits?days=7")
	assertTrendDelta(t, before, after, to, "grant", "count", 1)
	assertTrendDelta(t, before, after, to, "grant", "total", 150)
	if got := trendNumber(next, "balance_total") - trendNumber(body, "balance_total"); got != 150 {
		t.Errorf("balance_total grew by %d, want 150", got)
	}
}

func TestRunTrendCountsRunsByTheDayTheyWereCreatedAndTheStatusTheyAreInNow(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := trendOperator(t, a, "trend-run-operator")
	owner := a.login(t, "trend-run-owner")

	body, before := getTrend(t, operator, "/admin/trends/runs?days=7")
	from, to := trendDay(t, body["from"]), trendDay(t, body["to"])
	skillID := seedSkill(t, pool, owner.workspaceID, "trend-run-skill")
	seedRunWithCriteria(t, pool, owner.workspaceID, skillID, cmpVersion(t, pool, owner.workspaceID, skillID, "trend-now"), `[]`)
	seedRunCreatedAt(t, pool, owner.workspaceID, skillID,
		cmpVersion(t, pool, owner.workspaceID, skillID, "trend-before"), from.Add(-time.Second))

	_, after := getTrend(t, operator, "/admin/trends/runs?days=7")
	assertTrendDelta(t, before, after, to, "succeeded", "count", 1)
	assertTrendDelta(t, before, after, from, "succeeded", "count", 0)
	if _, ok := after[trendKey{from.AddDate(0, 0, -1).Format(time.DateOnly), "succeeded"}]; ok {
		t.Error("a day before the range came back as a bucket")
	}
}

func TestOperatorActionTrendCountsOnlyOperatorActions(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := trendOperator(t, a, "trend-action-operator")

	body, before := getTrend(t, operator, "/admin/trends/operator-actions?days=7")
	to := trendDay(t, body["to"])
	ctx := context.Background()
	insert := func(action, metadata string) {
		id := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO audit_events
			(actor_user_id, action, resource_type, resource_id, metadata, created_at)
			VALUES ($1, $2, 'skill', $3, $4::jsonb, $5)`,
			mustUUID(t, operator.userID), action, mustUUID(t, id), metadata, to.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { purgeAuditEvent(t, pool, id) })
	}
	insert("skill.takedown", `{"scope":"operator"}`)
	insert("skill.takedown", `{}`)
	insert("credit.lookup", `{}`)
	insert("artifact.download", `{}`)

	_, after := getTrend(t, operator, "/admin/trends/operator-actions?days=7")
	assertTrendDelta(t, before, after, to, "skill.takedown", "count", 1)
	assertTrendDelta(t, before, after, to, "credit.lookup", "count", 1)
	if _, ok := after[trendKey{to.Format(time.DateOnly), "artifact.download"}]; ok {
		t.Error("an action outside the operator list came back as a bucket")
	}
}

func seedRunCreatedAt(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, versionID string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'a run created before the range', '[]'::jsonb, $3)
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, seedTestCase(t, pool, workspaceID, skillID)), "sha256:trend-"+versionID,
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, finished_at, created_at)
		VALUES ($1, $2, $3, 'fake_sandbox', '{}'::jsonb, '{}'::jsonb, 'succeeded', $4, $4)`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID), at); err != nil {
		t.Fatal(err)
	}
}

func purgeAuditEvent(t *testing.T, pool *pgxpool.Pool, resourceID string) {
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM audit_events WHERE resource_id = $1`, mustUUID(t, resourceID)); err != nil {
		t.Errorf("cleanup: %v", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		t.Errorf("cleanup: %v", err)
	}
}
