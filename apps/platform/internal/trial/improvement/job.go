package eval

type JobArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (JobArgs) Kind() string { return "evaluate_run" }
