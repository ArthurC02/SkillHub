package run

// The Run aggregate. Its invariants live here: which status may follow which,
// which statuses are terminal, and what one state change owes the rest of the
// system. Change the meaning of a transition and you are changing ADR-008 (the
// Postgres state machine is the only source of run state) and ADR-025 (execution
// outcome and task verdict are different facts) — read both first.
//
// Iron rule 5: this Postgres state machine is the single source of truth for run
// state. Nothing else — not a provider, not a LangGraph checkpoint — may decide
// where a run is. Nothing outside this file may write runs.status.
//
// The top half is pure rules: no context, no database, no clock, so the legality
// of a move can be asserted without either. The bottom half is the one write path
// that applies them.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

var (
	// ErrIllegalTransition is a bug in the caller - the pair is not in the state
	// machine at all. It never depends on what is in the database.
	ErrIllegalTransition = errors.New("illegal run status transition")
	// ErrConflict is a legal transition applied to a run that had already moved.
	// Expected under at-least-once queue delivery, not an error to alarm about.
	ErrConflict = errors.New("run is no longer in the expected status")
	// ErrNoHappyPath means the successes-only route out of a status does not arrive
	// at `succeeded`. It can only come from the transition table disagreeing with
	// itself, so it is a bug rather than a runtime condition - but it is returned
	// instead of assumed away, because the alternative is a run wedged in a
	// non-terminal state with nothing said about it.
	ErrNoHappyPath = errors.New("no successes-only path to succeeded")
)

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

