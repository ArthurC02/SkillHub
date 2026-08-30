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
	// ReadTrace returns a bounded JSONL chunk beginning at byte offset from
	// <out>/trace/events.jsonl (TRACE-002). An absent file is not an
	// error: it means the workload has not emitted anything yet. The bytes are
	// untrusted workload output and are never parsed by the driver.
	ReadTrace(ctx context.Context, providerRunID string, offset int64) (data []byte, more bool, err error)
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
	// Rootless reports whether workloads actually run unprivileged on this
	// driver, on this host, right now. It is a detection and not a policy: the
	// dispatch gate refuses a provider that answers false, and that refusal is
	// the point of the field. Same shape as localdrv.Reaping() and
	// ResourceEnforcement() — a thing a driver cannot promise belongs somewhere
	// a caller and a test can both read, not in a constant.
	//
	// It used to be a literal `true` written into Capability, which was true of
	// dockerdrv (New refuses uid/gid 0) and unchecked on localdrv, where the
	// workload runs as whatever account sandboxd runs as — very possibly a
	// local administrator on M6's target machine.
	Rootless() bool
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
	// MaxResourcesUnenforced names the ceilings in MaxResources that this
	// deployment carries a number for but the operating system does not hold it
	// to. Empty is what a production node must be able to say. Without it a
	// driver that detected it enforces nothing could only lie in MaxResources or
	// refuse to run, because every ceiling there is required to be present.
	MaxResourcesUnenforced []string
	// ReapsDetachedDescendants: whether ending a run also ends a descendant that
	// left the process group or job it started in. False is a real answer, not a
	// missing one - see Isolation.ReapsDetachedDescendants.
	ReapsDetachedDescendants bool
	EgressModes              []string
	// EgressAllow is what tools/egress/render.py rendered onto this node from
	// infra/egress/allowlist.yaml: the destinations there is an nftables accept
	// rule for. Empty means this node routes nowhere, and EgressModesFor turns
	// that into a capability of `none` rather than a claim it cannot keep.
	EgressAllow []EgressDestination
	// EgressUnenforced says this node filters nothing: the workload reaches
	// whatever the host reaches, so EgressAllow describes what the user agreed
	// to rather than what anything holds the run to. Set only by a driver that
	// has no network boundary at all (localdrv, ADR-059); a node that renders
	// nftables rules leaves it false and accept() holds every destination to
	// them.
	EgressUnenforced bool
	Slots            int
	CancelGrace      time.Duration
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
	secrets []string
	// unmaskable marks an entry whose secrets are gone: an adopted attempt is
	// rebuilt from container labels, and labels must never carry a Virtual Key
	// or a pre-signed URL (D-05). Nothing can scrub what nothing knows, so the
	// workload's output is withheld instead of shipped raw (iron rule 11).
	unmaskable bool
	timedOut   bool
	cancelled  bool
	// traceURL is this attempt's ingestion destination, credential included
	// (TRACE-002). Empty means nothing is collecting and no draining happens.
	traceURL string
	// traceOffset is the byte immediately after the last complete JSONL line
	// accepted by the platform. Failed batches do not advance it.
	traceOffset int64
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
	// p02 is the resident P-02 probe (ADR-022 T10). Nil on a manager built
	// without one, which is what every test that is not about P-02 gets.
	p02 *P02Probe

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
			// TRACE-002: true only when a sink is actually wired. The claim is
			// "this provider will push trace events if you give it an ingestion
			// URL", and a build with no sink cannot keep it.
			EventStreaming: m.sink != nil,
		},
		Availability: &Availability{
			ConcurrentRunSlots: free,
			// A node that cannot vouch for its own P-02 block takes itself out
			// of rotation. `fail` is a breach and `unknown` is a probe that did
			// not complete; neither is a node the scheduler should be handed
			// untrusted code for, and `healthy` is the switch RUN-005 already
			// reads, so no new scheduling path is needed for either.
			Healthy: m.drv.Healthy(ctx) && m.p02Healthy(),
		},
		Security: m.securityCapability(),
	}
}

// p02Healthy reports whether the node's own P-02 reading allows it to serve.
//
// `not_configured` serves. That is deliberate and it is the local dev provider:
// a node nobody gave a forbidden-address list to has not failed a check, it was
// never given one. Production refuses to START in that state (main), which is
// where the asymmetry belongs - a node that is already running is not made
// safer by a capability field, and one that never starts cannot be dispatched to.
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

// WithP02 attaches the resident P-02 probe (ADR-022 T10) and starts it.
//
// The breach action is here rather than in the probe because only the Manager
// owns the runs. T10 says a detected connection terminates the run; the probe
// is not a run and the breach is a property of the node's wiring, so every live
// sandbox on it is reaching the same place. Destroying them is the one action
// available that is proportionate to that.
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

