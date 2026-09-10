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

type Driver interface {
	Start(ctx context.Context, providerRunID string, req RunRequest) error

	Wait(ctx context.Context, providerRunID string) (Outcome, error)

	Stop(ctx context.Context, providerRunID string, grace time.Duration) error

	Remove(ctx context.Context, providerRunID string) error

	ReadTrace(ctx context.Context, providerRunID string, offset int64) (data []byte, more bool, err error)

	ReadArtifacts(ctx context.Context, providerRunID string) ([]byte, error)

	WorkloadDone(ctx context.Context, providerRunID string) (bool, error)
	ReleaseWorkload(ctx context.Context, providerRunID string) error

	Adopt(ctx context.Context) ([]Adopted, error)

	Healthy(ctx context.Context) bool

	Rootless() bool
}

type Outcome struct {
	ExitCode  int
	OOMKilled bool
	Output    string
}

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

type Config struct {
	Provider       string
	Runtimes       []RuntimeCapability
	MaxResources   ResourceLimits
	IsolationLevel string

	MaxResourcesUnenforced []string

	ReapsDetachedDescendants bool
	EgressModes              []string

	EgressAllow []EgressDestination

	EgressUnenforced bool
	Slots            int
	CancelGrace      time.Duration
}

var (
	ErrConflict = errors.New("run already dispatched with different content")

	ErrNoSlot = errors.New("no free run slot")

	ErrNotFound = errors.New("no run with this handle")
)

type entry struct {
	run  ProviderRun
	hash string
	stop context.CancelFunc

	secrets []string

	unmaskable bool
	timedOut   bool
	cancelled  bool

	traceURL string

	traceOffset int64

	artifactGrant *ObjectGrant

	limits ResourceLimits

	startCancel context.CancelFunc

	artifacts          []Artifact
	artifactsTruncated bool
}

type Manager struct {
	drv Driver
	cfg Config
	now func() time.Time
	log *slog.Logger

	sink    TraceSink
	metrics *Metrics

	p02 *P02Probe

	mu    sync.Mutex
	runs  map[string]*entry
	byKey map[string]string
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

func (m *Manager) WithTrace(sink TraceSink, mx *Metrics) *Manager {
	m.sink, m.metrics = sink, mx
	return m
}

func (m *Manager) Capability(ctx context.Context) ProviderCapability {
	m.mu.Lock()
	live := len(m.runs)
	m.mu.Unlock()
	free := m.cfg.Slots - live
	if free < 0 {
		free = 0
	}
	return ProviderCapability{
		Provider:               m.cfg.Provider,
		Runtimes:               m.cfg.Runtimes,
		MaxResources:           m.cfg.MaxResources,
		MaxResourcesUnenforced: m.cfg.MaxResourcesUnenforced,
		Isolation: Isolation{
			Level:                    m.cfg.IsolationLevel,
			Rootless:                 m.drv.Rootless(),
			DedicatedWorkspacePerRun: true,
			ReapsDetachedDescendants: m.cfg.ReapsDetachedDescendants,
		},
		Network: &NetworkCapability{
			EgressModes:      m.cfg.EgressModes,
			EgressUnenforced: m.cfg.EgressUnenforced,
			PrivateNetwork:   true,
		},
		Features: &Features{
			ToolCalls: true,
			Scripts:   true,
			Artifacts: true,

			EventStreaming: m.sink != nil,
		},
		Availability: &Availability{
			ConcurrentRunSlots: free,

			Healthy: m.drv.Healthy(ctx) && m.p02Healthy(),
		},
		Security: m.securityCapability(),
	}
}

func (m *Manager) p02Healthy() bool {
	if m.p02 == nil {
		return true
	}
	switch m.p02.Result().State {
	case P02Fail, P02Unknown:
		return false
	default:
		return true
	}
}

func (m *Manager) securityCapability() *SecurityCapability {
	if m.p02 == nil {
		return nil
	}
	r := m.p02.Result()
	return &SecurityCapability{P02Probe: &r}
}

func (m *Manager) WithP02(ctx context.Context, probe *P02Probe) *Manager {
	m.p02 = probe
	if probe == nil {
		return m
	}
	prober, _ := m.drv.(EgressProber)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		probe.Run(ctx, prober, m.terminateEveryRun, m.log)
	}()
	return m
}

