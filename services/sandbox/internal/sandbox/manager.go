package sandbox

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Driver is the container work a Manager needs. Everything above it — the
// (run_id, attempt) idempotency key, the state machine, the wall clock, cancel
// and destroy semantics — is the contract's business and lives in Manager, so a
// fake Driver in a test exercises the same rules the real one runs under.
type Driver interface {
	// Start creates and starts one sandbox for req under the given handle. It
	// returns before the workload finishes.
	Start(ctx context.Context, providerRunID string, req RunRequest) error
	// Wait blocks until the workload exits. A non-nil error means the provider
	// could not carry the attempt through, not that the workload failed.
	Wait(ctx context.Context, providerRunID string) (Outcome, error)
	// Stop asks the workload to stop, then kills it after grace. Used both for
	// cancel and for the soft wall clock.
	Stop(ctx context.Context, providerRunID string, grace time.Duration) error
	// Remove releases everything the handle holds. Idempotent: removing what is
	// not there is success (SBX-009, iron rule 9).
	Remove(ctx context.Context, providerRunID string) error
	// ReadTrace returns the raw JSONL the workload has written to
	// <out>/trace/events.jsonl so far (TRACE-002). An absent file is not an
	// error: it means the workload has not emitted anything yet. The bytes are
	// untrusted workload output and are never parsed by the driver.
	ReadTrace(ctx context.Context, providerRunID string) ([]byte, error)
	// ReadArtifacts returns the workload's collected output as a tar stream, or
	// nothing when it produced none (SBX-008). Like ReadTrace the bytes are
	// untrusted and the driver does not open them; the manager does, to enforce
	// the size ceilings and build the manifest.
	ReadArtifacts(ctx context.Context, providerRunID string) ([]byte, error)
	// WorkloadDone reports whether the workload has finished and is waiting to
	// be released, and ReleaseWorkload lets it go. The two exist because the
	// sandbox's scratch space is a tmpfs: nothing can be read out of it once the
	// workload's process has exited, so collection has to happen in the window
	// between the two (PDM-005's cooperative stop window).
	WorkloadDone(ctx context.Context, providerRunID string) (bool, error)
	ReleaseWorkload(ctx context.Context, providerRunID string) error
	// Adopt reports the sandboxes this driver still holds, for rebuilding state
	// after a restart.
	Adopt(ctx context.Context) ([]Adopted, error)
	// Healthy reports whether the driver can reach its backend.
	Healthy(ctx context.Context) bool
}

// Outcome is how a workload ended, on its own terms.
type Outcome struct {
	ExitCode  int
	OOMKilled bool
	Output    string // tail of the workload's output, already size-bounded
}

// Adopted is what a restarted service can learn about a sandbox from the
// container itself. It is deliberately thin: only non-secret labels survive a
// restart, so a re-sent dispatch is matched by request hash and never by
// replaying the original body.
type Adopted struct {
	ProviderRunID string
	RunID         string
	RunAttemptID  string
	Attempt       int
	RequestHash   string
	CreatedAt     time.Time
	Running       bool
	HardDeadline  time.Time
}

// Config is provider-level policy: what this deployment can run and how hard it
// isolates. Per-run numbers come from the RunRequest, not from here.
type Config struct {
	Provider       string
	Runtimes       []RuntimeCapability
	MaxResources   ResourceLimits
	IsolationLevel string // "container" for the dev DockerProvider, "gvisor" in production
	EgressModes    []string
	Slots          int
	CancelGrace    time.Duration
}

var (
	// ErrConflict is the 409 case: same (run_id, attempt), different content.
	ErrConflict = errors.New("run already dispatched with different content")
	// ErrNoSlot is the 429 case.
	ErrNoSlot = errors.New("no free run slot")
	// ErrNotFound is the 404 case for read and cancel. DELETE never returns it.
	ErrNotFound = errors.New("no run with this handle")
)

