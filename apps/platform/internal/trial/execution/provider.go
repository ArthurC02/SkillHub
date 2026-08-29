package run

// The platform's half of contracts/openapi/sandbox-provider.yaml (RUN-005). The
// contract is frozen; these types are hand-written against it because no OpenAPI
// codegen is wired yet (see the contracts-drift job in .github/workflows/ci.yml),
// and only the fields the control plane actually reads are declared. Anything a
// provider sends that is not here is ignored, which is what lets the contract grow
// without breaking this client.
//
// Interaction is strictly polling: the platform is the only client, providers never
// call back and never touch the database (iron rule 2).
//
// Anti-corruption layer: the provider's contract types stop here. ProviderRunState,
// ProviderRun, RunResult and the rest are wire shapes, never domain state — the
// translation into gen.RunStatus is job.go's classifyResult/mapState, and the run's
// own status is decided by statemachine.go alone (iron rule 5). A provider vocabulary
// word appearing in the aggregate, or in a database column other than the attempt's
// error_class, means this layer leaked.

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

// ProviderRunState is the provider's own lifecycle, not the platform's run status.
// The mapping into the platform machine lives in job.go (iron rule 5).
type ProviderRunState string

const (
	ProviderStateCreating  ProviderRunState = "creating"
	ProviderStateRunning   ProviderRunState = "running"
	ProviderStateCompleted ProviderRunState = "completed"
	ProviderStateFailed    ProviderRunState = "failed"
	ProviderStateCancelled ProviderRunState = "cancelled"
)

// Terminal reports whether the provider is done with this attempt. The contract
// guarantees a result on exactly these three.
func (s ProviderRunState) Terminal() bool {
	return s == ProviderStateCompleted || s == ProviderStateFailed || s == ProviderStateCancelled
}

// RunError.class values the platform branches on. The rest of the enum is stored
// verbatim on the attempt without being interpreted.
const (
	errClassProvision          = "provision"
	errClassExecution          = "execution"
	errClassCleanup            = "cleanup"
	errClassCapabilityMismatch = "capability_mismatch"
	// The platform's own ceiling, not the gateway's 429. The contract's enum
	// glosses this as "gateway 429, distinct from a model 403", which was the only
	// way it could be reached when that sentence was written -- the ceiling was
	// enforced inside the sandbox and the control plane never decided anything
	// about tokens. Since 2026-08-25 the worker enforces it (04 丙-69), so the
	// platform is now the second thing that can produce this class, and PDM-005
	// §5.2's risk row asks for exactly that: 「Trace 中標為 budget_exhausted 而非
	// 泛用失敗」. It was reported as `execution` until this line existed, which is
	// the generic failure that row forbids.
	errClassBudgetExhausted = "budget_exhausted"
	errClassTimeout         = "timeout"
	errClassCancelled       = "cancelled"
)

// --- wire types --------------------------------------------------------------

type RuntimeSupport struct {
	Runtime          string   `json:"runtime"`
	Versions         []string `json:"versions"`
	AgentIntegration []string `json:"agent_integration,omitempty"`
}

// ProviderCapability is GET /capability. A zero numeric in MaxResources means "not
// declared" rather than "zero allowed": a provider that answers 0 vCPU is not
// claiming it cannot run anything, it simply left the field out.
type ProviderCapability struct {
	Provider     string           `json:"provider"`
	Runtimes     []RuntimeSupport `json:"runtimes"`
	MaxResources ResourceLimits   `json:"max_resources"`
	// MaxResourcesUnenforced names the ceilings the provider carries a number for
	// in MaxResources but does not have the operating system hold to. Empty means
	// every declared ceiling is enforced, which is what a production node must be
	// able to say.
	//
	// It is decoded here because a declaration nobody reads is the same defect it
	// was added to fix: ADR-059 decision 3's incident was a node whose declaration
	// and reality diverged with only a startup log line knowing. Match() is the
	// consumer.
	MaxResourcesUnenforced []string `json:"max_resources_unenforced,omitempty"`
	Isolation              struct {
		Level                    string `json:"level"`
		Rootless                 bool   `json:"rootless"`
		DedicatedWorkspacePerRun bool   `json:"dedicated_workspace_per_run"`
		// ReapsDetachedDescendants: whether ending a run also ends a descendant
		// that deliberately left its process group or job object. Pointer for the
		// same reason as Availability.Healthy - an omitted field is "did not say",
		// which is not the same claim as false.
		//
		// It does not gate dispatch: the two platforms of one driver legitimately
		// differ on it, and refusing on it would take the clean test mode out on a
		// fact that is disclosure-shaped rather than refusal-shaped. It reaches
		// the user through ProviderSummary instead (02:PORT-003, TEST-008).
		ReapsDetachedDescendants *bool `json:"reaps_detached_descendants,omitempty"`
	} `json:"isolation"`
	Network struct {
		EgressModes    []string `json:"egress_modes"`
		PrivateNetwork bool     `json:"private_network"`
	} `json:"network"`
	Availability struct {
		ConcurrentRunSlots int `json:"concurrent_run_slots"`
		// Pointer: an omitted `healthy` is "did not say", not "unhealthy". Only an
		// explicit false takes a provider out of rotation.
		Healthy *bool `json:"healthy"`
	} `json:"availability"`
	// Security is the one block here that carries a measurement rather than a
	// configuration claim, and it is on this response because the sandbox
	// provider contract is one way: every operation in sandbox-provider.yaml is
	// the control plane calling the node, and the node has no channel to push
	// an alert back. Polling this is the path that already exists.
	Security *SecurityCapability `json:"security,omitempty"`
}

