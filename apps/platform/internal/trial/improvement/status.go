package eval

import "slices"

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

func AllStatuses() []Status {
	return []Status{StatusPending, StatusCompleted, StatusFailed}
}

var statusSuccessors = map[Status][]Status{
	StatusPending: {StatusCompleted, StatusFailed},
}

func ParseStatus(s string) (Status, bool) {
	candidate := Status(s)
	if slices.Contains(AllStatuses(), candidate) {
		return candidate, true
	}
	return "", false
}

func (s Status) AwaitsTheJudge() bool { return s == StatusPending }

func CanTransitionStatus(from, to Status) bool {
	if _, known := ParseStatus(string(from)); !known {
		return false
	}
	if _, known := ParseStatus(string(to)); !known {
		return false
	}
	if from == to {
		return true
	}
	return slices.Contains(statusSuccessors[from], to)
}
