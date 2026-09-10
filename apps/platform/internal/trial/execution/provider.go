package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
)

type ProviderRunState string

const (
	ProviderStateCreating  ProviderRunState = "creating"
	ProviderStateRunning   ProviderRunState = "running"
	ProviderStateCompleted ProviderRunState = "completed"
	ProviderStateFailed    ProviderRunState = "failed"
	ProviderStateCancelled ProviderRunState = "cancelled"
)

func (s ProviderRunState) Terminal() bool {
	return s == ProviderStateCompleted || s == ProviderStateFailed || s == ProviderStateCancelled
}

const (
	errClassProvision          = "provision"
	errClassExecution          = "execution"
	errClassCleanup            = "cleanup"
	errClassCapabilityMismatch = "capability_mismatch"

	errClassBudgetExhausted = "budget_exhausted"
	errClassTimeout         = "timeout"
	errClassCancelled       = "cancelled"
)

type RuntimeSupport struct {
	Runtime          string   `json:"runtime"`
	Versions         []string `json:"versions"`
	AgentIntegration []string `json:"agent_integration,omitempty"`
}

type ProviderCapability struct {
	Provider     string           `json:"provider"`
	Runtimes     []RuntimeSupport `json:"runtimes"`
	MaxResources ResourceLimits   `json:"max_resources"`

	MaxResourcesUnenforced []string `json:"max_resources_unenforced,omitempty"`
	Isolation              struct {
		Level                    string `json:"level"`
		Rootless                 bool   `json:"rootless"`
		DedicatedWorkspacePerRun bool   `json:"dedicated_workspace_per_run"`

		ReapsDetachedDescendants *bool `json:"reaps_detached_descendants,omitempty"`
	} `json:"isolation"`
	Network struct {
		EgressModes []string `json:"egress_modes"`

		EgressUnenforced bool `json:"egress_unenforced"`
		PrivateNetwork   bool `json:"private_network"`
	} `json:"network"`
	Availability struct {
		ConcurrentRunSlots int `json:"concurrent_run_slots"`

		Healthy *bool `json:"healthy"`
	} `json:"availability"`

	Security *SecurityCapability `json:"security,omitempty"`
}

type SecurityCapability struct {
	P02Probe *P02ProbeReading `json:"p02_probe,omitempty"`
}

type P02ProbeReading struct {
	State string `json:"state"`

	CheckedAt time.Time `json:"checked_at"`

	Detail string `json:"detail,omitempty"`
}

func (c ProviderCapability) P02Breach() (bool, string) {
	if c.Security == nil || c.Security.P02Probe == nil {
		return false, ""
	}
	p := c.Security.P02Probe
	if p.State != "fail" {
		return false, ""
	}
	return true, p.Detail
}

type PackageRef struct {
	SkillVersionID string `json:"skill_version_id"`
	ContentHash    string `json:"content_hash"`
	ObjectKey      string `json:"object_key,omitempty"`
}

type datasetRef struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`

	ObjectKey string `json:"object_key,omitempty"`
}

type TestCaseSnapshotRef struct {
	TestCaseSnapshotID string       `json:"test_case_snapshot_id"`
	ContentHash        string       `json:"content_hash"`
	UserPrompt         string       `json:"user_prompt"`
	DatasetRefs        []datasetRef `json:"dataset_refs,omitempty"`
}

type RuntimeProfile struct {
	Runtime          string `json:"runtime"`
	RuntimeVersion   string `json:"runtime_version"`
	Model            string `json:"model,omitempty"`
	AgentIntegration string `json:"agent_integration,omitempty"`
}

type egressAllow struct {
	Purpose string `json:"purpose"`
	URL     string `json:"url"`
}

type EgressPolicy struct {
	Mode  string        `json:"mode"`
	Allow []egressAllow `json:"allow"`
}

type TracePolicy struct {
	Level string `json:"level"`

	IngestionURL string `json:"ingestion_url,omitempty"`
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

type RunRequest struct {
	RunID            string              `json:"run_id"`
	RunAttemptID     string              `json:"run_attempt_id"`
	Attempt          int                 `json:"attempt"`
	WorkspaceID      string              `json:"workspace_id"`
	IdempotencyKey   string              `json:"idempotency_key,omitempty"`
	SkillVersion     PackageRef          `json:"skill_version"`
	TestCaseSnapshot TestCaseSnapshotRef `json:"test_case_snapshot"`
	Runtime          RuntimeProfile      `json:"runtime"`
	ResourceLimits   ResourceLimits      `json:"resource_limits"`
	Egress           EgressPolicy        `json:"egress"`
	ObjectGrants     []ObjectGrant       `json:"object_grants,omitempty"`
	ModelGateway     *ModelGatewayGrant  `json:"model_gateway,omitempty"`
	Trace            TracePolicy         `json:"trace"`
}

type RunError struct {
	Class     string `json:"class"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type RunUsage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	ModelCostUSD     float64 `json:"model_cost_usd"`
	WallClockSeconds float64 `json:"wall_clock_seconds"`
}

