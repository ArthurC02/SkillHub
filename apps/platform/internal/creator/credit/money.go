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

const MaxMarkupBps = 1_000_000

var ErrAmountOutOfRange = errors.New("credit: amount is outside the billable range")

func BilledMicros(usdMicros, markupBps int64) (int64, error) {
	if usdMicros < 0 || usdMicros > MaxBillableMicros {
		return 0, fmt.Errorf("%w: %d micros", ErrAmountOutOfRange, usdMicros)
	}
	if markupBps < 0 || markupBps > MaxMarkupBps {
		return 0, fmt.Errorf("%w: markup %d bps", ErrAmountOutOfRange, markupBps)
	}
	return ceilDiv(usdMicros*markupBps, 10000), nil
}

func CreditsForMicros(billedMicros, microsPerCredit int64) int64 {
	return ceilDiv(billedMicros, microsPerCredit)
}

func BillableMicros(usd float64) (usdMicros int64, exact bool) {
	if math.IsNaN(usd) || math.IsInf(usd, 0) || usd <= 0 {
		return 0, false
	}
	// Compared before the conversion: converting a float past int64's range
	// is implementation-defined and can land back inside the billable range.
	scaled := usd * 1_000_000
	if scaled > float64(MaxBillableMicros) {
		return MaxBillableMicros, false
	}
	return int64(math.Ceil(scaled)), true
}

func UsageCost(costUSD *float64) (usdMicros int64, estimated bool) {
	if costUSD == nil {
		return 0, true
	}
	micros, exact := BillableMicros(*costUSD)
	return micros, !exact
}