func (m *Manager) terminateEveryRun(r P02Result) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.runs))
	for id, e := range m.runs {
		ids = append(ids, id)

		if e.startCancel != nil {
			e.startCancel()
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {

		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
			defer cancel()
			if err := m.Destroy(ctx, id); err != nil && m.log != nil {
				m.log.Error("could not destroy a run after a P-02 breach",
					"provider_run_id", id, "error", err)
			}
		}()
	}
	wg.Wait()
	if m.log != nil {
		m.log.Error("P-02 breach: terminated every live run on this node",
			"runs", len(ids), "detail", r.Detail)
	}
}

const teardownTimeout = 30 * time.Second

func (m *Manager) destroyBounded(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), teardownTimeout)
	defer cancel()
	return m.Destroy(ctx, id)
}

func (m *Manager) Create(ctx context.Context, req RunRequest) (run ProviderRun, created bool, err error) {
	if err := validate(req); err != nil {
		return ProviderRun{}, false, err
	}
	if re := m.cfg.accept(req); re != nil {
		return ProviderRun{}, false, re
	}

	if err := m.p02Refusal(); err != nil {
		return ProviderRun{}, false, err
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
	startCtx, cancelStart := context.WithCancel(ctx)
	e := &entry{
		hash:          hash,
		secrets:       secretsOf(req),
		traceURL:      req.Trace.IngestionURL,
		artifactGrant: artifactGrantOf(req),
		limits:        req.ResourceLimits,
		startCancel:   cancelStart,
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
	defer cancelStart()

	m.log.Info("run dispatched", "run_id", req.RunID, "attempt", req.Attempt, "provider_run_id", id)
	m.metrics.dispatched()
	m.metrics.active(live)

	if err := m.p02Refusal(); err != nil {
		_ = m.destroyBounded(id)
		return ProviderRun{}, false, err
	}

	if err := m.drv.Start(startCtx, id, req); err != nil {
		if refusal := m.p02Refusal(); refusal != nil {
			_ = m.destroyBounded(id)
			return ProviderRun{}, false, refusal
		}
		m.finish(id, Outcome{}, &RunError{
			Class:     ClassProvision,
			Message:   "sandbox could not be created",
			Retryable: true,
		})
		m.log.Error("sandbox start failed", "provider_run_id", id, "err", err)
		run, _, snapshotErr := m.snapshot(id)
		if snapshotErr != nil {
			_ = m.destroyBounded(id)
			return ProviderRun{}, false, &RunError{
				Class: ClassProvision, Message: "sandbox creation was revoked", Retryable: true,
			}
		}
		return run, true, nil
	}

	if refusal := m.p02Refusal(); refusal != nil {
		_ = m.destroyBounded(id)
		return ProviderRun{}, false, refusal
	}
	m.mu.Lock()
	current := m.runs[id]
	var running ProviderRun
	cancelled := false
	if current != nil {
		current.startCancel = nil
		current.run.State = StateRunning
		current.run.StartedAt = m.now()
		cancelled = current.cancelled
		running = current.run
		running.ObservedAt = m.now()
	}
	m.mu.Unlock()
	if current == nil {
		_ = m.destroyBounded(id)
		return ProviderRun{}, false, &RunError{
			Class: ClassProvision, Message: "sandbox creation was revoked", Retryable: true,
		}
	}

	soft := time.Duration(req.ResourceLimits.WallClockSoftSeconds) * time.Second
	hard := time.Duration(req.ResourceLimits.WallClockHardSeconds) * time.Second
	m.watch(id, soft, hard)
	if cancelled {

		stopCtx, stopCancel := context.WithTimeout(context.Background(), m.cfg.CancelGrace+time.Second)
		defer stopCancel()
		if err := m.drv.Stop(stopCtx, id, m.cfg.CancelGrace); err != nil {
			m.log.Error("post-start cancel stop failed", "provider_run_id", id, "err", err)
		}
	}
	return running, true, nil
}

func (m *Manager) p02Refusal() error {
	if m.p02 == nil {
		return nil
	}
	r := m.p02.Result()
	if r.State != P02Fail && r.State != P02Unknown {
		return nil
	}
	return &RunError{
		Class:     ClassProvision,
		Message:   "this node is refusing work: its P-02 isolation check has not passed",
		Retryable: true,
	}
}

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
		stopCtx, stopCancel := context.WithTimeout(context.Background(), hard-soft+time.Second)
		defer stopCancel()
		if err := m.drv.Stop(stopCtx, id, hard-soft); err != nil {
			m.log.Error("timeout stop failed", "provider_run_id", id, "err", err)
		}
	})

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer cancel()

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

		Artifacts:          e.artifacts,
		ArtifactsTruncated: e.artifactsTruncated,
	}
	switch {
	case e.cancelled:
		e.run.State, res.Status = StateCancelled, ResultCancelled
		res.Error = &RunError{Class: ClassCancelled, Message: "stopped on request"}
	case e.timedOut:

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

		e.run.State, res.Status = StateCompleted, ResultSucceeded
	case out.ExitCode == exitTokenBudget:

		e.run.State, res.Status = StateCompleted, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: "the workload reached the run's token ceiling (PDM-005 5.2a); see the trace's token_budget_exceeded event for the count"}
		e.run.StateReason = res.Error.Message
	default:

		e.run.State, res.Status = StateCompleted, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: fmt.Sprintf("workload exited with code %d", out.ExitCode)}
	}
	if e.unmaskable && res.AgentOutput != "" {

		res.AgentOutput = ""
		e.run.StateReason = "agent output withheld: its secrets could not be masked after a provider restart"
	}
	if !e.run.StartedAt.IsZero() {
		res.Usage = &RunUsage{WallClockSeconds: now.Sub(e.run.StartedAt).Seconds()}
	}
	e.run.Result = res
	m.metrics.finished(res.Status)
	m.log.Info("run finished", "provider_run_id", id, "state", e.run.State, "status", res.Status)
}

