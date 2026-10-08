package apiserver_test

import (
	"context"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
)

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
