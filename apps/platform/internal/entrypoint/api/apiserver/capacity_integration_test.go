package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
)

func assertCapacityMetricsAbsent(t *testing.T) {
	t.Helper()
	actual := scrapedSeries(t)
	for _, name := range []string{"skillhub_database_bytes", "skillhub_restore_budget_bytes", "skillhub_restore_budget_days_left"} {
		if value, present := actual[name]; present {
			t.Errorf("failed capacity still exports %s=%s", name, value)
		}
	}
}

func assertCapacityValues(t *testing.T, actual, expected map[string]string) {
	t.Helper()
	for name, want := range expected {
		if got, present := actual[name]; !present || got != want {
			t.Errorf("%s=%q (present=%t), want %q", name, got, present, want)
		}
	}
}

func TestCapacityMetricsRecoverAfterAReadFailure(t *testing.T) {
	config := requireDB(t).Config()
	config.MaxConns = 1
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TEMP TABLE database_size_samples (LIKE public.database_size_samples INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	clear := func() {
		metrics.DatabaseBytes.Reset()
		metrics.RestoreBudgetBytes.Reset()
		metrics.RestoreBudgetDaysLeft.Reset()
	}
	clear()
	t.Cleanup(clear)
	store := capacity.Store{Pool: pool, Rate: capacity.RestoreRate{BytesPerSecond: 3}}
	if _, err := pool.Exec(ctx, "INSERT INTO database_size_samples (sampled_on, database_bytes) VALUES (CURRENT_DATE, 2400)"); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGauges(ctx); err != nil {
		t.Fatal(err)
	}
	initial := scrapedSeries(t)
	assertCapacityValues(t, initial, map[string]string{
		"skillhub_database_bytes":           "2400",
		"skillhub_restore_budget_bytes":     "1800",
		"skillhub_restore_budget_days_left": "0",
	})
	closed, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if err := (capacity.Store{Pool: closed, Rate: store.Rate}).PublishGauges(ctx); err == nil || err.Error() != "closed pool" {
		t.Fatalf("failed observation = %v, want closed pool", err)
	}
	assertCapacityMetricsAbsent(t)
	if _, err := pool.Exec(ctx, "UPDATE database_size_samples SET database_bytes = 3600"); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGauges(ctx); err != nil {
		t.Fatal(err)
	}
	recovered := scrapedSeries(t)
	assertCapacityValues(t, recovered, map[string]string{
		"skillhub_database_bytes":           "3600",
		"skillhub_restore_budget_bytes":     "1800",
		"skillhub_restore_budget_days_left": "0",
	})
	if _, err := pool.Exec(ctx, "DELETE FROM database_size_samples"); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishGauges(ctx); err != nil {
		t.Fatal(err)
	}
	empty := scrapedSeries(t)
	if value, present := empty["skillhub_database_bytes"]; present {
		t.Errorf("empty sample history still exports database bytes %s", value)
	}
	assertCapacityValues(t, empty, map[string]string{
		"skillhub_restore_budget_bytes":     "1800",
		"skillhub_restore_budget_days_left": "+Inf",
	})
}

func TestCapacityKeepsOneSamplePerDayAndReportsEveryTableAgainstTheBudget(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	sampledDay := time.Date(2001, time.January, 1, 23, 0, 0, 0, time.UTC)
	forget := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM database_size_samples WHERE sampled_on = '2001-01-01'`)
	}
	forget()
	t.Cleanup(forget)

	store := capacity.Store{Pool: pool, Rate: capacity.RestoreRate{BytesPerSecond: 7, Measured: true}}
	if err := store.RecordToday(ctx, sampledDay.Add(-22*time.Hour)); err != nil {
		t.Fatal(err)
	}
	report, err := store.Report(ctx, sampledDay)
	if err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM database_size_samples WHERE sampled_on = '2001-01-01'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("samples for one day = %d, want 1", rows)
	}
	if report.DatabaseBytes <= 0 {
		t.Errorf("database_bytes = %d, want the live database size", report.DatabaseBytes)
	}
	if report.RestoreBudgetBytes != 7*600 || !report.RestoreRateMeasured {
		t.Errorf("budget = %d measured=%v, want 4200 measured", report.RestoreBudgetBytes, report.RestoreRateMeasured)
	}
	if report.DaysUntilBudget == nil || *report.DaysUntilBudget != 0 {
		t.Errorf("days_until_budget = %v, want 0 for a database far past a 4200-byte budget", report.DaysUntilBudget)
	}
	tables := map[string]int64{}
	for _, size := range report.Tables {
		tables[size.Table] = size.Bytes
	}
	if tables["database_size_samples"] <= 0 {
		t.Errorf("tables = %v, want database_size_samples listed with its size", tables)
	}
	if _, ok := tables["trace_events_default"]; ok {
		t.Errorf("a partition was listed on its own instead of under trace_events: %v", tables)
	}
	if _, ok := tables["trace_events"]; !ok {
		t.Errorf("trace_events is missing from %v", tables)
	}
}
