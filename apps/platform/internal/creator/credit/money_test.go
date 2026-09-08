package credit

import (
	"errors"
	"testing"
)

func TestBilledMicrosRoundsUpNeverDown(t *testing.T) {
	cases := []struct {
		usdMicros, markupBps, want int64
	}{
		{1_000_000, 13000, 1_300_000}, // $1 at 1.3x, exact
		{1, 13000, 2},                 // 0.0000013 must round up to 2, not truncate to 1
		{10000, 10000, 10000},         // no markup, exact
		{0, 13000, 0},
	}
	for _, c := range cases {
		got, err := BilledMicros(c.usdMicros, c.markupBps)
		if err != nil || got != c.want {
			t.Errorf("BilledMicros(%d, %d) = %d (%v), want %d", c.usdMicros, c.markupBps, got, err, c.want)
		}
	}
}

func TestCreditsForMicrosRoundsUp(t *testing.T) {
	cases := []struct {
		billed, micros, want int64
	}{
		{1_300_000, 1000, 1300}, // exact
		{1, 1000, 1},            // any positive spend costs at least 1 credit
		{1000, 1000, 1},
		{1001, 1000, 2},
		{0, 1000, 0},
	}
	for _, c := range cases {
		if got := CreditsForMicros(c.billed, c.micros); got != c.want {
			t.Errorf("CreditsForMicros(%d, %d) = %d, want %d", c.billed, c.micros, got, c.want)
		}
	}
}

// TestCeilingNeverUndercharges is the direction-sensitive check ADR-068
// decision 6 exists for: swapping ceilDiv for a naive truncating division
// (a+b-1 dropped, i.e. plain a/b) must turn this red, because 13
// microdollars marked up 1.3x and converted at $0.001/credit would then
// price at 0 credits instead of 1 — a nonzero real cost silently charged as
// free.
func TestCeilingNeverUndercharges(t *testing.T) {
	billed, err := BilledMicros(1, 13000)
	if err != nil {
		t.Fatal(err)
	}
	if got := CreditsForMicros(billed, 1000); got != 1 {
		t.Fatalf("a nonzero cost must never convert to zero credits, got %d", got)
	}
}

// An amount that cannot be multiplied by the markup without wrapping int64 is
// the one case where returning a number would be worse than returning an error:
// the wrapped product is negative, ceilDiv's guard turns it into 0, and an
// enormous cost bills as free. Found by the adversarial review of this batch.
func TestAnAmountTooLargeToPriceIsAnErrorNotZero(t *testing.T) {
	if _, err := BilledMicros(MaxBillableMicros+1, 13000); !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("an out-of-range amount must not be priced: %v", err)
	}
	// The value that used to wrap: 7.1e14 micros times 13000 overflows int64.
	if _, err := BilledMicros(710_000_000_000_000, 13000); !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("the overflowing amount must be refused, not billed as zero: %v", err)
	}
	if _, err := BilledMicros(-1, 13000); !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("a negative amount must be refused: %v", err)
	}
	got, err := BilledMicros(MaxBillableMicros, 13000)
	if err != nil || got <= 0 {
		t.Fatalf("the largest billable amount must still price: %d %v", got, err)
	}
}
