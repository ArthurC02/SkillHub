package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/eval"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
	"github.com/ArthurC02/skillhub/services/platform/internal/testlab"
	"github.com/ArthurC02/skillhub/services/platform/internal/trace"
)

var (
	// ErrNotFound covers "no such run" and "not yours" alike: existence is itself
	// private (WS-006), so callers turn this into a 404 either way.
	ErrNotFound = errors.New("run not found")
	// ErrIllegalTransition is a bug in the caller - the pair is not in the state
	// machine at all. It never depends on what is in the database.
	ErrIllegalTransition = errors.New("illegal run status transition")
	// ErrConflict is a legal transition applied to a run that had already moved.
	// Expected under at-least-once queue delivery, not an error to alarm about.
	ErrConflict = errors.New("run is no longer in the expected status")
	// ErrRunFinished is a cancel arriving after the run reached a terminal state.
	ErrRunFinished = errors.New("run has already finished")
)

// providerUnassigned is the provider of a run that has not been scheduled yet.
// The scheduler overwrites it the moment it picks one (RUN-005); runs.provider is
// NOT NULL, and naming the gap is better than defaulting to whichever provider
// happens to exist first.
const providerUnassigned = "unassigned"

// Failure classes written to runs.failure_class. The vocabulary is the platform's
// own and is fixed by a CHECK constraint in 0018 - run_attempts.error_class carries
// the provider's RunError.class separately, per attempt.
const (
	failureProvider   = "provider_error"
	failureWorkload   = "workload_error"
	failureTimeout    = "timeout"
	failureCancelled  = "cancelled"
	failureNoProvider = "capability_mismatch"
	failurePlatform   = "platform_error"
)

// Service owns run state. Every method that changes a run does so in one
// transaction that also writes the status history, the audit event and the outbox
// event (iron rule 9).
type Service struct {
	Pool *pgxpool.Pool
	// Queue enqueues the execution job in the same transaction as the run row, so
	// a committed run always has a job and a rolled-back one never does. Nil is
	// allowed: the run is created and stays queued forever, which is what a
	// deployment without a worker actually does.
	Queue *river.Client[pgx.Tx]
	// Providers is the configured sandbox fleet (RUN-005). Nil or empty is a
	// deployment with no sandbox: runs are still accepted and then fail saying so,
	// rather than being rejected as if the user had asked for something impossible.
	Providers *Registry
	// Store reads the stored skill package, for the one question the pre-run
	// permission summary cannot answer from the database: does this version carry
	// a script (02:TEST-005). Nil means the scan reports itself unavailable rather
	// than reporting a clean package.
	Store ObjectStore
	// MaxAttempts bounds automatic retries of one run (ADR-004: no unbounded
	// retries). Zero means defaultMaxAttempts.
	MaxAttempts int
	// PollInterval is how often a dispatched attempt is read back from the
	// provider. Zero means defaultPollInterval.
	PollInterval time.Duration
	// TraceSigner mints the per-attempt trace ingestion credential (TRACE-002).
	// Nil, or one with no secret, means no ingestion URL is handed to the
	// provider and no events are collected - which is honest for a deployment
	// that has not configured the secret, and better than an open endpoint.
	TraceSigner *trace.Signer
	// TraceIngestBaseURL is the origin an execution node reaches the control
	// plane on (SKILLHUB_TRACE_INGEST_URL). Empty has the same effect as a
	// disabled signer.
	TraceIngestBaseURL string
	// Quota is the PDM-010 free run allowance this deployment applies (ADR-028).
	// The zero value enforces nothing and displays nothing, which is the honest
	// state of a build with no allowance — see quota.go for why enforcement had to
	// exist before the display did.
	Quota QuotaLimits
	// Gateway mints and revokes the per-run model credential (SBX-008, ADR-017).
	// Nil is a deployment with no model gateway: no grant is minted, the egress
	// allow list defaultPolicy writes stays empty, and the sandbox gets no route
	// out. Only the worker sets it - the API never mints a key.
	Gateway *Gateway
}

