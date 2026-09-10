package credit

import (
	"errors"
	"fmt"
	"math"
)

// ceilDiv is ceiling integer division for a non-negative numerator and a
// positive denominator: ceil(a/b), computed without floating point so the
// result is exact and deterministic (CRED-002).
//
// ADR-068 decision 6 requires every USD<->credit conversion in this package
// to round up, never down and never to nearest: rounding down or to nearest
// would, across millions of small paid calls, systematically leave a
// fraction of a cent of real spend uncharged on every one of them — a
// one-directional loss for the platform. Rounding up is a one-directional
// gain instead, and per ADR-068 the resulting error is negligible at the
// scale a single call ever costs.
func ceilDiv(a, b int64) int64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

// MaxBillableMicros bounds one charge at a thousand US dollars. Nothing this
// platform pays for comes close (a whole creation session is measured in cents,
// and one call is capped at MaxCallCostUSD), so a value above it is not an
// expensive call, it is a bug upstream — a dollar amount passed where micros
// were expected, or a gateway reporting nonsense.
//
// The bound exists because of where the arithmetic fails without it: usdMicros
// times markupBps is int64 multiplication, and past roughly 7.09e14 it wraps
// negative, where ceilDiv's non-positive guard returns 0. An enormous cost
// would be billed as free — the one direction ADR-068 decision 6 says this
// package must never round.
const MaxBillableMicros = 1_000_000_000

// ErrAmountOutOfRange is returned for a cost this package refuses to convert.
var ErrAmountOutOfRange = errors.New("credit: amount is outside the billable range")

// BilledMicros applies the basis-points markup to a raw platform cost
// (usdMicros, i.e. millionths of a US dollar), rounding up. markupBps
// 10000 is no markup; 13000 is ADR-068's shipped default (1.3x).
//
// An amount past MaxBillableMicros is an error, never a number: silently
// charging zero for it is the failure this signature exists to prevent.
func BilledMicros(usdMicros, markupBps int64) (int64, error) {
	if usdMicros < 0 || usdMicros > MaxBillableMicros {
		return 0, fmt.Errorf("%w: %d micros", ErrAmountOutOfRange, usdMicros)
	}
	if markupBps < 0 || markupBps > 1_000_000 {
		return 0, fmt.Errorf("%w: markup %d bps", ErrAmountOutOfRange, markupBps)
	}
	return ceilDiv(usdMicros*markupBps, 10000), nil
}

// CreditsForMicros converts an already-marked-up USD-micros amount into
// whole credits, rounding up: any nonzero spend costs at least one credit.
func CreditsForMicros(billedMicros, microsPerCredit int64) int64 {
	return ceilDiv(billedMicros, microsPerCredit)
}

// UsageCost turns a gateway usage report into the two cost_events columns
// that describe what a call cost: `usd_micros` and whether `cost_source` is
// 'gateway' or 'estimated'.
//
// costSource is the gateway's own word, and only its own word counts as a
// measurement. Every caller of this function already discards a cost that
// arrived with any other source (eval's judge.go and suggest.go nil the
// field out before storing it) for the reason ADR-026 gives about verdicts:
// a number is only as good as the ruler that produced it, and a price the
// gateway did not set is a guess wearing a price's clothes.
//
// A call the gateway did not price is recorded at zero micros and marked
// estimated — not dropped. Dropping it would make the platform's own spend
// ledger silently disagree with the number of paid calls it made, and the
// first place that would show up is the p95 the start gate is derived from
// (ADR-068 decision 8), which reads this table. Zero-and-labelled is
// recoverable arithmetic; a missing row is not.
//
// Rounds up, for the reason ceilDiv documents.
func UsageCost(costUSD *float64, costSource string) (usdMicros int64, estimated bool) {
	if costUSD == nil || costSource != "gateway" {
		return 0, true
	}
	v := *costUSD
	if math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 {
		// A non-finite or negative cost is not a cheaper call, it is a
		// broken report — same disposition as an absent one.
		return 0, true
	}
	micros := int64(math.Ceil(v * 1_000_000))
	if micros > MaxBillableMicros {
		// The caller of RecordCost cannot refuse a call that already
		// happened, so this clamps rather than errors, and the clamp is
		// visible: an estimated row at the ceiling is a row somebody will
		// ask about, which is the intent.
		return MaxBillableMicros, true
	}
	return micros, false
}
