package batch

import (
	"context"
	"math"

	"github.com/jackc/pgx/v5/pgtype"
)

type BatchComparison struct {
	Reference Batch
	Candidate Batch
	PairedDifference
}

type PairedDifference struct {
	Paired       int
	Unpaired     int
	Difference   *float64
	Difference95 *Interval
	Verdict      string
	Better       int
	Worse        int
	Tied         int
}

func (s *BatchService) CompareBatches(ctx context.Context, workspaceID, skillID, candidateID, referenceID pgtype.UUID) (BatchComparison, error) {
	candidate, err := s.BatchReport(ctx, workspaceID, skillID, candidateID)
	if err != nil {
		return BatchComparison{}, err
	}
	reference, err := s.BatchReport(ctx, workspaceID, skillID, referenceID)
	if err != nil {
		return BatchComparison{}, err
	}
	return BatchComparison{
		Reference: reference.Batch, Candidate: candidate.Batch,
		PairedDifference: PairArms(candidate.ArmReport, reference.ArmReport),
	}, nil
}

func PairArms(first, second ArmReport) PairedDifference {
	var p PairedDifference
	secondRates := JudgedRates(second)
	var differences []float64
	for testCaseID, firstRate := range JudgedRates(first) {
		secondRate, both := secondRates[testCaseID]
		if !both {
			continue
		}
		differences = append(differences, firstRate-secondRate)
	}
	p.Paired = len(differences)
	p.Unpaired = len(unionOfTestCases(first, second)) - p.Paired
	for _, d := range differences {
		switch {
		case d > 0:
			p.Better++
		case d < 0:
			p.Worse++
		default:
			p.Tied++
		}
	}
	p.Difference, p.Difference95, p.Verdict = pairedDifference(differences)
	return p
}

func JudgedRates(report ArmReport) map[pgtype.UUID]float64 {
	rates := make(map[pgtype.UUID]float64, len(report.TestCases))
	for _, tc := range report.TestCases {
		if tc.Judged > 0 {
			rates[tc.TestCaseID] = float64(tc.Passed) / float64(tc.Judged)
		}
	}
	return rates
}

func unionOfTestCases(reports ...ArmReport) map[pgtype.UUID]bool {
	union := map[pgtype.UUID]bool{}
	for _, report := range reports {
		for _, tc := range report.TestCases {
			union[tc.TestCaseID] = true
		}
	}
	return union
}

const normalZ999 = 3.290526731491926

var (
	studentT975 = [...]float64{12.706, 4.303, 3.182, 2.776, 2.571, 2.447, 2.365, 2.306, 2.262, 2.228,
		2.201, 2.179, 2.160, 2.145, 2.131, 2.120, 2.110, 2.101, 2.093, 2.086,
		2.080, 2.074, 2.069, 2.064, 2.060, 2.056, 2.052, 2.048, 2.045, 2.042}
	studentT9995 = [...]float64{636.619, 31.599, 12.924, 8.610, 6.869, 5.959, 5.408, 5.041, 4.781, 4.587,
		4.437, 4.318, 4.221, 4.140, 4.073, 4.015, 3.965, 3.922, 3.883, 3.850,
		3.819, 3.792, 3.768, 3.745, 3.725, 3.707, 3.690, 3.674, 3.659, 3.646}
)

type criticalValues struct{ at95, at999 float64 }

func pairedCriticalValues(pairs int) criticalValues {
	df := pairs - 1
	if df < 1 || df > len(studentT975) {
		return criticalValues{normalZ95, normalZ999}
	}
	return criticalValues{studentT975[df-1], studentT9995[df-1]}
}

const (
	VerdictImproved          = "improved"
	VerdictLikelyImproved    = "likely_improved"
	VerdictNoClearDifference = "no_clear_difference"
	VerdictLikelyWorse       = "likely_worse"
	VerdictWorse             = "worse"
	VerdictTooFewPairs       = "too_few_pairs"
)

func pairedDifference(differences []float64) (*float64, *Interval, string) {
	n := float64(len(differences))
	if n == 0 {
		return nil, nil, VerdictTooFewPairs
	}
	var sum float64
	for _, d := range differences {
		sum += d
	}
	mean := sum / n
	if n < 2 {
		return &mean, nil, VerdictTooFewPairs
	}
	var squares float64
	for _, d := range differences {
		squares += (d - mean) * (d - mean)
	}
	standardError := math.Sqrt(squares/(n-1)) / math.Sqrt(n)
	critical := pairedCriticalValues(len(differences))
	interval := &Interval{Low: mean - critical.at95*standardError, High: mean + critical.at95*standardError}
	return &mean, interval, differenceVerdict(mean, standardError, critical)
}

func differenceVerdict(mean, standardError float64, critical criticalValues) string {
	switch {
	case mean-critical.at999*standardError > 0:
		return VerdictImproved
	case mean-critical.at95*standardError > 0:
		return VerdictLikelyImproved
	case mean+critical.at999*standardError < 0:
		return VerdictWorse
	case mean+critical.at95*standardError < 0:
		return VerdictLikelyWorse
	}
	return VerdictNoClearDifference
}

func (r ArmReport) On(testCaseIDs map[pgtype.UUID]bool) ArmReport {
	var only ArmReport
	for _, tc := range r.TestCases {
		if testCaseIDs[tc.TestCaseID] {
			only.TestCases = append(only.TestCases, tc)
		}
	}
	return only
}
