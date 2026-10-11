package eval

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

type BatchComparison struct {
	Baseline        Batch
	Candidate       Batch
	Paired          int
	Unpaired        int
	Difference      *float64
	Difference95    *Interval
	CandidateBetter int
	BaselineBetter  int
	Tied            int
}

func (s *BatchService) CompareBatches(ctx context.Context, workspaceID, skillID, candidateID, baselineID pgtype.UUID) (BatchComparison, error) {
	candidate, err := s.BatchReport(ctx, workspaceID, skillID, candidateID)
	if err != nil {
		return BatchComparison{}, err
	}
	baseline, err := s.BatchReport(ctx, workspaceID, skillID, baselineID)
	if err != nil {
		return BatchComparison{}, err
	}
	comparison := BatchComparison{Baseline: baseline.Batch, Candidate: candidate.Batch}
	baseRates := judgedRates(baseline)
	var differences []float64
	for testCaseID, candRate := range judgedRates(candidate) {
		baseRate, both := baseRates[testCaseID]
		if !both {
			continue
		}
		differences = append(differences, candRate-baseRate)
	}
	comparison.Paired = len(differences)
	comparison.Unpaired = len(unionOfTestCases(candidate, baseline)) - comparison.Paired
	for _, d := range differences {
		switch {
		case d > 0:
			comparison.CandidateBetter++
		case d < 0:
			comparison.BaselineBetter++
		default:
			comparison.Tied++
		}
	}
	comparison.Difference, comparison.Difference95 = pairedDifference(differences)
	return comparison, nil
}

func judgedRates(report BatchReport) map[pgtype.UUID]float64 {
	rates := make(map[pgtype.UUID]float64, len(report.TestCases))
	for _, tc := range report.TestCases {
		if tc.Judged > 0 {
			rates[tc.TestCaseID] = float64(tc.Passed) / float64(tc.Judged)
		}
	}
	return rates
}

func unionOfTestCases(reports ...BatchReport) map[pgtype.UUID]bool {
	union := map[pgtype.UUID]bool{}
	for _, report := range reports {
		for _, tc := range report.TestCases {
			union[tc.TestCaseID] = true
		}
	}
	return union
}

func pairedDifference(differences []float64) (*float64, *Interval) {
	n := float64(len(differences))
	if n == 0 {
		return nil, nil
	}
	var sum float64
	for _, d := range differences {
		sum += d
	}
	mean := sum / n
	if n < 2 {
		return &mean, nil
	}
	var squares float64
	for _, d := range differences {
		squares += (d - mean) * (d - mean)
	}
	standardError := math.Sqrt(squares/(n-1)) / math.Sqrt(n)
	return &mean, &Interval{Low: mean - normalZ95*standardError, High: mean + normalZ95*standardError}
}
