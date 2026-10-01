package wiring

import "github.com/riverqueue/river"

const (
	QueueRuns  = "runs"
	QueueModel = "model"

	housekeepingWorkers = 4
	runWorkers          = 16
	modelWorkers        = 8
)

var Queues = map[string]river.QueueConfig{
	river.QueueDefault: {MaxWorkers: housekeepingWorkers},
	QueueRuns:          {MaxWorkers: runWorkers},
	QueueModel:         {MaxWorkers: modelWorkers},
}
