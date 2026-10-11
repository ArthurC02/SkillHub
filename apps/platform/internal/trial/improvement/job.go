package eval

type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (JobArgs) Kind() string { return "evaluate_run" }

type BatchAdvanceArgs struct {
	WorkspaceID string `json:"workspace_id,omitempty"`
}

func (BatchAdvanceArgs) Kind() string { return "advance_evaluation_batches" }