func (s *Service) maxAttempts() int {
	if s.MaxAttempts > 0 {
		return s.MaxAttempts
	}
	return defaultMaxAttempts
}

func (s *Service) pollInterval() time.Duration {
	if s.PollInterval > 0 {
		return s.PollInterval
	}
	return defaultPollInterval
}

func (s *Service) providers() *Registry {
	if s.Providers == nil {
		return &Registry{}
	}
	return s.Providers
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

// ResourceLimits mirrors ResourceLimits in contracts/openapi/sandbox-provider.yaml.
// Values are PDM-005 5.2. Changing one means changing both - the contract is the
// spec, this is the snapshot written into policy_snapshot so TEST-005 can show the
// user what their run will be held to, and so a past run stays explainable after
// the defaults change.
type ResourceLimits struct {
	VCPU                 float64 `json:"vcpu"`
	MemoryBytes          int64   `json:"memory_bytes"`
	DiskBytes            int64   `json:"disk_bytes"`
	MaxPIDs              int     `json:"max_pids"`
	MaxOpenFiles         int     `json:"max_open_files"`
	WallClockSoftSeconds int     `json:"wall_clock_soft_seconds"`
	WallClockHardSeconds int     `json:"wall_clock_hard_seconds"`
	ArtifactTotalBytes   int64   `json:"artifact_total_bytes"`
	ArtifactFileBytes    int64   `json:"artifact_file_bytes"`
	TokenBudget          struct {
		MaxInputTokens  int `json:"max_input_tokens"`
		MaxOutputTokens int `json:"max_output_tokens"`
	} `json:"token_budget"`
}

func DefaultResourceLimits() ResourceLimits {
	l := ResourceLimits{
		VCPU:                 2,
		MemoryBytes:          4 << 30, // 4 GiB
		DiskBytes:            8 << 30, // 8 GiB: /work 6 + /out 2
		MaxPIDs:              256,
		MaxOpenFiles:         1024,
		WallClockSoftSeconds: 600, // reaching this marks the run timed_out
		WallClockHardSeconds: 900, // forced destroy
		ArtifactTotalBytes:   100 << 20,
		ArtifactFileBytes:    25 << 20,
	}
	// Enforced, and not by the Virtual Key's max_budget - prompt caching decoupled
	// spend from token count by 7-8x (PDM-005 5.2a). The counting happens in the
	// sandbox harness, which is the only party that sees a per-response token
	// count: RunUsage carries none back here. These numbers travel to it inside
	// this same snapshot, so the ceiling that stops a run is the one the pre-run
	// permission summary showed the user and they confirmed (02:TEST-005).
	l.TokenBudget.MaxInputTokens = 300_000
	l.TokenBudget.MaxOutputTokens = 60_000
	return l
}

// policySnapshot is runs.policy_snapshot: what the run is held to, frozen at
// creation (ADR-003) and read back by the scheduler so capability matching happens
// against what the user was shown, not against today's defaults.
type policySnapshot struct {
	ResourceLimits ResourceLimits `json:"resource_limits"`
	Egress         EgressPolicy   `json:"egress"`
}

// defaultPolicy is the policy a new run gets, and the ONLY place it is written.
//
// Egress is default-deny. The allow list names the model gateway when one is
// configured (SBX-007: the sandbox is placed on a network where that address,
// and nothing else, is reachable) and is otherwise empty, which the dev provider
// serves as `--network none` - no egress at all, strictly stronger. Object
// storage and trace ingestion are deliberately *not* on it: the execution node
// moves those bytes on the sandbox's behalf, so the sandbox itself never holds a
// route to the platform's storage (SBX-008).
//
// The URL here is not an authorization - it is the destination the user is shown
// and agrees to before the run starts (02:TEST-005). The credential for it is
// the Virtual Key, minted per attempt at dispatch and never part of this policy.
//
// Every reader goes through here: Create freezes it onto the run, the scheduler
// matches providers against it, and the pre-run summary shows it. A second copy
// would let the screen the user confirms drift away from what the run is
// actually held to, and the hash would keep saying they agreed (02:TEST-005).
func defaultPolicy() policySnapshot {
	allow := []egressAllow{}
	if url := GatewayURL(); url != "" {
		allow = append(allow, egressAllow{Purpose: "model_gateway", URL: url})
	}
	return policySnapshot{
		ResourceLimits: DefaultResourceLimits(),
		Egress:         EgressPolicy{Mode: "default_deny", Allow: allow},
	}
}

func defaultPolicySnapshot() ([]byte, error) { return json.Marshal(defaultPolicy()) }

// CreateParams is one run request. The workspace comes from the session, never
// from the request body (iron rule 3).
type CreateParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID
	// ConfirmedSummaryHash is the pre-run permission summary the user agreed to
	// (02:TEST-005). It is not trusted as an authorization by itself — the summary
	// is rebuilt and the agreement looked up in requirePermissionConfirmation.
	ConfirmedSummaryHash string
}

