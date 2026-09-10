package sandbox

import "time"

type RunState string

const (
	StateCreating  RunState = "creating"
	StateRunning   RunState = "running"
	StateCompleted RunState = "completed"
	StateFailed    RunState = "failed"
	StateCancelled RunState = "cancelled"
)

func (s RunState) Terminal() bool {
	return s == StateCompleted || s == StateFailed || s == StateCancelled
}

type ResultStatus string

const (
	ResultSucceeded ResultStatus = "succeeded"
	ResultFailed    ResultStatus = "failed"
	ResultCancelled ResultStatus = "cancelled"
	ResultTimedOut  ResultStatus = "timed_out"
)

const exitTokenBudget = 9

const (
	ClassProvision          = "provision"
	ClassExecution          = "execution"
	ClassCleanup            = "cleanup"
	ClassCapabilityMismatch = "capability_mismatch"
	ClassCancelled          = "cancelled"
	ClassTimeout            = "timeout"
)

type RunRequest struct {
	RunID          string              `json:"run_id"`
	RunAttemptID   string              `json:"run_attempt_id"`
	Attempt        int                 `json:"attempt"`
	WorkspaceID    string              `json:"workspace_id"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	SkillVersion   PackageRef          `json:"skill_version"`
	TestCase       TestCaseSnapshotRef `json:"test_case_snapshot"`
	Runtime        RuntimeProfile      `json:"runtime"`
	ResourceLimits ResourceLimits      `json:"resource_limits"`
	Egress         EgressPolicy        `json:"egress"`
	ObjectGrants   []ObjectGrant       `json:"object_grants,omitempty"`
	ModelGateway   *ModelGatewayGrant  `json:"model_gateway,omitempty"`
	Trace          TracePolicy         `json:"trace"`
	Extensions     map[string]any      `json:"provider_extensions,omitempty"`
}

type PackageRef struct {
	SkillVersionID string `json:"skill_version_id"`
	ContentHash    string `json:"content_hash"`
	ObjectKey      string `json:"object_key,omitempty"`
}

type TestCaseSnapshotRef struct {
	SnapshotID  string       `json:"test_case_snapshot_id"`
	ContentHash string       `json:"content_hash"`
	UserPrompt  string       `json:"user_prompt"`
	DatasetRefs []DatasetRef `json:"dataset_refs,omitempty"`
}

type DatasetRef struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`
	ObjectKey   string `json:"object_key,omitempty"`
}

type RuntimeProfile struct {
	Runtime          string `json:"runtime"`
	RuntimeVersion   string `json:"runtime_version"`
	Model            string `json:"model,omitempty"`
	AgentIntegration string `json:"agent_integration,omitempty"`
}

type ResourceLimits struct {
	VCPU                 float64      `json:"vcpu"`
	MemoryBytes          int64        `json:"memory_bytes"`
	DiskBytes            int64        `json:"disk_bytes"`
	MaxPIDs              int64        `json:"max_pids"`
	MaxOpenFiles         int64        `json:"max_open_files"`
	WallClockSoftSeconds int          `json:"wall_clock_soft_seconds"`
	WallClockHardSeconds int          `json:"wall_clock_hard_seconds"`
	ArtifactTotalBytes   int64        `json:"artifact_total_bytes,omitempty"`
	ArtifactFileBytes    int64        `json:"artifact_file_bytes,omitempty"`
	TokenBudget          *TokenBudget `json:"token_budget,omitempty"`
}

var DefaultLimits = ResourceLimits{
	VCPU:                 2,
	MemoryBytes:          4 << 30,
	DiskBytes:            8 << 30,
	MaxPIDs:              256,
	MaxOpenFiles:         1024,
	WallClockSoftSeconds: 600,
	WallClockHardSeconds: 900,
	ArtifactTotalBytes:   100 << 20,
	ArtifactFileBytes:    25 << 20,

	TokenBudget: &TokenBudget{MaxInputTokens: 300_000, MaxOutputTokens: 60_000},
}

