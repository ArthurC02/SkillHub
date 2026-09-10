package credit

const MinStatSamples = 20

type Statistics struct {
	SampleCount  int
	P50UsdMicros int64
	P90UsdMicros int64
	P95UsdMicros int64
	MaxUsdMicros int64
}