// Create records a queued run and enqueues its execution job, in one transaction.
//
// The test case is snapshotted here rather than referenced: a run points at frozen
// content (iron rule 4), so later edits to the test case cannot rewrite what a past
// run was asked to do.
//
// This wrapper is only here to give the refusals one exit. create below has a
// dozen error returns spread over four files, and gate B's whole point is that any
// one of them stops the run — so "a run was refused" had a dozen places it could
// have been recorded and, until now, was recorded in none of them. Audited here
// instead of at each check for the same reason requirePermissionConfirmation lives
// in Create rather than in the handler: one choke point cannot be forgotten by the
// next check somebody adds.
func (s *Service) Create(ctx context.Context, p CreateParams) (gen.Run, error) {
	run, err := s.create(ctx, p)
	if err != nil {
		s.auditRefusal(ctx, p, err)
	}
	return run, err
}

// auditRefusal records a gate B refusal (NFR-001). Best effort, deliberately:
// nothing was written by the path that just failed, so there is no transaction to
// join (iron rule 9 has nothing to hold this to) and no state for a missing row to
// contradict. Failing the request because the note about the refusal could not be
// filed would turn a 422 into a 500 and tell the user even less.
//
// Only gate B refusals are recorded. A lookup that came back ErrNotFound is the
// caller asking about something that is not theirs — WS-006 makes that answer
// deliberately indistinguishable from "does not exist", and a row per probe would
// be an enumeration log. Infrastructure errors are not refusals at all; those are
// what slog and the error counters are for.
func (s *Service) auditRefusal(ctx context.Context, p CreateParams, err error) {
	var r refusal
	var reason string
	switch {
	case errors.As(err, &r):
		reason = r.reason
	// The two gate B conditions that live with their features (gateb.go's header
	// lists them). Neither goes through refused(), so they are matched on their
	// sentinels; each has exactly one refusal, so nothing is lost by the coarser
	// match.
	case errors.Is(err, ErrPermissionsNotConfirmed):
		reason = "permissions_unconfirmed"
	case errors.Is(err, ErrNoCompatibleProvider):
		reason = "capability_mismatch"
	default:
		return
	}
	// Identifiers and the reason code only — never the error string, which quotes
	// scan findings and package content (iron rule 11). There is no run to point
	// at, so the resource is the version that was refused.
	if logErr := audit.Log(ctx, s.queries(), audit.Event{
		Actor:        p.Actor,
		Workspace:    p.WorkspaceID,
		Action:       audit.ActionRunRefused,
		ResourceType: audit.ResourceVersion,
		ResourceID:   p.VersionID,
		Metadata:     map[string]any{"reason": reason},
	}); logErr != nil {
		slog.Error("recording a refused run failed", "reason", reason, "error", logErr)
	}
}