func (m *Manager) Get(id string) (ProviderRun, error) {
	run, _, err := m.snapshot(id)
	return run, err
}

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

func (m *Manager) Cancel(ctx context.Context, id string) (ProviderRun, error) {
	m.mu.Lock()
	e := m.runs[id]
	if e == nil {
		m.mu.Unlock()
		return ProviderRun{}, ErrNotFound
	}
	terminal := e.run.State.Terminal()
	var cancelStart context.CancelFunc
	if !terminal {
		e.cancelled = true
		cancelStart = e.startCancel
		if e.run.CancelRequestedAt.IsZero() {
			e.run.CancelRequestedAt = m.now()
		}
	}
	run := e.run
	run.ObservedAt = m.now()
	m.mu.Unlock()

	if !terminal {
		if cancelStart != nil {
			cancelStart()
		}
		m.log.Info("cancel requested", "provider_run_id", id)
		if err := m.drv.Stop(ctx, id, m.cfg.CancelGrace); err != nil {
			m.log.Error("cancel stop failed", "provider_run_id", id, "err", err)
		}
	}
	return run, nil
}

func (m *Manager) Destroy(ctx context.Context, id string) error {
	m.mu.Lock()
	if e := m.runs[id]; e != nil && e.startCancel != nil {
		e.startCancel()
	}
	m.mu.Unlock()
	if err := m.drv.Remove(ctx, id); err != nil {

		m.log.Error("destroy failed", "provider_run_id", id, "err", err)
		return err
	}

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

	m.log.Info("run destroyed", "provider_run_id", id)
	return nil
}

