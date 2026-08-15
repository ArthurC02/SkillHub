package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
)

var (
	// ErrNotFound covers "no such run" and "not yours" alike: existence is itself
	// private (WS-006), so callers turn this into a 404 either way.
	ErrNotFound = errors.New("run not found")
	// ErrIllegalTransition is a bug in the caller — the pair is not in the state
	// machine at all. It never depends on what is in the database.
	ErrIllegalTransition = errors.New("illegal run status transition")
	// ErrConflict is a legal transition applied to a run that had already moved.
	// Expected under at-least-once queue delivery, not an error to alarm about.
	ErrConflict = errors.New("run is no longer in the expected status")
	// ErrRunFinished is a cancel arriving after the run reached a terminal state.
	ErrRunFinished = errors.New("run has already finished")
)

// providerUnassigned is the provider of a run that has not been scheduled yet.
// Provider selection and capability matching are RUN-005; runs.provider is NOT
// NULL, and naming the gap is better than defaulting to whichever provider
// happens to exist first.
const providerUnassigned = "unassigned"

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
}

func (s *Service) queries() *gen.Queries { return gen.New(s.Pool) }

// resourceLimits mirrors ResourceLimits in contracts/openapi/sandbox-provider.yaml.
// Values are PDM-005 §5.2. Changing one means changing both — the contract is the
// spec, this is the snapshot written into policy_snapshot so TEST-005 can show the
// user what their run will be held to, and so a past run stays explainable after
// the defaults change.
type resourceLimits struct {
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

func defaultResourceLimits() resourceLimits {
	l := resourceLimits{
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
	// Enforced by the worker counting gateway-reported input_tokens, not by the
	// Virtual Key's max_budget — prompt caching decoupled spend from token count
	// by 7-8x (PDM-005 §5.2a). Counting lands with the provider in RUN-005/SBX-006.
	l.TokenBudget.MaxInputTokens = 300_000
	l.TokenBudget.MaxOutputTokens = 60_000
	return l
}

// defaultPolicySnapshot is what the run is held to, frozen at creation (ADR-003).
// Egress is default-deny with the three destinations of PDM-005 §5.2; the concrete
// URLs are filled in when a provider is selected (RUN-005), so only the shape and
// the deny default are asserted here.
func defaultPolicySnapshot() ([]byte, error) {
	return json.Marshal(map[string]any{
		"resource_limits": defaultResourceLimits(),
		"egress": map[string]any{
			"mode":  "default_deny",
			"allow": []any{},
		},
	})
}

// CreateParams is one run request. The workspace comes from the session, never
// from the request body (iron rule 3).
type CreateParams struct {
	WorkspaceID pgtype.UUID
	Actor       pgtype.UUID
	SkillID     pgtype.UUID
	VersionID   pgtype.UUID
	TestCaseID  pgtype.UUID
}

// Create records a queued run and enqueues its execution job, in one transaction.
//
// The test case is snapshotted here rather than referenced: a run points at frozen
// content (iron rule 4), so later edits to the test case cannot rewrite what a past
// run was asked to do.
func (s *Service) Create(ctx context.Context, p CreateParams) (gen.Run, error) {
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

	policy, err := defaultPolicySnapshot()
	if err != nil {
		return gen.Run{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.Run{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	snapshot, err := q.CreateTestCaseSnapshotFromTestCase(ctx, gen.CreateTestCaseSnapshotFromTestCaseParams{
		ID: p.TestCaseID, WorkspaceID: p.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
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
		if _, err := s.Queue.InsertTx(ctx, tx, JobArgs{
			RunID:       uuidString(run.ID),
			WorkspaceID: uuidString(run.WorkspaceID),
		}, nil); err != nil {
			return gen.Run{}, err
		}
	}
	return run, tx.Commit(ctx)
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
	run, err := q.TransitionRun(ctx, gen.TransitionRunParams{
		RunID: p.RunID, WorkspaceID: p.WorkspaceID,
		FromStatus: p.From, ToStatus: p.To, Reason: reason,
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
	return run, tx.Commit(ctx)
}

// record writes the three things every state change owes: the append-only status
// history (RUN-002), the audit event (NFR-001) and the outbox event (ADR-008).
// q must be the caller's transaction handle — that is the whole point.
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
	// RunExecutionCompleted, ...) are a coarser view of the same stream and will
	// be derived by the publisher in RUN-005, which is also when the schemas move
	// to contracts/events/. Payload carries identifiers and outcome only
	// (iron rule 11): no prompt, no output, no key.
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
// Propagating the intent — signalling the provider, transitioning to cancelled
// once the workload is actually down, and the wall-clock timeout that shares this
// machinery — is RUN-006.
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
