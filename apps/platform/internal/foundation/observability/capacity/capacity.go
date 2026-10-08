package capacity

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

const (
	RestoreWindow                = 10 * time.Minute
	DefaultRestoreBytesPerSecond = 50 << 20
	GrowthWindow                 = 30 * 24 * time.Hour
	SampleInterval               = time.Hour
	RestoreRateEnv               = "RESTORE_BYTES_PER_SECOND"

	day = 24 * time.Hour
)

type RestoreRate struct {
	BytesPerSecond int64
	Measured       bool
}

func ParseRestoreRate(raw string) (RestoreRate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return RestoreRate{BytesPerSecond: DefaultRestoreBytesPerSecond}, nil
	}
	rate, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || rate <= 0 {
		return RestoreRate{}, fmt.Errorf("%s=%q must be a positive whole number of bytes per second", RestoreRateEnv, raw)
	}
	return RestoreRate{BytesPerSecond: rate, Measured: true}, nil
}

func (r RestoreRate) BudgetBytes() int64 {
	return r.BytesPerSecond * int64(RestoreWindow/time.Second)
}

type Sample struct {
	Day   time.Time
	Bytes int64
}

func DaysUntilBudget(samples []Sample, budget int64) float64 {
	if len(samples) == 0 {
		return math.Inf(1)
	}
	current := samples[len(samples)-1].Bytes
	if current >= budget {
		return 0
	}
	perDay := growthPerDay(samples)
	if perDay <= 0 {
		return math.Inf(1)
	}
	return float64(budget-current) / perDay
}

// growthPerDay is the least-squares slope of size over day, so one noisy
// sample (a vacuum, a bulk delete) bends the trend instead of setting it.
func growthPerDay(samples []Sample) float64 {
	if len(samples) < 2 {
		return 0
	}
	origin := samples[0].Day
	var n, sumX, sumY, sumXY, sumXX float64
	for _, s := range samples {
		x := float64(s.Day.Sub(origin)) / float64(day)
		y := float64(s.Bytes)
		n++
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denominator
}

type TableSize struct {
	Table string `json:"table"`
	Bytes int64  `json:"bytes"`
}

type Report struct {
	DatabaseBytes         int64       `json:"database_bytes"`
	RestoreBytesPerSecond int64       `json:"restore_bytes_per_second"`
	RestoreRateMeasured   bool        `json:"restore_rate_measured"`
	RestoreBudgetBytes    int64       `json:"restore_budget_bytes"`
	DaysUntilBudget       *float64    `json:"days_until_budget"`
	GrowthBytesPerDay     float64     `json:"growth_bytes_per_day"`
	Tables                []TableSize `json:"tables"`
}

type Store struct {
	Pool *pgxpool.Pool
	Rate RestoreRate
}

func (s Store) RecordToday(ctx context.Context, now time.Time) error {
	_, err := gen.New(s.Pool).RecordDatabaseSize(ctx, date(now))
	return err
}

func (s Store) Recent(ctx context.Context, now time.Time) ([]Sample, error) {
	rows, err := gen.New(s.Pool).ListDatabaseSizesSince(ctx, date(now.Add(-GrowthWindow)))
	if err != nil {
		return nil, err
	}
	samples := make([]Sample, len(rows))
	for i, row := range rows {
		samples[i] = Sample{Day: row.SampledOn.Time, Bytes: row.DatabaseBytes}
	}
	return samples, nil
}

func (s Store) PublishGauges(ctx context.Context) error {
	samples, err := s.Recent(ctx, time.Now())
	if err != nil {
		return err
	}
	budget := s.Rate.BudgetBytes()
	metrics.RestoreBudgetBytes.Set(float64(budget))
	metrics.RestoreBudgetDaysLeft.Set(DaysUntilBudget(samples, budget))
	if len(samples) > 0 {
		metrics.DatabaseBytes.Set(float64(samples[len(samples)-1].Bytes))
	}
	return nil
}

func (s Store) Report(ctx context.Context, now time.Time) (Report, error) {
	if err := s.RecordToday(ctx, now); err != nil {
		return Report{}, err
	}
	samples, err := s.Recent(ctx, now)
	if err != nil {
		return Report{}, err
	}
	tables, err := s.tableSizes(ctx)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		RestoreBytesPerSecond: s.Rate.BytesPerSecond,
		RestoreRateMeasured:   s.Rate.Measured,
		RestoreBudgetBytes:    s.Rate.BudgetBytes(),
		GrowthBytesPerDay:     growthPerDay(samples),
		Tables:                tables,
	}
	if len(samples) > 0 {
		report.DatabaseBytes = samples[len(samples)-1].Bytes
	}
	if days := DaysUntilBudget(samples, report.RestoreBudgetBytes); !math.IsInf(days, 1) {
		report.DaysUntilBudget = &days
	}
	return report, nil
}

func (s Store) tableSizes(ctx context.Context) ([]TableSize, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT coalesce(parent.relname, c.relname)::text, sum(pg_total_relation_size(c.oid))::bigint
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		LEFT JOIN pg_inherits i ON i.inhrelid = c.oid
		LEFT JOIN pg_class parent ON parent.oid = i.inhparent
		WHERE c.relkind = 'r'
		GROUP BY 1
		ORDER BY 2 DESC, 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tables := []TableSize{}
	for rows.Next() {
		var size TableSize
		if err := rows.Scan(&size.Table, &size.Bytes); err != nil {
			return nil, err
		}
		tables = append(tables, size)
	}
	return tables, rows.Err()
}

func date(t time.Time) pgtype.Date {
	t = t.UTC()
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}