type entry struct {
	run  ProviderRun
	hash string
	stop context.CancelFunc
	// secrets are the values this provider injected into the sandbox. Kept only
	// to mask them out of workload output before it leaves the process (iron
	// rule 11); never logged, never serialized.
	secrets   []string
	timedOut  bool
	cancelled bool
	// traceURL is this attempt's ingestion destination, credential included
	// (TRACE-002). Empty means nothing is collecting and no draining happens.
	traceURL string
	// traceSent is how many whole events of the file have been accepted. It is
	// a low-water mark only: a batch that failed leaves it where it was, so the
	// events go again and the platform dedupes them by event_id.
	traceSent int
	// artifactGrant is the one write authorization this attempt was dispatched
	// with (SBX-008). Nil means nothing was authorized and nothing is collected.
	artifactGrant *ObjectGrant
	// limits are the run's own ceilings, kept for the artifact collection that
	// happens after the workload is gone and the RunRequest is out of scope.
	limits ResourceLimits
	// artifacts is the manifest collected before the terminal result is written.
	artifacts []Artifact
}

// Manager owns the run bookkeeping for one provider process.
//
// ponytail: state lives in memory and is rebuilt from container labels on
// restart (Adopt), so a crash between "container started" and "entry recorded"
// leaks one sandbox until the platform's orphan scan (RUN-007) reaches it via
// GET /runs. The upgrade path is a small local durable store on the execution
// node — not the core database, which this plane must never touch (iron rule 2).
type Manager struct {
	drv Driver
	cfg Config
	now func() time.Time
	log *slog.Logger
	// sink pushes collected trace batches to the control plane. Nil disables
	// collection entirely, which is what a manager test wants and what a
	// deployment with no ingestion URL gets anyway.
	sink    TraceSink
	metrics *Metrics

	mu    sync.Mutex
	runs  map[string]*entry // provider_run_id -> entry
	byKey map[string]string // run_id|attempt -> provider_run_id
	wg    sync.WaitGroup
}

func NewManager(drv Driver, cfg Config, log *slog.Logger) *Manager {
	if cfg.CancelGrace <= 0 {
		cfg.CancelGrace = 10 * time.Second
	}
	return &Manager{
		drv:   drv,
		cfg:   cfg,
		now:   func() time.Time { return time.Now().UTC() },
		log:   log,
		runs:  map[string]*entry{},
		byKey: map[string]string{},
	}
}

// WithTrace turns on trace collection (TRACE-002) and observation. Both are
// optional and set after construction, so the contract behaviour a Manager test
// exercises is identical with and without them.
func (m *Manager) WithTrace(sink TraceSink, mx *Metrics) *Manager {
	m.sink, m.metrics = sink, mx
	return m
}

// Capability answers GET /capability.
func (m *Manager) Capability(ctx context.Context) ProviderCapability {
	m.mu.Lock()
	live := len(m.runs)
	m.mu.Unlock()
	free := m.cfg.Slots - live
	if free < 0 {
		free = 0
	}
	return ProviderCapability{
		Provider:     m.cfg.Provider,
		Runtimes:     m.cfg.Runtimes,
		MaxResources: m.cfg.MaxResources,
		Isolation: Isolation{
			Level:                    m.cfg.IsolationLevel,
			Rootless:                 true,
			DedicatedWorkspacePerRun: true,
		},
		Network: &NetworkCapability{EgressModes: m.cfg.EgressModes, PrivateNetwork: true},
		Features: &Features{
			ToolCalls: true,
			Scripts:   true,
			Artifacts: true,
			// TRACE-002: true only when a sink is actually wired. The claim is
			// "this provider will push trace events if you give it an ingestion
			// URL", and a build with no sink cannot keep it.
			EventStreaming: m.sink != nil,
		},
		Availability: &Availability{ConcurrentRunSlots: free, Healthy: m.drv.Healthy(ctx)},
	}
}