func (s *Service) create(ctx context.Context, p CreateParams) (gen.Run, error) {
	// SEC-012 action ①, first of the two entry points (halt.go): while the fleet is
	// held for a P1, no new run is accepted. Before every other check, including the
	// workspace-scoped reads below, because a platform that has stopped is not going
	// to become able to run this by the caller fixing anything about the request —
	// and because a stopped platform should not be doing lookups on its way to
	// saying no.
	if err := s.requireDispatchable(ctx); err != nil {
		return gen.Run{}, err
	}
	// Both reads are workspace scoped, so another workspace's version or test case
	// is "not found" rather than "forbidden" (WS-006).
	version, err := s.queries().GetSkillVersion(ctx, gen.GetSkillVersionParams{
		ID: p.VersionID, WorkspaceID: p.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}
	// The version must belong to the skill in the URL, or /skills/A/runs could run
	// a version of skill B that the caller also happens to own.
	if version.SkillID != p.SkillID {
		return gen.Run{}, ErrNotFound
	}
	// 0023: materials under a licensing hold are not copied into a sandbox.
	// Checked from the skill row before anything else is read, because it is the
	// one refusal that no amount of the caller fixing their request can clear.
	skill, err := s.queries().GetSkill(ctx, gen.GetSkillParams{
		ID: p.SkillID, WorkspaceID: p.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}
	if err := s.requireNotAccessRestricted(skill); err != nil {
		return gen.Run{}, err
	}

	policy, err := defaultPolicySnapshot()
	if err != nil {
		return gen.Run{}, err
	}
	// RUN-005 / ADR-004: work no configured provider can run is refused here,
	// before it is queued, rather than after a user has watched it sit in a queue.
	var decoded policySnapshot
	if err := json.Unmarshal(policy, &decoded); err != nil {
		return gen.Run{}, err
	}
	if err := s.checkSchedulable(ctx, decoded); err != nil {
		return gen.Run{}, err
	}
	// SEC-002 gate B: a version whose static scan is blocking, or that cannot be
	// scanned at all, does not start (02:SEC-003). Before the permission summary,
	// because a run that may not start at all should not ask the user to agree to
	// its permissions first.
	if err := s.requireScanNotBlocking(ctx, version.PackageObjectKey); err != nil {
		return gen.Run{}, err
	}
	// SEC-002 gate B: no run starts on permissions the user has not seen and
	// agreed to, and an agreement to a summary that has since changed does not
	// carry over (02:TEST-005). Checked here rather than in the handler so every
	// caller of Create passes through it. Ordered after checkSchedulable so a
	// fleet that cannot carry the work still answers with the capability reason,
	// which is the more actionable of the two.
	if err := s.requirePermissionConfirmation(ctx, p); err != nil {
		return gen.Run{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	// SEC-002 gate B: the workspace concurrency ceiling (PDM-005 §5.2). Inside the
	// transaction, unlike the three checks above, because it is the only one whose
	// answer another request can invalidate while this one is deciding - it takes a
	// per-workspace lock that is released by this commit.
	if err := s.requireRunSlot(ctx, q, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}
	// PDM-010's free allowance (ADR-028 決策 2). Immediately after the concurrency
	// check and deliberately not anywhere else: this is the only point where a run
	// is about to exist and nothing has been spent on it, and it reuses the lock
	// that call just took, so two simultaneous requests cannot both see the last
	// remaining run. Nothing on the model gateway answers this question — see
	// quota.go for why max_budget, tpm_limit and the concurrency limit are three
	// different brakes and none of them is a monthly allowance.
	if err := s.requireQuota(ctx, q, p.WorkspaceID); err != nil {
		return gen.Run{}, err
	}

	// The test lab owns the snapshot's shape and its hash; this passes it the
	// transaction handle so the snapshot and the run below commit together
	// (iron rule 9). A test case that is missing, in another workspace, or
	// soft-deleted comes back as not found - a deleted draft is not runnable.
	snapshot, err := testlab.CreateSnapshot(ctx, q, p.WorkspaceID, p.TestCaseID)
	if errors.Is(err, testlab.ErrNotFound) {
		return gen.Run{}, ErrNotFound
	}
	if err != nil {
		return gen.Run{}, err
	}

	run, err := q.CreateRun(ctx, gen.CreateRunParams{
		WorkspaceID:        p.WorkspaceID,
		SkillVersionID:     version.ID,
		TestCaseSnapshotID: snapshot.ID,
		Provider:           providerUnassigned,
		// Empty until a provider is chosen and its runtime pinned (RUN-005).
		RuntimeSnapshot: []byte("{}"),
		PolicySnapshot:  policy,
	})
	if err != nil {
		return gen.Run{}, err
	}

	if err := s.record(ctx, q, run, nil, pgtype.UUID{}, "run requested", p.Actor, audit.ActionRunCreate); err != nil {
		return gen.Run{}, err
	}

	if s.Queue != nil {
		// Same unique options the supervisor uses, so a re-enqueue after a restart
		// lands on this job instead of starting a second driver (RUN-008).
		if _, err := s.Queue.InsertTx(ctx, tx, JobArgs{
			RunID:       uuidString(run.ID),
			WorkspaceID: uuidString(run.WorkspaceID),
		}, executeInsertOpts()); err != nil {
			return gen.Run{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Run{}, err
	}
	metrics.RunCreated.Inc()
	return run, nil
}

// TransitionParams is one state change.
type TransitionParams struct {
	WorkspaceID pgtype.UUID
	RunID       pgtype.UUID
	// AttemptID is the attempt this change belongs to; zero before dispatch or
	// when the control plane decides a terminal state with no live attempt.
	AttemptID pgtype.UUID
	From, To  gen.RunStatus
	Reason    string
	// FailureClass is why the run ended badly, in the platform's vocabulary
	// (RUN-006). Empty leaves whatever is already there - it rides along with the
	// status write because the 0005 trigger freezes the row on the way in.
	FailureClass string
	// Actor is the user who caused it; zero for worker-initiated changes.
	Actor pgtype.UUID
}

// Transition applies one state change: the run row, its history, the audit event
// and the outbox event, all in one transaction (iron rule 9). Nothing else in the
// codebase may write runs.status.
func (s *Service) Transition(ctx context.Context, p TransitionParams) (gen.Run, error) {
	if !CanTransition(p.From, p.To) {
		return gen.Run{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, p.From, p.To)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	reason := &p.Reason
	if p.Reason == "" {
		reason = nil
	}
	failureClass := &p.FailureClass
	if p.FailureClass == "" {
		failureClass = nil
	}
	run, err := q.TransitionRun(ctx, gen.TransitionRunParams{
		RunID: p.RunID, WorkspaceID: p.WorkspaceID,
		FromStatus: p.From, ToStatus: p.To, Reason: reason, FailureClass: failureClass,
	})
	// Zero rows means the run moved under us, or does not belong to this
	// workspace. Both are "do not apply this change", never "apply it anyway".
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrConflict
	}
	if err != nil {
		return gen.Run{}, err
	}

	from := p.From
	if err := s.record(ctx, q, run, &from, p.AttemptID, p.Reason, p.Actor, audit.ActionRunTransition); err != nil {
		return gen.Run{}, err
	}

	// TRACE-004: a failure the control plane decided is one the user has to be
	// able to see in the trace. Without this, a run that never reached a sandbox
	// - no provider, provisioning failed, timed out in the queue - would have an
	// empty timeline, which is exactly the case RUN-004 says must still show
	// diagnostics. Written in this transaction, so the event and the state change
	// commit together (iron rule 9).
	if err := s.recordFailureEvent(ctx, q, run, p); err != nil {
		return gen.Run{}, err
	}

	// RUN-002 requires cleanup after a run ends, however it ended: a run that reached a
	// terminal state owes a cleanup, and the job that does it is enqueued in the
	// same transaction as the state change. Nobody has to remember to call it, and
	// a rolled-back transition leaves no orphan cleanup behind (iron rule 9).
	if IsTerminal(p.To) && s.Queue != nil {
		if _, err := s.Queue.InsertTx(ctx, tx, CleanupArgs{
			RunID: uuidString(run.ID), WorkspaceID: uuidString(run.WorkspaceID),
		}, cleanupInsertOpts()); err != nil {
			return gen.Run{}, err
		}
		// EVAL-001, enqueued in the same transaction for the same reason cleanup is:
		// a committed terminal run always has an evaluation job and a rolled-back
		// transition never does (iron rule 9).
		//
		// This does NOT feed back into the run. The job writes `evaluations` and
		// nothing else; `runs.status` and `runs.failure_class` are already final at
		// this point and stay that way whatever the verdict turns out to be
		// (ADR-025). Only `succeeded` and `failed` are evaluated: a cancelled or
		// timed-out run was stopped before it could produce the thing the criteria
		// are about, and paying a judge to say so tells nobody anything.
		if p.To == gen.RunStatusSucceeded || p.To == gen.RunStatusFailed {
			if _, err := s.Queue.InsertTx(ctx, tx, eval.JobArgs{
				RunID: uuidString(run.ID), WorkspaceID: uuidString(run.WorkspaceID),
			}, eval.InsertOpts()); err != nil {
				return gen.Run{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return gen.Run{}, err
	}
	observeTransition(run, p)
	return run, nil
}

// recordFailureEvent writes the orchestrator's own `error` trace event for a run
// that ended badly. Cancellation is excluded: a user stopping their own run is
// not a failure, and putting it in the error list would make the general summary
// (TRACE-006) read as if something went wrong.
func (s *Service) recordFailureEvent(ctx context.Context, q *gen.Queries, run gen.Run, p TransitionParams) error {
	if p.To != gen.RunStatusFailed && p.To != gen.RunStatusTimedOut {
		return nil
	}
	code := p.FailureClass
	if code == "" {
		code = "unclassified"
	}
	return trace.RecordOrchestratorEvent(ctx, q, run.WorkspaceID, run.ID,
		attemptNumber(ctx, q, run), trace.TypeError, "error", map[string]any{
			// ADR-004's taxonomy, as the schema's errorPayload requires it. The
			// platform's own failure_class is the stable diagnostic code (NFR-003).
			"category": failureCategory(p.FailureClass),
			"code":     code,
			"message":  p.Reason,
			// Recording the decision the retry policy took, not making it here.
			"retryable": p.FailureClass == failureProvider,
		})
}

// failureCategory maps runs.failure_class onto the schema's error categories.
// The two vocabularies are not the same list on purpose: failure_class answers
// "what killed the run" for the retry policy, the category answers "which stage"
// for the user reading a timeline.
func failureCategory(failureClass string) string {
	switch failureClass {
	case failureProvider, failureNoProvider:
		return "provision"
	default:
		return "execution"
	}
}

// attemptNumber is the attempt an orchestrator-side event belongs to. A run with
// no attempt yet (refused before dispatch) still needs a stream to write into,
// and attempt 1 is the honest answer: that is the attempt that was being set up.
func attemptNumber(ctx context.Context, q *gen.Queries, run gen.Run) int {
	attempts, err := q.ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil || len(attempts) == 0 {
		return 1
	}
	return int(attempts[len(attempts)-1].AttemptNumber)
}

// observeTransition publishes the run funnel numbers of O11Y-001. It runs after
// the commit: a metric for a transition that rolled back would be a lie, and one
// that is lost because the process died a microsecond later is not.
func observeTransition(run gen.Run, p TransitionParams) {
	if p.To == gen.RunStatusProvisioning && run.CreatedAt.Valid {
		metrics.RunQueueDuration.Observe(time.Since(run.CreatedAt.Time).Seconds())
	}
	if !IsTerminal(p.To) {
		return
	}
	failureClass := p.FailureClass
	if failureClass == "" {
		failureClass = "none"
	}
	metrics.RunTerminal.WithLabelValues(string(p.To), failureClass).Inc()
	if run.CreatedAt.Valid && run.FinishedAt.Valid {
		metrics.RunDuration.WithLabelValues(string(p.To)).
			Observe(run.FinishedAt.Time.Sub(run.CreatedAt.Time).Seconds())
	}
}

// record writes the three things every state change owes: the append-only status
// history (RUN-002), the audit event (NFR-001) and the outbox event (ADR-008).
// q must be the caller's transaction handle - that is the whole point.
func (s *Service) record(
	ctx context.Context, q *gen.Queries, run gen.Run,
	from *gen.RunStatus, attemptID pgtype.UUID, reason string, actor pgtype.UUID, action string,
) error {
	reasonPtr := &reason
	if reason == "" {
		reasonPtr = nil
	}
	if err := q.InsertRunStatusTransition(ctx, gen.InsertRunStatusTransitionParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID, RunAttemptID: attemptID,
		FromStatus: from, ToStatus: run.Status, Reason: reasonPtr,
	}); err != nil {
		return err
	}

	meta := map[string]any{"to_status": string(run.Status)}
	if from != nil {
		meta["from_status"] = string(*from)
	}
	if reason != "" {
		meta["reason"] = reason
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: action,
		ResourceType: audit.ResourceRun, ResourceID: run.ID, Metadata: meta,
	}); err != nil {
		return err
	}

	// One event type per state entered, so a consumer routes on event_type rather
	// than by parsing a payload. The ADR-008 workflow names (RunProvisioned,
	// RunExecutionCompleted, ...) are a coarser view of the same stream; deriving
	// them, and moving the schemas to contracts/events/, waits for a consumer that
	// needs them - internal/outbox delivers what is here today. Payload carries
	// identifiers and outcome only (iron rule 11): no prompt, no output, no key.
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = q.InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType:    "run." + string(run.Status),
		EventVersion: 1,
		// The platform run_id is the correlation for everything in this workflow;
		// a provider id never substitutes for it (ADR-008, iron rule 10).
		CorrelationID: run.ID,
		CausationID:   attemptID,
		WorkspaceID:   run.WorkspaceID,
		AggregateType: audit.ResourceRun,
		AggregateID:   run.ID,
		Payload:       payload,
	})
	return err
}

// Get returns one run, workspace scoped.
func (s *Service) Get(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.Run, error) {
	run, err := s.queries().GetRun(ctx, gen.GetRunParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Run{}, ErrNotFound
	}
	return run, err
}

// List returns the workspace's Run history, newest first (WS-004).
//
// The page size is clamped rather than trusted: it comes from a query string and
// it decides how much work one request does.
func (s *Service) List(
	ctx context.Context, workspaceID pgtype.UUID, limit, offset int32,
) ([]gen.ListWorkspaceRunsRow, error) {
	if limit <= 0 || limit > maxRunPageSize {
		limit = defaultRunPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return s.queries().ListWorkspaceRuns(ctx, gen.ListWorkspaceRunsParams{
		WorkspaceID: workspaceID, PageSize: limit, PageOffset: offset,
	})
}

const (
	defaultRunPageSize = 50
	maxRunPageSize     = 200
)

// Artifacts returns one run's output manifest (WS-004's read side, and what makes
// DeleteArtifact reachable — a delete for something nobody can list is a delete
// nobody can press).
func (s *Service) Artifacts(
	ctx context.Context, workspaceID, runID pgtype.UUID,
) ([]gen.Artifact, error) {
	// The run itself is read first so a caller learns nothing about another
	// workspace's run id: an unknown run and somebody else's run answer alike.
	if _, err := s.Get(ctx, workspaceID, runID); err != nil {
		return nil, err
	}
	return s.queries().ListRunArtifacts(ctx, gen.ListRunArtifactsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// DeleteArtifact removes one Run output the owner asked to be gone
// (02:WS-002 3, 02:SEC-006 1). Until now the only way to reach a run artifact was
// to delete the whole account.
//
// Idempotent, exactly as the download package's delete is: an id that is not
// there, is already deleted, or belongs to somebody else all reach the same
// answer. The caller's intent — this file must not exist — holds in every one of
// those cases.
//
// The row is soft-deleted and the object removed after the commit, the order
// DeleteDownload uses and for the same reason: object storage has no rollback, so
// removing bytes a live row still points at is the failure that cannot be
// repaired, while an orphan object is swept by retention.
func (s *Service) DeleteArtifact(
	ctx context.Context, ws gen.Workspace, runID, artifactID pgtype.UUID,
) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := gen.New(tx)

	row, err := q.SoftDeleteRunArtifact(ctx, gen.SoftDeleteRunArtifactParams{
		ArtifactID: artifactID, RunID: runID, WorkspaceID: ws.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := audit.Log(ctx, q, audit.Event{
		Actor: ws.OwnerUserID, Workspace: ws.ID,
		Action: audit.ActionArtifactDelete, ResourceType: audit.ResourceArtifact,
		ResourceID: row.ID,
		Metadata:   map[string]any{"run_id": uuidString(runID)},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if row.PurgedAt.Valid || s.Store == nil {
		return nil // the bytes are already gone, or there is no store to ask
	}
	// A run's outputs are one archive per attempt, so several rows can name one
	// object key. Removing it while another row still points at it would delete a
	// file the owner did not ask about; the count is asked after the soft delete,
	// so it cannot count the row just removed.
	shared, err := gen.New(s.Pool).CountArtifactsSharingObject(ctx, row.ObjectKey)
	if err != nil || shared > 0 {
		if err != nil {
			slog.Warn("could not check whether a run artifact object is shared; leaving the bytes",
				"object_key", row.ObjectKey, "error", err)
		}
		return nil
	}
	// Best effort: the row is gone either way, Remove is idempotent, and the
	// retention sweep reaches the same key.
	if err := s.Store.Remove(ctx, row.ObjectKey); err != nil {
		slog.Warn("run artifact object not removed; the retention sweep will retry",
			"object_key", row.ObjectKey, "error", err)
	}
	return nil
}

// Linkage returns the skill the run's version belongs to and the editable test
// case its snapshot was frozen from — the two ids the runs row does not carry and
// the read surface has to serve (RUN-002).
//
// Neither is an authorization. Whether the run may be repeated is preflight's
// answer, and whether its inputs still exist is a separate question again.
func (s *Service) Linkage(ctx context.Context, workspaceID, runID pgtype.UUID) (gen.GetRunLinkageRow, error) {
	row, err := s.queries().GetRunLinkage(ctx, gen.GetRunLinkageParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.GetRunLinkageRow{}, ErrNotFound
	}
	return row, err
}

// History returns the status transitions of one run, oldest first (RUN-002: the
// user can see the current status and when it last changed).
func (s *Service) History(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunStatusTransition, error) {
	return s.queries().ListRunStatusTransitions(ctx, gen.ListRunStatusTransitionsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// Attempts returns the run's attempts with their provider ids (RUN-003).
func (s *Service) Attempts(ctx context.Context, workspaceID, runID pgtype.UUID) ([]gen.RunAttempt, error) {
	return s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: runID, WorkspaceID: workspaceID,
	})
}

// RequestCancel records the user's intent to cancel (RUN-004). It does not change
// the run's status: the workload is still running until something stops it, and
// reporting `cancelled` before that would be a lie about a live sandbox.
//
// Propagating the intent - signalling the provider, transitioning to cancelled
// once the workload is actually down, and the wall-clock timeout that shares this
// machinery - is RUN-006.
func (s *Service) RequestCancel(ctx context.Context, workspaceID, runID, actor pgtype.UUID) (gen.Run, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	run, err := q.RequestRunCancel(ctx, gen.RequestRunCancelParams{ID: runID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		// Either it does not exist here, or it is already finished. Distinguish,
		// so a user who cancelled a second too late is told which.
		if _, err := s.Get(ctx, workspaceID, runID); err != nil {
			return gen.Run{}, err
		}
		return gen.Run{}, ErrRunFinished
	}
	if err != nil {
		return gen.Run{}, err
	}

	if err := audit.Log(ctx, q, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: audit.ActionRunCancelAsk,
		ResourceType: audit.ResourceRun, ResourceID: run.ID,
		Metadata: map[string]any{"status_at_request": string(run.Status)},
	}); err != nil {
		return gen.Run{}, err
	}
	return run, tx.Commit(ctx)
}

func uuidString(u pgtype.UUID) string {
	v, _ := u.Value()
	s, _ := v.(string)
	return s
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }
