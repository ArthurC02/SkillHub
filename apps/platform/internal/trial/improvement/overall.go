package eval

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
