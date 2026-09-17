package eval

type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

func AllStatuses() []Status {
	return []Status{StatusPending, StatusCompleted, StatusFailed}
}

func (s Status) AwaitsTheJudge() bool { return s == StatusPending }