// SecurityCapability mirrors ProviderCapability.security in
// contracts/openapi/sandbox-provider.yaml. Named rather than anonymous because
// a test has to be able to build one: this side and apps/sandbox are separate
// modules that agree only through that file, so the assertion that they still
// do is a test constructing this shape by hand.
type SecurityCapability struct {
	P02Probe *P02ProbeReading `json:"p02_probe,omitempty"`
}

// P02ProbeReading is a node's own last answer to threat model P-02 (ADR-022 T10).
type P02ProbeReading struct {
	// State is `pass`, `fail`, `unknown` or `not_configured`. `fail` means a
	// connection SUCCEEDED from a sandbox's own network position to an address a
	// sandbox must never reach - the architecture regression signal of
	// 02:SEC-010's P1 row.
	State string `json:"state"`
	// CheckedAt is when the reading was taken, not when it was reported. A
	// resident probe that stopped keeps answering `pass` with a timestamp that
	// stops moving, and that is the only way to see it from here.
	CheckedAt time.Time `json:"checked_at"`
	// Detail on `fail` is which destination answered: an address and a port,
	// never a payload or a credential (iron rule 11).
	Detail string `json:"detail,omitempty"`
}

// P02Breach reports whether this node says a sandbox on it reached something it
// must not. Only an explicit `fail` counts.
//
// `unknown` deliberately does not halt the fleet. ADR-022 §3 counts unknown as
// fail for acceptance and that is the right rule for a signature on a release;
// it is the wrong rule for a switch that stops every run in the pool, because a
// node that has merely just booted has not taken a reading yet. A node in that
// state removes itself from rotation with `healthy: false` instead, which the
// scheduler already reads.
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

// PackageRef, TestCaseSnapshotRef and friends carry only references and hashes.
// Dataset bytes and the package archive travel through object grants, never here.
type PackageRef struct {
	SkillVersionID string `json:"skill_version_id"`
	ContentHash    string `json:"content_hash"`
	ObjectKey      string `json:"object_key,omitempty"`
}

type datasetRef struct {
	DatasetID   string `json:"dataset_id"`
	FileName    string `json:"file_name"`
	ContentHash string `json:"content_hash"`
	// ObjectKey pairs this reference with the ObjectGrant that can fetch it.
	// Without it the execution node would have to guess which grant belongs to
	// which file (SBX-008).
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
	// Empty until TRACE-002 wires ingestion. A provider that gets no URL emits no
	// events, which is honest: nothing is collecting them yet.
	IngestionURL string `json:"ingestion_url,omitempty"`
}

