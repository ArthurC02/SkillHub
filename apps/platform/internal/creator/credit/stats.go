package credit

import "time"

const MinStatSamples = 20

const MaxStatisticsAge = 48 * time.Hour

type Statistics struct {
	SampleCount  int
	P50UsdMicros int64
	P90UsdMicros int64
	P95UsdMicros int64
	MaxUsdMicros int64
	WindowEnd    time.Time
}