// Create dispatches one attempt. created reports whether a sandbox was made:
// false means this (run_id, attempt) was already dispatched and the existing run
// is returned unchanged (200 rather than 201).
func (m *Manager) Create(ctx context.Context, req RunRequest) (run ProviderRun, created bool, err error) {
	if err := validate(req); err != nil {
		return ProviderRun{}, false, err
	}
	if re := m.cfg.accept(req); re != nil {
		return ProviderRun{}, false, re
	}
	hash := HashRequest(req)
	key := req.RunID + "|" + fmt.Sprint(req.Attempt)

	m.mu.Lock()
	if id, ok := m.byKey[key]; ok {
		e := m.runs[id]
		m.mu.Unlock()
		if e == nil {
			return ProviderRun{}, false, ErrNotFound
		}
		if e.hash != hash {
			return ProviderRun{}, false, ErrConflict
		}
		return m.snapshot(id)
	}
	if len(m.runs) >= m.cfg.Slots {
		m.mu.Unlock()
		return ProviderRun{}, false, ErrNoSlot
	}
	id := newHandle()
	now := m.now()
	e := &entry{
		hash:          hash,
		secrets:       secretsOf(req),
		traceURL:      req.Trace.IngestionURL,
		artifactGrant: artifactGrantOf(req),
		limits:        req.ResourceLimits,
		run: ProviderRun{
			RunID:         req.RunID,
			RunAttemptID:  req.RunAttemptID,
			Provider:      m.cfg.Provider,
			ProviderRunID: id,
			State:         StateCreating,
			CreatedAt:     now,
		},
	}
	m.runs[id] = e
	m.byKey[key] = id
	live := len(m.runs)
	m.mu.Unlock()

	// Log identifiers only: a RunRequest carries a Virtual Key and pre-signed
	// URLs, and none of it belongs in a log line (iron rule 11).
	m.log.Info("run dispatched", "run_id", req.RunID, "attempt", req.Attempt, "provider_run_id", id)
	m.metrics.dispatched()
	m.metrics.active(live)

	if err := m.drv.Start(ctx, id, req); err != nil {
		m.finish(id, Outcome{}, &RunError{
			Class:     ClassProvision,
			Message:   "sandbox could not be created",
			Retryable: true,
		})
		m.log.Error("sandbox start failed", "provider_run_id", id, "err", err)
		run, _, _ := m.snapshot(id)
		return run, true, nil
	}

	m.mu.Lock()
	e.run.State = StateRunning
	e.run.StartedAt = m.now()
	m.mu.Unlock()

	soft := time.Duration(req.ResourceLimits.WallClockSoftSeconds) * time.Second
	hard := time.Duration(req.ResourceLimits.WallClockHardSeconds) * time.Second
	m.watch(id, soft, hard)
	return m.mustSnapshot(id), true, nil
}

// watch follows one sandbox to its end and enforces the wall clock (SBX-006,
// C-15). The soft limit sends a stop with the soft-to-hard gap as its grace
// window — the gap PDM-005 describes as the chance for a cooperative stop to
// collect artifacts — and the driver kills whatever is left when it expires.
func (m *Manager) watch(id string, soft, hard time.Duration) {
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	if e := m.runs[id]; e != nil {
		e.stop = cancel
	}
	m.mu.Unlock()

	timer := time.AfterFunc(soft, func() {
		m.mu.Lock()
		e := m.runs[id]
		if e == nil || e.run.State.Terminal() {
			m.mu.Unlock()
			return
		}
		e.timedOut = true
		m.mu.Unlock()
		m.log.Warn("wall clock reached, stopping sandbox", "provider_run_id", id)
		if err := m.drv.Stop(context.Background(), id, hard-soft); err != nil {
			m.log.Error("timeout stop failed", "provider_run_id", id, "err", err)
		}
	})

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()
		// TRACE-002: drain while the workload is alive, and once more after it
		// exits. The container is still there at that point - DELETE is what
		// removes it - so the final pass is the last chance to read /out.
		stopTrace := m.startTraceCollector(id)
		out, err := m.drv.Wait(ctx, id)
		stopTrace()
		timer.Stop()

		var re *RunError
		if err != nil && ctx.Err() == nil {
			re = &RunError{Class: ClassExecution, Message: "sandbox could not be followed to its end", Retryable: true}
			m.log.Error("wait failed", "provider_run_id", id, "err", err)
		}
		m.finish(id, out, re)
	}()
}

// finish writes the terminal state and its result. Intent beats outcome: a
// cancelled or timed-out attempt reports that, whatever exit code the workload
// managed to produce on its way down.
func (m *Manager) finish(id string, out Outcome, re *RunError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.runs[id]
	if e == nil || e.run.State.Terminal() {
		return
	}
	now := m.now()
	e.run.FinishedAt = now
	res := &RunResult{
		RunID:         e.run.RunID,
		RunAttemptID:  e.run.RunAttemptID,
		ProviderRunID: id,
		StartedAt:     e.run.StartedAt,
		FinishedAt:    now,
		AgentOutput:   mask(out.Output, e.secrets),
		// Manifest only: the bytes went to object storage under the write grant
		// (SBX-008). Empty when the workload produced nothing, when no grant was
		// issued, or when collection failed - all three are "no artifacts here",
		// and none of them changes the run's own outcome.
		Artifacts: e.artifacts,
	}
	switch {
	case e.cancelled:
		e.run.State, res.Status = StateCancelled, ResultCancelled
		res.Error = &RunError{Class: ClassCancelled, Message: "stopped on request"}
	case e.timedOut:
		// A wall-clock stop is the provider enforcing a limit against the
		// attempt, so the state is failed and the result says timed_out.
		e.run.State, res.Status = StateFailed, ResultTimedOut
		res.Error = &RunError{Class: ClassTimeout, Message: "wall clock limit reached", Retryable: true}
		e.run.StateReason = "wall clock limit reached"
	case re != nil:
		e.run.State, res.Status = StateFailed, ResultFailed
		res.Error = re
		e.run.StateReason = re.Message
	case out.OOMKilled:
		e.run.State, res.Status = StateFailed, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: "memory limit enforced against the workload"}
		e.run.StateReason = res.Error.Message
	case out.ExitCode == 0:
		// The workload ran to its own end and said it succeeded.
		e.run.State, res.Status = StateCompleted, ResultSucceeded
	default:
		// Ran to its own end and reported failure: completed, not failed.
		e.run.State, res.Status = StateCompleted, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: fmt.Sprintf("workload exited with code %d", out.ExitCode)}
	}
	if !e.run.StartedAt.IsZero() {
		res.Usage = &RunUsage{WallClockSeconds: now.Sub(e.run.StartedAt).Seconds()}
	}
	e.run.Result = res
	m.metrics.finished(res.Status)
	m.log.Info("run finished", "provider_run_id", id, "state", e.run.State, "status", res.Status)
}

// Get answers GET /runs/{provider_run_id}.
func (m *Manager) Get(id string) (ProviderRun, error) {
	run, _, err := m.snapshot(id)
	return run, err
}

// List answers GET /runs?active=true: everything this provider still holds
// resources for, destroyed runs excluded. `result` is dropped from the entries
// as the contract requires.
func (m *Manager) List() ProviderRunList {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	list := ProviderRunList{Provider: m.cfg.Provider, Runs: []ProviderRun{}, ObservedAt: now}
	for _, e := range m.runs {
		r := e.run
		r.Result = nil
		r.ObservedAt = now
		list.Runs = append(list.Runs, r)
	}
	return list
}

// Cancel answers POST /runs/{provider_run_id}/cancel. It records intent and
// returns immediately; an already-terminal attempt is not an error, because the
// run can finish between the caller's poll and its cancel.
func (m *Manager) Cancel(ctx context.Context, id string) (ProviderRun, error) {
	m.mu.Lock()
	e := m.runs[id]
	if e == nil {
		m.mu.Unlock()
		return ProviderRun{}, ErrNotFound
	}
	terminal := e.run.State.Terminal()
	if !terminal {
		e.cancelled = true
		if e.run.CancelRequestedAt.IsZero() {
			e.run.CancelRequestedAt = m.now()
		}
	}
	m.mu.Unlock()

	if !terminal {
		m.log.Info("cancel requested", "provider_run_id", id)
		if err := m.drv.Stop(ctx, id, m.cfg.CancelGrace); err != nil {
			m.log.Error("cancel stop failed", "provider_run_id", id, "err", err)
		}
	}
	return m.mustSnapshot(id), nil
}

// Destroy answers DELETE /runs/{provider_run_id}. Idempotent and without a 404:
// a handle nothing is held for is already in the state the caller asked for.
func (m *Manager) Destroy(ctx context.Context, id string) error {
	m.mu.Lock()
	e := m.runs[id]
	if e != nil {
		if e.stop != nil {
			e.stop()
		}
		delete(m.runs, id)
		for k, v := range m.byKey {
			if v == id {
				delete(m.byKey, k)
			}
		}
	}
	live := len(m.runs)
	m.mu.Unlock()
	m.metrics.active(live)

	if err := m.drv.Remove(ctx, id); err != nil {
		// Resources are still held; the platform records the cleanup failure
		// separately and retries, and repeating this call is safe.
		m.log.Error("destroy failed", "provider_run_id", id, "err", err)
		return err
	}
	m.log.Info("run destroyed", "provider_run_id", id)
	return nil
}