// terminateEveryRun is the node's own first action on a P-02 breach. It does not
// wait for the platform to notice: the platform learns about this by polling
// GET /capability, and between two polls is exactly the window in which
// untrusted code is talking to the core database.
func (m *Manager) terminateEveryRun(r P02Result) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.runs))
	for id := range m.runs {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		// Destroy, not Cancel: cancel is a cooperative stop with a grace period,
		// and a workload that has a route to the database is not one to
		// negotiate a shutdown window with.
		if err := m.Destroy(context.Background(), id); err != nil && m.log != nil {
			m.log.Error("could not destroy a run after a P-02 breach",
				"provider_run_id", id, "error", err)
		}
	}
	if m.log != nil {
		m.log.Error("P-02 breach: terminated every live run on this node",
			"runs", len(ids), "detail", r.Detail)
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
	// A node in P-02 breach refuses work, and this is a belt rather than the
	// braces: the capability response already reports itself unhealthy and the
	// scheduler stops sending. But `healthy` is polled and cached (Registry has
	// a TTL), so a dispatch decided before the last reading can still arrive,
	// and the whole point of this state is that the window matters.
	if m.p02 != nil {
		if r := m.p02.Result(); r.State == P02Fail {
			return ProviderRun{}, false, &RunError{
				Class:     ClassProvision,
				Message:   "this node is refusing work: its P-02 probe reached an address a sandbox must not (" + r.Detail + ")",
				Retryable: true,
			}
		}
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
	case out.ExitCode == exitTokenBudget:
		// Completed, like any other workload that stopped itself: the harness
		// enforced the run's own token ceiling and shut the turn down cleanly,
		// so its artifacts and the tail of its trace are collected as usual.
		// Only the message differs, and it has to: "workload exited with code 9"
		// in a run's failure detail would be the platform declining to say what
		// its own limit did (NFR-001).
		e.run.State, res.Status = StateCompleted, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: "the workload reached the run's token ceiling (PDM-005 5.2a); see the trace's token_budget_exceeded event for the count"}
		e.run.StateReason = res.Error.Message
	default:
		// Ran to its own end and reported failure: completed, not failed.
		e.run.State, res.Status = StateCompleted, ResultFailed
		res.Error = &RunError{Class: ClassExecution, Message: fmt.Sprintf("workload exited with code %d", out.ExitCode)}
	}
	if e.unmaskable && res.AgentOutput != "" {
		// Losing the tail of the output costs a user one debugging aid. Shipping
		// it unmasked puts this run's Virtual Key, its pre-signed grant URL and
		// its trace ingestion token into stored, displayed text (NFR-002,
		// TRACE-001), which costs everyone.
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
	if err := m.drv.Remove(ctx, id); err != nil {
		// Keep the entry and its slot while the runtime still holds resources.
		// The platform will retry this idempotent operation.
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
			// The labels cannot rebuild the RunRequest, so this entry has no
			// secrets list and never will: whatever the workload wrote cannot be
			// masked and must not leave this process.
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
	// A token ceiling above what this provider counts to would be a limit shown
	// to a user and then not applied, which is the state PDM-005 5.2a was closed
	// to end. A request with no token_budget at all is still accepted: the run is
	// then bounded by spend, rate and wall clock, and nobody was told otherwise.
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
	// SBX-007. The two modes are ordered, not alternatives: a node with no
	// egress route declares only `none` and can still carry a run that is
	// allowed to reach nothing, but never one that names a destination - it has
	// no route to offer, and substituting a weaker mode is never allowed
	// whatever the request said.
	case len(req.Egress.Allow) > 0 && !contains(c.EgressModes, "default_deny"):
		return mismatch("this provider has no egress route, so it cannot allow %d destination(s)", len(req.Egress.Allow))
	}

	// ADR-022 A1-e. Every destination the request names has to be one this node
	// actually rendered a rule for, and the check is on the address and the
	// port, not on the purpose: a request naming any host at all while calling
	// it `model_gateway` would otherwise be accepted.
	//
	// Refusing here rather than dispatching is the whole requirement. A run sent
	// to a node with no route for its destination does not fail fast - it starts,
	// the workload's first model call hangs, and it ends at the wall clock as a
	// timeout, which reads as the skill being slow. The user was shown that
	// destination and agreed to it (02:TEST-005); this is the first code that
	// holds anything to it.
	//
	// A node that enforces nothing is exempt, and the exemption is not a
	// loophole in A1-e - it is the same requirement read correctly. A1-e refuses
	// a destination the node has no ROUTE to, because accepting it would end in
	// a timeout that reads as a slow skill. A host process routes to everything,
	// so there is no destination it would time out on, and refusing here would
	// be the false statement rather than the true one. What it cannot promise is
	// the other half - that the workload reaches ONLY these - and that is what
	// EgressUnenforced puts in the capability response for the platform to gate
	// on (04 丙-98), instead of being silently absent as it was until
	// 2026-08-30.
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

// renderedSummary names what this node does route to, so a capability_mismatch
// is actionable without shell access to the node. Purpose and port only: the
// address is in the node's own rendered file and repeating it here would put
// the topology into every rejected dispatch's error.
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
	// The trace ingestion URL is the third injected credential: the driver puts
	// it in the workload's environment as SKILLHUB_TRACE_URL, so an `env` dump or
	// a framework that prints its config on error carries it straight into
	// out.Output. What is secret is the last path segment, not the whole URL
	// (the platform mints base + "/internal/trace/" + token), so register the
	// token: that redacts a bare token as well as one inside an echoed URL, it
	// is the same value the platform's own masker treats as known, and it leaves
	// the non-secret base in the output where it helps rather than hides.
	if u := req.Trace.IngestionURL; u != "" {
		if i := strings.LastIndex(u, "/"); i >= 0 && i < len(u)-1 {
			out = append(out, u[i+1:])
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