// ObjectGrant is one short-lived, single-path authorization (SBX-008). URL is
// secret material: it *is* the authorization, so it is never logged or traced
// (iron rule 11) and it never appears in a permission summary the user reads.
type ObjectGrant struct {
	Purpose   string    `json:"purpose"` // skill_package | dataset | artifact_upload
	ObjectKey string    `json:"object_key"`
	Access    string    `json:"access"` // read | write
	URL       string    `json:"url,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ModelGatewayGrant is the attempt's LiteLLM Virtual Key (ADR-017). VirtualKey is
// secret material and travels no further than the execution node and the sandbox
// it creates.
type ModelGatewayGrant struct {
	BaseURL      string    `json:"base_url"`
	VirtualKey   string    `json:"virtual_key,omitempty"`
	MaxBudgetUSD float64   `json:"max_budget_usd,omitempty"`
	TPMLimit     int       `json:"tpm_limit,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// RunRequest is one execution attempt.
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

// RunArtifact is one row of the manifest an attempt reported: the name, size and
// hash of a file the workload wrote. The bytes are not here - they are inside the
// single archive the attempt's write grant authorized (see grantsFor) - and the
// control plane never opens it (iron rule 1).
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
	Status       string `json:"status"` // succeeded | failed | cancelled | timed_out
	AgentOutput  string `json:"agent_output,omitempty"`
	// Artifacts is the manifest, decoded because EVAL-001 needs it: without a
	// stored manifest, "the run reported success and produced no files" - handoff
	// 丙-5's concrete case - cannot be told apart from "nobody wrote the manifest
	// down", and an evaluator cannot honestly report either.
	Artifacts []RunArtifact `json:"artifacts,omitempty"`
	Usage     *RunUsage     `json:"usage,omitempty"`
	Error     *RunError     `json:"error,omitempty"`
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
	// The provider's clock when the snapshot was taken. An orphan scan judges
	// against this instant, not against its own now(), so a sandbox created while
	// the list was in flight is never called a leak.
	ObservedAt time.Time `json:"observed_at"`
}

// --- client ------------------------------------------------------------------

// Provider is one configured sandbox provider. Deployment-static (ADR-018 E1):
// there is no dynamic registration, because a provider that can register itself
// is a provider an attacker can register.
type Provider struct {
	Name    string
	BaseURL string
	// Long-lived infrastructure credential, never a per-run one, and never
	// logged (iron rule 11). Unexported so it cannot be marshalled by accident.
	token string
	HTTP  *http.Client
}

// providerError is a non-2xx answer, classified enough for the retry policy to
// decide without parsing message strings.
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

// retryable reports whether re-sending could plausibly succeed. Transport failures
// and 429/5xx are the provider having a bad moment; 4xx below 429 is the request
// itself being wrong, and repeating it fails identically forever (ADR-004: only
// known-idempotent actions retry, and never without bound).
func retryable(err error) bool {
	if pe, ok := errors.AsType[*providerError](err); ok {
		return pe.Status == http.StatusTooManyRequests || pe.Status >= 500
	}
	// Context cancellation is the platform stopping, not the provider failing.
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func (p *Provider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return http.DefaultClient
}

// do performs one contract call. want lists the status codes that are a success
// for this operation; anything else becomes a providerError.
//
// operation is the contract operation name, used only as a metric label: it is a
// fixed string from the call sites below, never anything derived from a URL, so
// provider metrics cannot grow a series per run (O11Y-002).
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

	// Bounded read: a provider is not trusted to send a sane body (iron rule 1
	// applies to what it says as much as to what it runs).
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return resp.StatusCode, err
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
	// Both error shapes of the contract: {error} and RunError{class,message}.
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

// Capability reads GET /capability.
func (p *Provider) Capability(ctx context.Context) (ProviderCapability, error) {
	var c ProviderCapability
	_, err := p.do(ctx, http.MethodGet, "capability", "/capability", nil, &c, http.StatusOK)
	return c, err
}

// CreateRun dispatches one attempt. 201 and 200 are both success: 200 means this
// (run_id, attempt) was already dispatched and the existing sandbox came back, which
// is exactly what makes a re-send safe when the worker cannot tell whether the first
// call landed.
func (p *Provider) CreateRun(ctx context.Context, req RunRequest) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodPost, "create_run", "/runs", req, &pr, http.StatusCreated, http.StatusOK)
	return pr, err
}

// GetRun is the polling read.
func (p *Provider) GetRun(ctx context.Context, providerRunID string) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodGet, "get_run", "/runs/"+url.PathEscape(providerRunID), nil, &pr, http.StatusOK)
	return pr, err
}

// Cancel asks the provider to stop an attempt. 202 covers "already terminal" too,
// so a cancel that races the run finishing is not an error to special-case.
func (p *Provider) Cancel(ctx context.Context, providerRunID string) (ProviderRun, error) {
	var pr ProviderRun
	_, err := p.do(ctx, http.MethodPost, "cancel_run", "/runs/"+url.PathEscape(providerRunID)+"/cancel", nil, &pr, http.StatusAccepted)
	return pr, err
}

// Destroy releases everything the attempt holds. Idempotent and 404-free by
// contract, so a worker that crashed mid-cleanup can simply call it again
// (iron rule 9).
func (p *Provider) Destroy(ctx context.Context, providerRunID string) error {
	_, err := p.do(ctx, http.MethodDelete, "destroy_run", "/runs/"+url.PathEscape(providerRunID), nil, nil, http.StatusNoContent)
	return err
}

