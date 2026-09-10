package credit

import (
	"errors"
	"fmt"
	"math"
)

func ceilDiv(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

// usdMicros*markupBps is int64 multiplication that wraps negative past
// roughly 7.09e14; ceilDiv's non-positive guard would then return 0,
// billing an oversized cost as free instead of refusing it.
const MaxBillableMicros = 1_000_000_000

var ErrAmountOutOfRange = errors.New("credit: amount is outside the billable range")

func BilledMicros(usdMicros, markupBps int64) (int64, error) {
	if usdMicros < 0 || usdMicros > MaxBillableMicros {
		return 0, fmt.Errorf("%w: %d micros", ErrAmountOutOfRange, usdMicros)
	}
	if markupBps < 0 || markupBps > 1_000_000 {
		return 0, fmt.Errorf("%w: markup %d bps", ErrAmountOutOfRange, markupBps)
	}
	return ceilDiv(usdMicros*markupBps, 10000), nil
}

func CreditsForMicros(billedMicros, microsPerCredit int64) int64 {
	return ceilDiv(billedMicros, microsPerCredit)
}

func UsageCost(costUSD *float64, costSource string) (usdMicros int64, estimated bool) {
	if costUSD == nil || costSource != "gateway" {
		return 0, true
	}
	v := *costUSD
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {

		return 0, true
	}
	micros := int64(math.Ceil(v * 1_000_000))
	if micros > MaxBillableMicros {

		return MaxBillableMicros, true
	}
	return micros, false
}
