package reflection

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func uuidN(n byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{n}, Valid: true} }

func settled(ids ...byte) []BatchTrial {
	out := make([]BatchTrial, len(ids))
	for i, id := range ids {
		out[i] = BatchTrial{EndRunID: uuidN(id), Settled: true}
	}
	return out
}

func TestTheReadingOrderTakesEveryCasesFirstTrialBeforeAnySecond(t *testing.T) {
	got := readingOrder([][]BatchTrial{settled(11, 12, 13), settled(21), settled(31, 32)})
	want := []pgtype.UUID{uuidN(11), uuidN(21), uuidN(31), uuidN(12), uuidN(32), uuidN(13)}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order[%d] = %v, want %v (whole: %v)", i, got[i], want[i], got)
		}
	}
}

func TestAReflectionReadsEightFailingRunsAndCountsTheRest(t *testing.T) {
	for _, tc := range []struct{ failing, read, notRead int }{
		{1, 1, 0}, {8, 8, 0}, {9, 8, 1},
	} {
		read, notRead := readable(make([]eval.RunFailure, tc.failing))
		if len(read) != tc.read || notRead != tc.notRead {
			t.Errorf("%d failing: read %d, not read %d; want %d and %d", tc.failing, len(read), notRead, tc.read, tc.notRead)
		}
	}
}

func TestTheReflectionDigestSaysWhenItsReadingIsPartial(t *testing.T) {
	partial := digest(8, 3, 5, []string{"body"})
	if !strings.Contains(partial, "3 further failing runs were not read") || !strings.Contains(partial, "reads 8 failing runs from 5 development test cases") {
		t.Errorf("partial digest = %q", partial)
	}
	if !strings.Contains(partial, "## failing run 1 of 1\nbody") {
		t.Errorf("the run section is missing from %q", partial)
	}
	whole := digest(2, 0, 2, nil)
	if strings.Contains(whole, "not read") {
		t.Errorf("a complete reading says something was left out: %q", whole)
	}
}

func TestADevelopmentSetIsAcceptedOnlyWhenItIsAProperSubsetOfSettledCases(t *testing.T) {
	a, b, c := uuidN(1), uuidN(2), uuidN(3)
	batch := BatchTrials{TestCases: []BatchTestCaseTrials{
		{TestCaseID: a, Trials: settled(11, 12)},
		{TestCaseID: b, Trials: []BatchTrial{{EndRunID: uuidN(21), Settled: true}, {Settled: false}}},
		{TestCaseID: c, Trials: settled(31)},
	}}
	for _, tc := range []struct {
		name  string
		batch BatchTrials
		dev   []pgtype.UUID
		want  error
	}{
		{"one settled case", batch, []pgtype.UUID{a}, nil},
		{"all but one", batch, []pgtype.UUID{a, c}, nil},
		{"no case", batch, nil, ErrInvalid},
		{"every case", batch, []pgtype.UUID{a, b, c}, ErrInvalid},
		{"a foreign case", batch, []pgtype.UUID{uuidN(9)}, ErrInvalid},
		{"a case twice", batch, []pgtype.UUID{a, a}, ErrInvalid},
		{"a case with an unsettled trial", batch, []pgtype.UUID{b}, ErrNotReady},
		{"a launching batch", BatchTrials{Launching: true, TestCases: batch.TestCases}, []pgtype.UUID{a}, ErrNotReady},
	} {
		got, err := devTrials(tc.batch, tc.dev)
		if !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err = %v, want %v", tc.name, err, tc.want)
			continue
		}
		if tc.want == nil && len(got) != len(tc.dev) {
			t.Errorf("%s: %d cases kept, want %d", tc.name, len(got), len(tc.dev))
		}
	}
}