// The standard lifecycle of ADR-004 / RUN-002:
//
//	queued → provisioning → preparing → running → evaluating
//	       → succeeded | failed | cancelled | timed_out
//
// `cleaning_up` is not in this machine. ADR-004 places cleanup *after* a terminal
// state so the execution outcome and the cleanup outcome are recorded separately;
// it lives in runs.cleanup_status (0004). A run that failed and whose sandbox was
// then successfully torn down is two different facts.
//
// successors is the whole legality rule, as data. Each non-terminal state may
// advance to exactly one next state, and may fail out to any of the three
// unhappy terminals from wherever it is:
//
//   - failed: at any point, from a policy rejection before dispatch to a broken
//     evaluation afterwards.
//   - cancelled: RUN-004 names all five non-terminal states as cancellable.
//   - timed_out: the wall clock (PDM-005 §5.2) covers queue wait as well as
//     execution, so a run can time out before a sandbox ever exists.
//
// succeeded is reachable only from evaluating: a run whose result was never
// judged has not succeeded, it has merely finished.
var successors = map[gen.RunStatus][]gen.RunStatus{
	gen.RunStatusQueued: {
		gen.RunStatusProvisioning,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusProvisioning: {
		gen.RunStatusPreparing,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusPreparing: {
		gen.RunStatusRunning,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusRunning: {
		gen.RunStatusEvaluating,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	gen.RunStatusEvaluating: {
		gen.RunStatusSucceeded,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	},
	// Terminal: absent from the map, so every transition out of one is illegal.
	// The 0005 trigger enforces the same thing in the database.
}

// AllStatuses is every value of the run_status enum, in lifecycle order. Exported
// so the state machine test can assert over the full cross product rather than
// over a list someone remembered to update.
//
// Adding a value to the enum in db/migrations means adding it here: sqlc generates
// the constants but no list of them, so this is the only place the full set exists
// in Go, and the test pins its size for exactly that reason.
var AllStatuses = []gen.RunStatus{
	gen.RunStatusQueued,
	gen.RunStatusProvisioning,
	gen.RunStatusPreparing,
	gen.RunStatusRunning,
	gen.RunStatusEvaluating,
	gen.RunStatusSucceeded,
	gen.RunStatusFailed,
	gen.RunStatusCancelled,
	gen.RunStatusTimedOut,
}

// CanTransition reports whether a run may move from one status to another.
// Self-transitions are illegal: re-applying a state change is caught by the
// expected-from-status guard in TransitionRun, not silently accepted here.
func CanTransition(from, to gen.RunStatus) bool {
	for _, s := range successors[from] {
		if s == to {
			return true
		}
	}
	return false
}

// IsTerminal reports whether a run in this status is finished executing. Cleanup
// may still be outstanding — that is runs.cleanup_status, not this.
func IsTerminal(s gen.RunStatus) bool {
	_, ongoing := successors[s]
	return !ongoing
}

// unhappyTerminals are the three ways a run ends badly. Every non-terminal state
// can exit to all three, which is what makes the remaining successor of a state
// its happy path - see NextOnSuccess.
var unhappyTerminals = map[gen.RunStatus]bool{
	gen.RunStatusFailed:    true,
	gen.RunStatusCancelled: true,
	gen.RunStatusTimedOut:  true,
}

// NextOnSuccess is where a run goes when nothing has gone wrong: the one successor
// in its row that is not a failure exit. ok is false for a terminal state, which
// has no next anything.
//
// The driver used to read successors[status][0] for this. That made the order
// inside each row load-bearing without saying so anywhere: reordering a row is an
// edit that changes no legality at all, and it would have sent the happy path
// somewhere else with nothing to fail. Asking which successor is not an unhappy
// terminal is the rule the table's own comment states, and it does not care what
// order the row is written in.
func NextOnSuccess(from gen.RunStatus) (gen.RunStatus, bool) {
	for _, s := range successors[from] {
		if !unhappyTerminals[s] {
			return s, true
		}
	}
	return "", false
}

// HappyPath is every status a run in `from` still has to enter to reach
// `succeeded`, in order; empty if it is already there.
//
// The whole route is computed before any of it is applied, so a machine that does
// not arrive is one error return rather than something discovered halfway through
// a sequence of committed transitions. The step cap is what makes that true even
// if the table ever grew a cycle: no acyclic route can be longer than the number
// of statuses, so exceeding it means the walk would never end, and looping there
// would mean a database write per lap.
func HappyPath(from gen.RunStatus) ([]gen.RunStatus, error) {
	var path []gen.RunStatus
	for cur := from; cur != gen.RunStatusSucceeded; {
		if len(path) >= len(AllStatuses) {
			return nil, fmt.Errorf("%w: %s still had not arrived after %d steps", ErrNoHappyPath, from, len(path))
		}
		next, ok := NextOnSuccess(cur)
		if !ok {
			return nil, fmt.Errorf("%w: %s is terminal", ErrNoHappyPath, cur)
		}
		path = append(path, next)
		cur = next
	}
	return path, nil
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
	if err := s.record(ctx, q, tx, run, &from, p.AttemptID, p.Reason, p.Actor, audit.ActionRunTransition); err != nil {
		return gen.Run{}, err
	}

	// TRACE-004: a failure the control plane decided is one the user has to be
	// able to see in the trace. Without this, a run that never reached a sandbox
	// - no provider, provisioning failed, timed out in the queue - would have an
	// empty timeline, which is exactly the case RUN-004 says must still show
	// diagnostics. Written in this transaction, so the event and the state change
	// commit together (iron rule 9).
	if err := s.recordFailureEvent(ctx, tx, q, run, p); err != nil {
		return gen.Run{}, err
	}

	// RUN-002 requires cleanup after a run ends, however it ended: a run that reached a
	// terminal state owes a cleanup, and the job that does it is enqueued in the
	// same transaction as the state change. Nobody has to remember to call it, and
	// a rolled-back transition leaves no orphan cleanup behind (iron rule 9).
	//
	// Cleanup is the only job enqueued here, because it is this context's own
	// housekeeping. What *other* contexts do about a finished run — evaluation,
	// above all — is driven by the `run.<status>` event written above, not from in
	// here (DDD-005, contracts/events/domain-events.md §4 rule 5). Same
	// transactional guarantee, one direction fewer.
	if IsTerminal(p.To) && s.Queue != nil {
		if _, err := s.Queue.InsertTx(ctx, tx, CleanupArgs{
			RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID),
		}, cleanupInsertOpts()); err != nil {
			return gen.Run{}, err
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
func (s *Service) recordFailureEvent(ctx context.Context, tx pgx.Tx, q *gen.Queries, run gen.Run, p TransitionParams) error {
	if p.To != gen.RunStatusFailed && p.To != gen.RunStatusTimedOut {
		return nil
	}
	code := p.FailureClass
	if code == "" {
		code = "unclassified"
	}
	return trace.RecordOrchestratorEvent(ctx, tx, run.WorkspaceID, run.ID,
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
// tx is the same transaction q was derived from, passed separately because
// outbox.Insert takes it by type: iron rule 9 is a property the compiler can hold
// only if the handle is one (contracts/events/domain-events.md §2).
func (s *Service) record(
	ctx context.Context, q *gen.Queries, tx pgx.Tx, run gen.Run,
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
	if err := audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: action,
		ResourceType: audit.ResourceRun, ResourceID: run.ID, Metadata: meta,
	}); err != nil {
		return err
	}

	// One event type per state entered, so a consumer routes on event_type rather
	// than by parsing a payload. The ADR-008 workflow names (RunProvisioned,
	// RunExecutionCompleted, ...) are a coarser view of the same stream; deriving
	// them waits for a consumer that needs them - internal/foundation/messaging/outbox delivers what is
	// here today. Payload carries identifiers and outcome only (iron rule 11): no
	// prompt, no output, no key.
	//
	// The type comes from a mapping that can fail rather than from "run." + status.
	// Concatenation meant a new run status shipped a new, uncatalogued event type
	// the moment the enum grew, and nothing would have said so; now the state
	// change rolls back until someone adds the event to the catalogue too.
	eventType, err := outbox.StatusEvent(string(run.Status))
	if err != nil {
		return err
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return outbox.Insert(ctx, tx, outbox.NewEvent{
		EventType:    eventType,
		EventVersion: outbox.EventVersion1,
		// The platform run_id is the correlation for everything in this workflow;
		// a provider id never substitutes for it (ADR-008, iron rule 10).
		CorrelationID: run.ID,
		// The attempt that caused the change, zero (NULL) for the genesis event: a
		// run's creation has no attempt yet and nothing precedes it, which is the
		// one exemption the catalogue's §2 allows (DDD-012).
		CausationID:   attemptID,
		WorkspaceID:   run.WorkspaceID,
		AggregateType: outbox.AggregateRun,
		AggregateID:   run.ID,
		Payload:       payload,
	})
}
