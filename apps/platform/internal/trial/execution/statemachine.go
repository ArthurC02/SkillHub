package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

var (
	ErrIllegalTransition = errors.New("illegal run status transition")

	ErrConflict = errors.New("run is no longer in the expected status")

	ErrNoHappyPath = errors.New("no successes-only path to succeeded")
)

const (
	failureProvider   = "provider_error"
	failureWorkload   = "workload_error"
	failureTimeout    = "timeout"
	failureCancelled  = "cancelled"
	failureNoProvider = "capability_mismatch"
	failurePlatform   = "platform_error"
)

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
}

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

func CanTransition(from, to gen.RunStatus) bool {
	for _, s := range successors[from] {
		if s == to {
			return true
		}
	}
	return false
}

func IsTerminal(s gen.RunStatus) bool {
	_, ongoing := successors[s]
	return !ongoing
}

var unhappyTerminals = map[gen.RunStatus]bool{
	gen.RunStatusFailed:    true,
	gen.RunStatusCancelled: true,
	gen.RunStatusTimedOut:  true,
}

func NextOnSuccess(from gen.RunStatus) (gen.RunStatus, bool) {
	for _, s := range successors[from] {
		if !unhappyTerminals[s] {
			return s, true
		}
	}
	return "", false
}

func HappyPath(from gen.RunStatus) ([]gen.RunStatus, error) {
	var path []gen.RunStatus
	for cur := from; cur != gen.RunStatusSucceeded; {
		// Bounds the walk by the number of statuses, so a table with a cycle
		// fails here instead of looping forever.
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

type TransitionParams struct {
	WorkspaceID pgtype.UUID
	RunID       pgtype.UUID

	AttemptID pgtype.UUID
	From, To  gen.RunStatus
	Reason    string

	FailureClass string

	Actor pgtype.UUID
}

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

	if err := s.recordFailureEvent(ctx, tx, q, run, p); err != nil {
		return gen.Run{}, err
	}

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

			"category": failureCategory(p.FailureClass),
			"code":     code,
			"message":  p.Reason,

			"retryable": p.FailureClass == failureProvider,
		})
}

func failureCategory(failureClass string) string {
	switch failureClass {
	case failureProvider, failureNoProvider:
		return "provision"
	default:
		return "execution"
	}
}

func attemptNumber(ctx context.Context, q *gen.Queries, run gen.Run) int {
	attempts, err := q.ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil || len(attempts) == 0 {
		return 1
	}
	return int(attempts[len(attempts)-1].AttemptNumber)
}

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

		CorrelationID: run.ID,

		CausationID:   attemptID,
		WorkspaceID:   run.WorkspaceID,
		AggregateType: outbox.AggregateRun,
		AggregateID:   run.ID,
		Payload:       payload,
	})
}
