package reflection

import "github.com/jackc/pgx/v5/pgtype"

type CaseRates map[pgtype.UUID]float64

type PairedVerdict struct {
	Mean                  *float64
	Low, High             *float64
	Verdict               string
	AtLeastLikelyImproved bool
}

func keptCandidates(source CaseRates, candidates []CaseRates, dev []pgtype.UUID) []bool {
	kept := make([]bool, len(candidates))
	for _, testCaseID := range dev {
		best, found := bestRate(candidates, testCaseID)
		floor, judged := source[testCaseID]
		if !found || !judged || best <= floor {
			continue
		}
		for i, rates := range candidates {
			if rate, ok := rates[testCaseID]; ok && rate == best {
				kept[i] = true
			}
		}
	}
	return kept
}

func bestRate(candidates []CaseRates, testCaseID pgtype.UUID) (float64, bool) {
	best, found := 0.0, false
	for _, rates := range candidates {
		if rate, ok := rates[testCaseID]; ok && (!found || rate > best) {
			best, found = rate, true
		}
	}
	return best, found
}

func recommendedCandidate(standings []CandidateStanding, asked int) int {
	choice := -1
	if len(standings) < asked {
		return choice
	}
	for i, s := range standings {
		if s.BatchID.Valid && !s.Settled {
			return -1
		}
		verdict := s.HeldOut
		if !s.Kept || !verdict.AtLeastLikelyImproved || verdict.Mean == nil {
			continue
		}
		if choice < 0 || *verdict.Mean > *standings[choice].HeldOut.Mean {
			choice = i
		}
	}
	return choice
}