func (m *Manager) Adopt(ctx context.Context) error {
	found, err := m.drv.Adopt(ctx)
	if err != nil {
		return err
	}
	for _, a := range found {
		e := &entry{
			hash: a.RequestHash,

			unmaskable: true,
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
	case l.VCPU <= 0 || l.MemoryBytes <= 0 || l.DiskBytes <= 0 || l.MaxPIDs <= 0 || l.MaxOpenFiles <= 0 ||
		l.WallClockSoftSeconds <= 0 || l.WallClockHardSeconds <= 0 ||
		l.ArtifactTotalBytes <= 0 || l.ArtifactFileBytes <= 0:
		return mismatch("resource_limits must set every ceiling: this provider will not run unbounded")
	case max.VCPU <= 0 || max.MemoryBytes <= 0 || max.DiskBytes <= 0 || max.MaxPIDs <= 0 || max.MaxOpenFiles <= 0 ||
		max.WallClockSoftSeconds <= 0 || max.WallClockHardSeconds <= 0 ||
		max.ArtifactTotalBytes <= 0 || max.ArtifactFileBytes <= 0:
		return mismatch("provider capability must declare every resource ceiling")
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
	case l.MaxOpenFiles > max.MaxOpenFiles:
		return mismatch("max_open_files %d exceeds the %d this provider can enforce", l.MaxOpenFiles, max.MaxOpenFiles)
	case l.WallClockSoftSeconds > max.WallClockSoftSeconds:
		return mismatch("wall_clock_soft_seconds %d exceeds the %d this provider allows", l.WallClockSoftSeconds, max.WallClockSoftSeconds)
	case l.WallClockHardSeconds > max.WallClockHardSeconds:
		return mismatch("wall_clock_hard_seconds %d exceeds the %d this provider allows", l.WallClockHardSeconds, max.WallClockHardSeconds)
	case l.ArtifactTotalBytes > max.ArtifactTotalBytes:
		return mismatch("artifact_total_bytes %d exceeds the %d this provider can enforce", l.ArtifactTotalBytes, max.ArtifactTotalBytes)
	case l.ArtifactFileBytes > max.ArtifactFileBytes:
		return mismatch("artifact_file_bytes %d exceeds the %d this provider can enforce", l.ArtifactFileBytes, max.ArtifactFileBytes)

	case l.TokenBudget != nil && max.TokenBudget == nil:
		return mismatch("provider capability does not declare a token budget")
	case l.TokenBudget != nil && (l.TokenBudget.MaxInputTokens <= 0 || l.TokenBudget.MaxOutputTokens <= 0):
		return mismatch("token_budget must set both ceilings")
	case l.TokenBudget != nil &&
		(l.TokenBudget.MaxInputTokens > max.TokenBudget.MaxInputTokens ||
			l.TokenBudget.MaxOutputTokens > max.TokenBudget.MaxOutputTokens):
		return mismatch("token_budget %d/%d exceeds the %d/%d this provider can enforce",
			l.TokenBudget.MaxInputTokens, l.TokenBudget.MaxOutputTokens,
			max.TokenBudget.MaxInputTokens, max.TokenBudget.MaxOutputTokens)
	case req.Egress.Mode != "default_deny" && req.Egress.Mode != "none":
		return mismatch("egress mode %q is not supported", req.Egress.Mode)
	case req.Egress.Mode == "none" && len(req.Egress.Allow) > 0:
		return mismatch("egress mode none cannot carry an allow list")

	case len(req.Egress.Allow) > 0 && !contains(c.EgressModes, "default_deny"):
		return mismatch("this provider has no egress route, so it cannot allow %d destination(s)", len(req.Egress.Allow))
	}

	if c.EgressUnenforced {
		return nil
	}
	for _, want := range req.Egress.Allow {
		routed := false
		for _, have := range c.EgressAllow {
			if have.routes(want) {
				routed = true
				break
			}
		}
		if !routed {
			host, port, ok := hostPort(want.URL)
			if !ok {
				return mismatch("egress destination %q for %s is not a URL naming a host and port, "+
					"so no accept rule could match it", want.URL, want.Purpose)
			}
			return mismatch("this node renders no egress rule for %s at %s:%d; "+
				"it routes to %s (ADR-022 A1-e)", want.Purpose, host, port, c.renderedSummary())
		}
	}
	return nil
}

func (c Config) renderedSummary() string {
	if len(c.EgressAllow) == 0 {
		return "nothing (no destination has a pinned address)"
	}
	parts := make([]string, 0, len(c.EgressAllow))
	for _, d := range c.EgressAllow {
		parts = append(parts, fmt.Sprintf("%s:%d", d.Purpose, d.Port))
	}
	return strings.Join(parts, ", ")
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// HashRequest hashes the re-marshalled request rather than the raw body, so
// key order and whitespace differences don't change the result.
func HashRequest(req RunRequest) string {
	b, err := json.Marshal(req)
	if err != nil {

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
	// Registers only the URL's last path segment (the token), so it also
	// redacts a bare token echoed on its own.
	if u := req.Trace.IngestionURL; u != "" {
		if i := strings.LastIndex(u, "/"); i >= 0 && i < len(u)-1 {
			out = append(out, u[i+1:])
		}
	}
	return out
}

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