// ListActive reads the runs the provider still holds resources for (RUN-007).
func (p *Provider) ListActive(ctx context.Context) (ProviderRunList, error) {
	var list ProviderRunList
	_, err := p.do(ctx, http.MethodGet, "list_active", "/runs?active=true", nil, &list, http.StatusOK)
	return list, err
}

// --- registry ----------------------------------------------------------------

// Registry is the configured set of providers plus a short-lived cache of what
// each one says it can do. ProviderCapability.availability is a current reading,
// not a manifest, so it is cached for seconds and not for the process lifetime.
type Registry struct {
	Providers []*Provider
	// TTL bounds how stale a capability answer may be when a run is scheduled
	// against it. Zero means capabilityTTL.
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
	// ErrNoProvider is an empty registry: a deployment with no sandbox at all.
	// Distinct from ErrNoCompatibleProvider because it is an operator problem, not
	// something the user asked for wrongly.
	ErrNoProvider = errors.New("no sandbox provider is configured")
	// ErrNoCompatibleProvider is RUN-005's refusal. Its message names what did not
	// match, per provider, because ADR-004 requires a reason a user can read.
	ErrNoCompatibleProvider = errors.New("no configured sandbox provider can run this request")
)

// NewRegistryFromEnv reads the deployment's static provider configuration:
//
//	SKILLHUB_SANDBOX_PROVIDERS=self_hosted=http://sandbox:9000,dev=http://localhost:9001
//	SKILLHUB_SANDBOX_TOKEN_SELF_HOSTED=<bearer token>
//
// One token per provider, keyed by the upper-cased name, so rotating one provider's
// credential does not touch another's. An empty list yields an empty registry rather
// than an error: cmd/api and cmd/worker both start fine without a sandbox, and the
// run that needs one fails saying so.
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

// NewRegistry builds a registry over already-constructed providers. Used by tests
// and by anything that configures a provider from something other than the
// environment.
func NewRegistry(providers ...*Provider) *Registry { return &Registry{Providers: providers} }

// NewProvider builds one provider client. token is the deployment's bearer
// credential and is kept unexported from here on.
func NewProvider(name, baseURL, token string) *Provider {
	return &Provider{Name: name, BaseURL: baseURL, token: token, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// Lookup returns the configured provider of that name, or nil. Cleanup uses it to
// reach the provider an old attempt named, which may since have been removed from
// the configuration.
func (r *Registry) Lookup(name string) *Provider {
	for _, p := range r.Providers {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// Capability returns the provider's declared capability, cached for TTL. Failures
// are cached too, for the same short window: a provider that is down should not be
// re-probed once per scheduling decision.
func (r *Registry) Capability(ctx context.Context, p *Provider) (ProviderCapability, error) {
	ttl := r.TTL
	if ttl == 0 {
		ttl = capabilityTTL
	}
	// The lock covers the map, not the network. Holding it across p.Capability -
	// a 30 second client timeout - serialised every scheduling decision and every
	// preflight screen in the process behind one unreachable node, so a single
	// black-holed provider stopped the API's pre-run summary for everybody.
	//
	// ponytail: double-checked, not singleflight. Two callers racing the same cold
	// provider both fetch; the ceiling is one duplicate probe per provider per TTL
	// window, which is cheaper than the dependency and far cheaper than the lock.
	if entry, ok := r.freshCapability(p.Name, ttl); ok {
		return entry.capability, entry.err
	}
	capability, err := p.Capability(ctx)
	// O11Y-002: capability fetch health. Counted on the miss path only, so the
	// number is "how often we actually asked", not "how often we used the answer".
	// `unhealthy` is the provider answering and declaring itself out of rotation,
	// which is a different operational fact from being unreachable.
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

// freshCapability is the read half of the double check: it takes the lock only
// for the map lookup. ok reports whether the entry was there and still inside the
// TTL - failures are cached on the same terms, which is what stops a down
// provider from being re-probed once per scheduling decision.
func (r *Registry) freshCapability(name string, ttl time.Duration) (cachedCapability, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, ok := r.cached[name]
	if !ok || time.Since(entry.at) >= ttl {
		return cachedCapability{}, false
	}
	return entry, true
}

// Refresh drops the cache, so the next scheduling decision re-reads every
// provider. Called once at worker start (RUN-005 reads capability at startup); the TTL covers
// the periodic half.
func (r *Registry) Refresh() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cached = nil
}