// Adopt rebuilds run state from the sandboxes the driver still holds. Without
// it a restarted provider would answer 404 for live attempts and report an
// empty GET /runs, which the platform's orphan scan reads as "nothing leaked".
func (m *Manager) Adopt(ctx context.Context) error {
	found, err := m.drv.Adopt(ctx)
	if err != nil {
		return err
	}
	for _, a := range found {
		e := &entry{
			hash: a.RequestHash,
			run: ProviderRun{
				RunID: a.RunID, RunAttemptID: a.RunAttemptID,
				Provider: m.cfg.Provider, ProviderRunID: a.ProviderRunID,
				State: StateRunning, CreatedAt: a.CreatedAt, StartedAt: a.CreatedAt,
				StateReason: "adopted after provider restart",
			},
		}
		m.mu.Lock()
		m.runs[a.ProviderRunID] = e
		m.byKey[a.RunID+"|"+fmt.Sprint(a.Attempt)] = a.ProviderRunID
		m.mu.Unlock()

		if !a.Running {
			// Exit code is gone with the process we never watched; report the
			// attempt as one the provider could not carry through rather than
			// inventing an outcome for it.
			m.finish(a.ProviderRunID, Outcome{}, &RunError{
				Class:     ClassExecution,
				Message:   "provider restarted while the attempt was in flight",
				Retryable: true,
			})
			continue
		}
		remaining := time.Until(a.HardDeadline)
		if remaining < 0 {
			remaining = 0
		}
		m.watch(a.ProviderRunID, remaining, remaining)
	}
	if len(found) > 0 {
		m.log.Info("adopted sandboxes after restart", "count", len(found))
	}
	return nil
}

// Wait blocks until every watcher has finished, for an orderly shutdown.
func (m *Manager) Wait() { m.wg.Wait() }

func (m *Manager) snapshot(id string) (ProviderRun, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.runs[id]
	if e == nil {
		return ProviderRun{}, false, ErrNotFound
	}
	r := e.run
	r.ObservedAt = m.now()
	return r, false, nil
}

func (m *Manager) mustSnapshot(id string) ProviderRun {
	r, _, _ := m.snapshot(id)
	return r
}

// validate rejects a body that cannot describe a run at all (400). Capability
// refusals are accept()'s job and answer 422.
func validate(req RunRequest) error {
	switch {
	case req.RunID == "":
		return errors.New("run_id is required")
	case req.RunAttemptID == "":
		return errors.New("run_attempt_id is required")
	case req.Attempt < 1:
		return errors.New("attempt must be 1 or greater")
	case req.WorkspaceID == "":
		return errors.New("workspace_id is required")
	case req.TestCase.UserPrompt == "":
		return errors.New("test_case_snapshot.user_prompt is required")
	case req.Runtime.Runtime == "":
		return errors.New("runtime.runtime is required")
	case req.Egress.Mode == "":
		return errors.New("egress.mode is required")
	case req.Trace.Level == "":
		return errors.New("trace.level is required")
	}
	return nil
}