type RunArtifact struct {
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentHash string `json:"content_hash"`
	ObjectKey   string `json:"object_key,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
}

type RunResult struct {
	RunID        string `json:"run_id"`
	RunAttemptID string `json:"run_attempt_id"`
	Status       string `json:"status"`
	AgentOutput  string `json:"agent_output,omitempty"`

	Artifacts          []RunArtifact `json:"artifacts,omitempty"`
	ArtifactsTruncated bool          `json:"artifacts_truncated,omitempty"`
	Usage              *RunUsage     `json:"usage,omitempty"`
	Error              *RunError     `json:"error,omitempty"`
}

type ProviderRun struct {
	RunID             string           `json:"run_id"`
	RunAttemptID      string           `json:"run_attempt_id"`
	Provider          string           `json:"provider"`
	ProviderRunID     string           `json:"provider_run_id"`
	State             ProviderRunState `json:"state"`
	StateReason       string           `json:"state_reason,omitempty"`
	CancelRequestedAt *time.Time       `json:"cancel_requested_at,omitempty"`
	CreatedAt         *time.Time       `json:"created_at,omitempty"`
	StartedAt         *time.Time       `json:"started_at,omitempty"`
	FinishedAt        *time.Time       `json:"finished_at,omitempty"`
	ObservedAt        time.Time        `json:"observed_at"`
	Result            *RunResult       `json:"result,omitempty"`
}

type ProviderRunList struct {
	Provider string        `json:"provider"`
	Runs     []ProviderRun `json:"runs"`

	ObservedAt time.Time `json:"observed_at"`
}

type Provider struct {
	Name    string
	BaseURL string

	token string
	HTTP  *http.Client
}

type providerError struct {
	Status  int
	Class   string
	Message string
}

func (e *providerError) Error() string {
	if e.Class != "" {
		return fmt.Sprintf("provider returned %d (%s): %s", e.Status, e.Class, e.Message)
	}
	return fmt.Sprintf("provider returned %d: %s", e.Status, e.Message)
}

func retryable(err error) bool {
	if pe, ok := errors.AsType[*providerError](err); ok {
		return pe.Status == http.StatusTooManyRequests || pe.Status >= 500
	}

	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func (p *Provider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return http.DefaultClient
}

func (p *Provider) do(ctx context.Context, method, operation, path string, body, out any, want ...int) (int, error) {
	start := time.Now()
	status, err := p.call(ctx, method, path, body, out, want...)
	metrics.ProviderRequest.WithLabelValues(p.Name, operation, metrics.StatusClass(status)).Inc()
	metrics.ObserveSince(metrics.ProviderRequestDuration.WithLabelValues(p.Name, operation), start)
	return status, err
}

func (p *Provider) call(ctx context.Context, method, path string, body, out any, want ...int) (int, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(p.BaseURL, "/")+path, payload)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	resp, err := p.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := readBoundedResponse(resp.Body, 4<<20)
	if err != nil {
		for _, code := range want {
			if resp.StatusCode == code {
				return resp.StatusCode, err
			}
		}
		return resp.StatusCode, &providerError{Status: resp.StatusCode, Message: err.Error()}
	}
	for _, code := range want {
		if resp.StatusCode == code {
			if out != nil && len(raw) > 0 {
				if err := json.Unmarshal(raw, out); err != nil {
					return resp.StatusCode, fmt.Errorf("decode %s %s: %w", method, path, err)
				}
			}
			return resp.StatusCode, nil
		}
	}

	var errBody struct {
		Error   string `json:"error"`
		Class   string `json:"class"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &errBody)
	message := errBody.Error
	if message == "" {
		message = errBody.Message
	}
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}
	return resp.StatusCode, &providerError{Status: resp.StatusCode, Class: errBody.Class, Message: message}
}

