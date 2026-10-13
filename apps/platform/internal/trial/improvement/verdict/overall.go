package verdict

type Overall string

const (
	OverallMet          Overall = "met"
	OverallPartiallyMet Overall = "partially_met"
	OverallNotMet       Overall = "not_met"
	OverallUndetermined Overall = "undetermined"
)

func AllOveralls() []Overall {
	return []Overall{OverallMet, OverallPartiallyMet, OverallNotMet, OverallUndetermined}
}

func OverallFrom(results []CriterionResult) Overall {
	if len(results) == 0 {
		return OverallUndetermined
	}
	passed, failed, _ := Tally(results)
	switch {
	case passed == len(results):
		return OverallMet
	case failed == len(results):
		return OverallNotMet
	case passed > 0:
		return OverallPartiallyMet
	case failed > 0:
		return OverallNotMet
	default:
		return OverallUndetermined
	}
}

func Tally(results []CriterionResult) (passed, failed, undetermined int) {
	for _, r := range results {
		switch r.Result {
		case ResultPassed:
			passed++
		case ResultFailed:
			failed++
		default:
			undetermined++
		}
	}
	return passed, failed, undetermined
}
