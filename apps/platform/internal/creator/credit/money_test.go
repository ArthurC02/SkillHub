package credit

import (
	"errors"
	"math"
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

func TestAFigureTooLargeToScaleIsCappedNotWrapped(t *testing.T) {
	for _, tc := range []struct {
		name string
		usd  float64
	}{
		{"one micro past the ceiling", float64(MaxBillableMicros+1) / 1_000_000},
		{"past what a micro count can hold", float64(math.MaxInt64) / 1_000_000 * 2},
		{"astronomically large but finite", 1e300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			micros, exact := BillableMicros(tc.usd)
			if micros != MaxBillableMicros || exact {
				t.Errorf("BillableMicros(%v) = %d / %v, want %d / false: a figure that cannot be scaled "+
					"must cap at the ceiling, never wrap to a negative one", tc.usd, micros, exact, MaxBillableMicros)
			}
		})
	}
}

func TestUsageCostBillsOnlyAPositiveFiniteFigure(t *testing.T) {
	usd := func(f float64) *float64 { return &f }
	for _, tc := range []struct {
		name          string
		cost          *float64
		wantMicros    int64
		wantEstimated bool
	}{
		{"no figure", nil, 0, true},
		{"zero", usd(0), 0, true},
		{"negative", usd(-0.01), 0, true},
		{"not a number", usd(math.NaN()), 0, true},
		{"infinite", usd(math.Inf(1)), 0, true},
		{"a fraction of a micro rounds up", usd(0.0000001), 1, false},
		{"an ordinary figure", usd(0.0031), 3100, false},
		{"exactly the billable ceiling", usd(1000), MaxBillableMicros, false},
		{"just past the billable ceiling", usd(1000.001), MaxBillableMicros, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			micros, estimated := UsageCost(tc.cost)
			if micros != tc.wantMicros || estimated != tc.wantEstimated {
				t.Errorf("UsageCost = %d / %v, want %d / %v", micros, estimated, tc.wantMicros, tc.wantEstimated)
			}
		})
	}
}
