package credit

// MinStatSamples is the sample-count floor below which gate ①'s threshold
// falls back to Config.StartFallbackCredits instead of the measured p95
// (ADR-068 decision 8). Not env-configurable: this is the statistical
// judgment call itself ("a p95 of fewer than 20 points is not reliable"),
// not a per-deployment knob like the four CREDIT_* settings.
const MinStatSamples = 20

// Statistics is one cost_statistics row (migration 0060): a rolling
// window's cost distribution for one kind of session or call, in
// USD-micros. The percentiles themselves are computed in SQL
// (db/queries/cost.sql's AggregateCostEventsWindow, using Postgres'
// percentile_cont) — this package does not reimplement that math in Go, so
// there is exactly one definition of "p95" in the system, not two that can
// silently disagree.
type Statistics struct {
	SampleCount  int
	P50UsdMicros int64
	P90UsdMicros int64
	P95UsdMicros int64
	MaxUsdMicros int64
}
