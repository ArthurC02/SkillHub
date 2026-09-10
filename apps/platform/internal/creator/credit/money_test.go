package credit

import (
	"errors"
	"testing"
)

func TestBilledMicrosRoundsUpNeverDown(t *testing.T) {
	cases := []struct {
		usdMicros, markupBps, want int64
	}{
		{1_000_000, 13000, 1_300_000},
		{1, 13000, 2},
		{10000, 10000, 10000},
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
		{1_300_000, 1000, 1300},
		{1, 1000, 1},
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

func TestCeilingNeverUndercharges(t *testing.T) {
	billed, err := BilledMicros(1, 13000)
	if err != nil {
		t.Fatal(err)
	}
	if got := CreditsForMicros(billed, 1000); got != 1 {
		t.Fatalf("a nonzero cost must never convert to zero credits, got %d", got)
	}
}

func TestAnAmountTooLargeToPriceIsAnErrorNotZero(t *testing.T) {
	if _, err := BilledMicros(MaxBillableMicros+1, 13000); !errors.Is(err, ErrAmountOutOfRange) {
		t.Fatalf("an out-of-range amount must not be priced: %v", err)
	}

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