type TokenBudget struct {
	MaxInputTokens  int64 `json:"max_input_tokens,omitempty"`
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

type EgressPolicy struct {
	Mode  string             `json:"mode"`
	Allow []EgressAllowEntry `json:"allow"`
}

type EgressAllowEntry struct {
	Purpose string `json:"purpose"`
	URL     string `json:"url"`
}

type ObjectGrant struct {
	Purpose   string    `json:"purpose"`
	ObjectKey string    `json:"object_key"`
	Access    string    `json:"access"`
	URL       string    `json:"url,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ModelGatewayGrant struct {
	BaseURL      string    `json:"base_url"`
	VirtualKey   string    `json:"virtual_key,omitempty"`
	MaxBudgetUSD float64   `json:"max_budget_usd,omitempty"`
	TPMLimit     int       `json:"tpm_limit,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type TracePolicy struct {
	Level        string `json:"level"`
	IngestionURL string `json:"ingestion_url,omitempty"`
}

type ProviderRun struct {
	RunID             string     `json:"run_id"`
	RunAttemptID      string     `json:"run_attempt_id"`
	Provider          string     `json:"provider"`
	ProviderRunID     string     `json:"provider_run_id"`
	State             RunState   `json:"state"`
	StateReason       string     `json:"state_reason,omitempty"`
	CancelRequestedAt time.Time  `json:"cancel_requested_at,omitzero"`
	CreatedAt         time.Time  `json:"created_at,omitzero"`
	StartedAt         time.Time  `json:"started_at,omitzero"`
	FinishedAt        time.Time  `json:"finished_at,omitzero"`
	ObservedAt        time.Time  `json:"observed_at"`
	Result            *RunResult `json:"result,omitempty"`
}

type ProviderRunList struct {
	Provider   string        `json:"provider"`
	Runs       []ProviderRun `json:"runs"`
	ObservedAt time.Time     `json:"observed_at"`
}

type RunResult struct {
	RunID              string         `json:"run_id"`
	RunAttemptID       string         `json:"run_attempt_id"`
	ProviderRunID      string         `json:"provider_run_id,omitempty"`
	Status             ResultStatus   `json:"status"`
	StartedAt          time.Time      `json:"started_at,omitzero"`
	FinishedAt         time.Time      `json:"finished_at,omitzero"`
	AgentOutput        string         `json:"agent_output,omitempty"`
	Artifacts          []Artifact     `json:"artifacts,omitempty"`
	ArtifactsTruncated bool           `json:"artifacts_truncated,omitempty"`
	Usage              *RunUsage      `json:"usage,omitempty"`
	Error              *RunError      `json:"error,omitempty"`
	Diagnostics        map[string]any `json:"provider_diagnostics,omitempty"`
}

type Artifact struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	ObjectKey   string `json:"object_key,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type RunUsage struct {
	InputTokens      int64   `json:"input_tokens,omitempty"`
	OutputTokens     int64   `json:"output_tokens,omitempty"`
	ModelCostUSD     float64 `json:"model_cost_usd,omitempty"`
	WallClockSeconds float64 `json:"wall_clock_seconds,omitempty"`
	PeakMemoryBytes  int64   `json:"peak_memory_bytes,omitempty"`
}

type RunError struct {
	Class     string `json:"class"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

func (e *RunError) Error() string { return e.Class + ": " + e.Message }

type ProviderCapability struct {
	Provider     string              `json:"provider"`
	Runtimes     []RuntimeCapability `json:"runtimes"`
	MaxResources ResourceLimits      `json:"max_resources"`

	MaxResourcesUnenforced []string            `json:"max_resources_unenforced,omitempty"`
	Isolation              Isolation           `json:"isolation"`
	Network                *NetworkCapability  `json:"network,omitempty"`
	Features               *Features           `json:"features,omitempty"`
	Regions                []string            `json:"regions,omitempty"`
	Availability           *Availability       `json:"availability,omitempty"`
	Security               *SecurityCapability `json:"security,omitempty"`
}

type RuntimeCapability struct {
	Runtime          string   `json:"runtime"`
	Versions         []string `json:"versions"`
	AgentIntegration []string `json:"agent_integration,omitempty"`
}

type Isolation struct {
	Level string `json:"level"`

	Rootless                 bool `json:"rootless"`
	DedicatedWorkspacePerRun bool `json:"dedicated_workspace_per_run"`

	ReapsDetachedDescendants bool `json:"reaps_detached_descendants"`
}

type NetworkCapability struct {
	EgressModes []string `json:"egress_modes,omitempty"`

	EgressUnenforced bool `json:"egress_unenforced,omitempty"`
	PrivateNetwork   bool `json:"private_network,omitempty"`
}

type Features struct {
	MCP            bool `json:"mcp"`
	ToolCalls      bool `json:"tool_calls"`
	Scripts        bool `json:"scripts"`
	Artifacts      bool `json:"artifacts"`
	EventStreaming bool `json:"event_streaming"`
	GPU            bool `json:"gpu"`
}

type Availability struct {
	ConcurrentRunSlots int  `json:"concurrent_run_slots"`
	Healthy            bool `json:"healthy"`
}

type SecurityCapability struct {
	P02Probe *P02Result `json:"p02_probe,omitempty"`
}