// accept is the capability check: a limit this provider cannot enforce, or a
// runtime it does not have, is refused before anything is created (422), as a
// classified RunError rather than a message the platform has to parse.
func (c Config) accept(req RunRequest) *RunError {
	mismatch := func(format string, args ...any) *RunError {
		return &RunError{Class: ClassCapabilityMismatch, Message: fmt.Sprintf(format, args...), Retryable: false}
	}
	ok := false
	for _, rt := range c.Runtimes {
		if rt.Runtime != req.Runtime.Runtime {
			continue
		}
		for _, v := range rt.Versions {
			if v == req.Runtime.RuntimeVersion {
				ok = true
			}
		}
		if !ok {
			return mismatch("runtime %s version %s is not available here", req.Runtime.Runtime, req.Runtime.RuntimeVersion)
		}
		if req.Runtime.AgentIntegration != "" {
			found := false
			for _, ai := range rt.AgentIntegration {
				if ai == req.Runtime.AgentIntegration {
					found = true
				}
			}
			if !found {
				return mismatch("agent integration %s is not supported here", req.Runtime.AgentIntegration)
			}
		}
	}
	if !ok {
		return mismatch("runtime %s is not available here", req.Runtime.Runtime)
	}

	l, max := req.ResourceLimits, c.MaxResources
	switch {
	case l.VCPU <= 0 || l.MemoryBytes <= 0 || l.DiskBytes <= 0 || l.MaxPIDs <= 0 || l.MaxOpenFiles <= 0:
		return mismatch("resource_limits must set every ceiling: this provider will not run unbounded")
	case l.WallClockHardSeconds <= l.WallClockSoftSeconds:
		return mismatch("wall_clock_hard_seconds must be greater than wall_clock_soft_seconds")
	case l.VCPU > max.VCPU:
		return mismatch("vcpu %.2f exceeds the %.2f this provider can enforce", l.VCPU, max.VCPU)
	case l.MemoryBytes > max.MemoryBytes:
		return mismatch("memory_bytes %d exceeds the %d this provider can enforce", l.MemoryBytes, max.MemoryBytes)
	case l.DiskBytes > max.DiskBytes:
		return mismatch("disk_bytes %d exceeds the %d this provider can enforce", l.DiskBytes, max.DiskBytes)
	case l.MaxPIDs > max.MaxPIDs:
		return mismatch("max_pids %d exceeds the %d this provider can enforce", l.MaxPIDs, max.MaxPIDs)
	case l.WallClockHardSeconds > max.WallClockHardSeconds:
		return mismatch("wall_clock_hard_seconds %d exceeds the %d this provider allows", l.WallClockHardSeconds, max.WallClockHardSeconds)
	case req.Egress.Mode != "default_deny" && req.Egress.Mode != "none":
		return mismatch("egress mode %q is not supported", req.Egress.Mode)
	case req.Egress.Mode == "none" && len(req.Egress.Allow) > 0:
		return mismatch("egress mode none cannot carry an allow list")
	// SBX-007. The two modes are ordered, not alternatives: a node with no
	// egress route declares only `none` and can still carry a run that is
	// allowed to reach nothing, but never one that names a destination - it has
	// no route to offer, and substituting a weaker mode is never allowed
	// whatever the request said.
	case len(req.Egress.Allow) > 0 && !contains(c.EgressModes, "default_deny"):
		return mismatch("this provider has no egress route, so it cannot allow %d destination(s)", len(req.Egress.Allow))
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// HashRequest decides whether a re-sent dispatch is the same one. It hashes the
// re-marshalled request rather than the raw body so that key order and
// whitespace do not turn a legitimate retry into a 409. Exported because the
// driver stamps it onto the sandbox, which is how the answer survives a restart.
func HashRequest(req RunRequest) string {
	b, err := json.Marshal(req)
	if err != nil {
		// Marshalling a RunRequest cannot fail unless provider_extensions holds
		// something unserializable, which arrived as JSON in the first place.
		return "unhashable"
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func newHandle() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// secretsOf collects the values this provider is about to inject into a
// sandbox, so workload output can be scrubbed of them before it is stored or
// shown (iron rule 11).
func secretsOf(req RunRequest) []string {
	var out []string
	if req.ModelGateway != nil && req.ModelGateway.VirtualKey != "" {
		out = append(out, req.ModelGateway.VirtualKey)
	}
	for _, g := range req.ObjectGrants {
		if g.URL != "" {
			out = append(out, g.URL)
		}
	}
	return out
}

// artifactGrantOf is the attempt's single write authorization, if it has one.
func artifactGrantOf(req RunRequest) *ObjectGrant {
	for i, g := range req.ObjectGrants {
		if g.Purpose == "artifact_upload" && g.Access == "write" {
			return &req.ObjectGrants[i]
		}
	}
	return nil
}

func mask(s string, secrets []string) string {
	for _, sec := range secrets {
		if sec == "" {
			continue
		}
		s = strings.ReplaceAll(s, sec, "***")
	}
	return s
}