func (p *Provider) Capability(ctx context.Context) (ProviderCapability, error) {
	var c ProviderCapability
	_, err := p.do(ctx, http.MethodGet, "capability", "/capability", nil, &c, http.StatusOK)
	return c, err
}

func (p *Provider) CreateRun(ctx context.Context, req RunRequest) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodPost, "create_run", "/runs", req, &pr, http.StatusCreated, http.StatusOK)
	return pr, err
}

func (p *Provider) GetRun(ctx context.Context, providerRunID string) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodGet, "get_run", "/runs/"+url.PathEscape(providerRunID), nil, &pr, http.StatusOK)
	return pr, err
}

func (p *Provider) Cancel(ctx context.Context, providerRunID string) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodPost, "cancel_run", "/runs/"+url.PathEscape(providerRunID)+"/cancel", nil, &pr, http.StatusAccepted)
	return pr, err
}

func (p *Provider) Destroy(ctx context.Context, providerRunID string) error {
	_, err := p.do(ctx, http.MethodDelete, "destroy_run", "/runs/"+url.PathEscape(providerRunID), nil, nil, http.StatusNoContent)
	return err
}

func (p *Provider) ListActive(ctx context.Context) (ProviderRunList, error) {
	var list ProviderRunList
	_, err := p.do(ctx, http.MethodGet, "list_active", "/runs?active=true", nil, &list, http.StatusOK)
	return list, err
}

type Registry struct {
	Providers []*Provider

	TTL time.Duration

	mu     sync.Mutex
	cached map[string]cachedCapability
}

type cachedCapability struct {
	capability ProviderCapability
	at         time.Time
	err        error
}

const capabilityTTL = 30 * time.Second

var (
	ErrNoProvider = errors.New("no sandbox provider is configured")

	ErrNoCompatibleProvider = errors.New("no configured sandbox provider can run this request")
)

func NewRegistryFromEnv() *Registry {
	r := &Registry{}
	for _, entry := range strings.Split(os.Getenv("SKILLHUB_SANDBOX_PROVIDERS"), ",") {
		name, base, ok := strings.Cut(strings.TrimSpace(entry), "=")
		name, base = strings.TrimSpace(name), strings.TrimSpace(base)
		if !ok || name == "" || base == "" {
			continue
		}
		r.Providers = append(r.Providers, &Provider{
			Name:    name,
			BaseURL: base,
			token:   os.Getenv("SKILLHUB_SANDBOX_TOKEN_" + strings.ToUpper(name)),
			HTTP:    &http.Client{Timeout: 30 * time.Second},
		})
	}
	return r
}

func NewRegistry(providers ...*Provider) *Registry { return &Registry{Providers: providers} }

func NewProvider(name, baseURL, token string) *Provider {
	return &Provider{Name: name, BaseURL: baseURL, token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (r *Registry) Lookup(name string) *Provider {
	for _, p := range r.Providers {
		if p.Name == name {
			return p
		}
	}
	return nil
}

func (r *Registry) Capability(ctx context.Context, p *Provider) (ProviderCapability, error) {
	ttl := r.TTL
	if ttl == 0 {
		ttl = capabilityTTL
	}

	if entry, ok := r.freshCapability(p.Name, ttl); ok {
		return entry.capability, entry.err
	}
	capability, err := p.Capability(ctx)

	switch {
	case err != nil:
		metrics.ProviderCapability.WithLabelValues(p.Name, "error").Inc()
	case capability.Availability.Healthy != nil && !*capability.Availability.Healthy:
		metrics.ProviderCapability.WithLabelValues(p.Name, "unhealthy").Inc()
	default:
		metrics.ProviderCapability.WithLabelValues(p.Name, "ok").Inc()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cached == nil {
		r.cached = map[string]cachedCapability{}
	}
	r.cached[p.Name] = cachedCapability{capability: capability, at: time.Now(), err: err}
	return capability, err
}

func (r *Registry) freshCapability(name string, ttl time.Duration) (cachedCapability, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cached[name]
	if !ok || time.Since(entry.at) >= ttl {
		return cachedCapability{}, false
	}
	return entry, true
}

func (r *Registry) Refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached = nil
}
