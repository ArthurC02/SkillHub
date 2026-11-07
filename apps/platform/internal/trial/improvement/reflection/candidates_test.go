package reflection

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func caseID(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{b}, Valid: true} }

func TestACandidateIsKeptOnlyWhenItLeadsADevCaseAndBeatsTheSourceThere(t *testing.T) {
	a, b, held := caseID(1), caseID(2), caseID(9)
	dev := []pgtype.UUID{a, b}
	for _, tc := range []struct {
		name       string
		source     CaseRates
		candidates []CaseRates
		want       []bool
	}{
		{"the only leader above the source", CaseRates{a: 0.5}, []CaseRates{{a: 1}, {a: 0.5}}, []bool{true, false}},
		{"two leaders tied above the source", CaseRates{a: 0.25}, []CaseRates{{a: 0.75}, {a: 0.75}}, []bool{true, true}},
		{"a leader tied with the source", CaseRates{a: 0.5}, []CaseRates{{a: 0.5}, {a: 0.25}}, []bool{false, false}},
		{"each leads a different dev case", CaseRates{a: 0, b: 0}, []CaseRates{{a: 1, b: 0.5}, {a: 0.5, b: 1}}, []bool{true, true}},
		{"leading only a held-out case", CaseRates{a: 0.5, held: 0}, []CaseRates{{a: 0.5, held: 1}, {a: 0.25, held: 0}}, []bool{false, false}},
		{"the source has no judged trial on the case", CaseRates{}, []CaseRates{{a: 1}}, []bool{false}},
		{"a candidate with no judged trial", CaseRates{a: 0}, []CaseRates{nil, {a: 0.5}}, []bool{false, true}},
		{"below another candidate everywhere", CaseRates{a: 0, b: 0}, []CaseRates{{a: 1, b: 1}, {a: 0.5, b: 0.5}}, []bool{true, false}},
	} {
		if got := keptCandidates(tc.source, tc.candidates, dev); !slices.Equal(got, tc.want) {
			t.Errorf("%s: kept %v, want %v", tc.name, got, tc.want)
		}
	}
}

func mean(v float64) *float64 { return &v }

func TestOnlyAKeptCandidateLikelyImprovedOnHeldOutCasesIsRecommended(t *testing.T) {
	improved := func(m float64) PairedVerdict {
		return PairedVerdict{Mean: mean(m), Verdict: "likely_improved", AtLeastLikelyImproved: true}
	}
	unclear := PairedVerdict{Mean: mean(0.4), Verdict: "no_clear_difference"}
	settled := func(kept bool, heldOut PairedVerdict) CandidateStanding {
		return CandidateStanding{CandidateBatch: eval.CandidateBatch{BatchID: pgtype.UUID{Valid: true}}, Settled: true, Kept: kept, HeldOut: heldOut}
	}
	running := settled(true, improved(0.5))
	running.Settled = false
	unavailable := CandidateStanding{}
	for _, tc := range []struct {
		name      string
		standings []CandidateStanding
		asked     int
		want      int
	}{
		{"the higher held-out mean of two", []CandidateStanding{settled(true, improved(0.2)), settled(true, improved(0.3))}, 2, 1},
		{"equal means take the earlier candidate", []CandidateStanding{settled(true, improved(0.2)), settled(true, improved(0.2))}, 2, 0},
		{"improved but dominated on dev", []CandidateStanding{settled(false, improved(0.5)), settled(true, improved(0.1))}, 2, 1},
		{"kept with no clear difference", []CandidateStanding{settled(true, unclear)}, 1, -1},
		{"a batch has not settled", []CandidateStanding{settled(true, improved(0.9)), running}, 2, -1},
		{"a candidate not yet recorded", []CandidateStanding{settled(true, improved(0.5))}, 2, -1},
		{"an unavailable candidate does not hold the round back", []CandidateStanding{unavailable, settled(true, improved(0.1))}, 2, 1},
		{"improved with no paired case", []CandidateStanding{settled(true, PairedVerdict{AtLeastLikelyImproved: true})}, 1, -1},
	} {
		if got := recommendedCandidate(tc.standings, tc.asked); got != tc.want {
			t.Errorf("%s: recommended %d, want %d", tc.name, got, tc.want)
		}
	}
}
