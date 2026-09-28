package creation

type evaluationStatus string

const evaluationCompleted evaluationStatus = "completed"

type evaluationOverall string

const overallMet evaluationOverall = "met"

type criterionResult string

const (
	criterionFailed       criterionResult = "failed"
	criterionUndetermined criterionResult = "undetermined"
)
