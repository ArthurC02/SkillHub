package run

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

var (
	ErrIllegalTransition = errors.New("illegal run status transition")

	ErrConflict = errors.New("run is no longer in the expected status")

	ErrNoHappyPath = errors.New("no successes-only path to succeeded")
)

type FailureClass string

const (
	failureProvider   FailureClass = "provider_error"
	failureWorkload   FailureClass = "workload_error"
	failureTimeout    FailureClass = "timeout"
	failureCancelled  FailureClass = "cancelled"
	failureNoProvider FailureClass = "capability_mismatch"
	failurePlatform   FailureClass = "platform_error"
)

func AllFailureClasses() []FailureClass {
	return []FailureClass{
		failureProvider, failureWorkload, failureTimeout,
		failureCancelled, failureNoProvider, failurePlatform,
	}
}

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

	FailureClass FailureClass

	Actor pgtype.UUID
}

func (s *Service) Transition(ctx context.Context, p TransitionParams) (gen.Run, error) {
	if !CanTransition(p.From, p.To) {
		return gen.Run{}, fmt.Errorf("%w: %s -> %s", ErrIllegalTransition, p.From, p.To)
	}
	r, err := s.commandRun(ctx, p.WorkspaceID, p.RunID, p.Actor, func(r *Run) error {
		if r.Status() != p.From {
			return ErrConflict
		}
		r.Transition(p.To, p.Reason, p.FailureClass, p.AttemptID)
		return nil
	})
	if errors.Is(err, ErrNotFound) {
		return gen.Run{}, ErrConflict
	}
	if err != nil {
		return gen.Run{}, err
	}
	observeTransition(r.Row(), p)
	return r.Row(), nil
}

func (s *Service) recordFailureEvent(ctx context.Context, tx pgx.Tx, q *gen.Queries, run gen.Run, failure FailureClass, reason string) error {
	if run.Status != gen.RunStatusFailed && run.Status != gen.RunStatusTimedOut {
		return nil
	}
	code := string(failure)
	if code == "" {
		code = "unclassified"
	}
	return trace.RecordOrchestratorEvent(ctx, tx, run.WorkspaceID, run.ID,
		attemptNumber(ctx, q, run), trace.TypeError, "error", map[string]any{

			"category": failure.category(),
			"code":     code,
			"message":  reason,

			"retryable": failure.retryable(),
		})
}

func (c FailureClass) category() string {
	switch c {
	case failureProvider, failureNoProvider:
		return "provision"
	default:
		return "execution"
	}
}

func (c FailureClass) retryable() bool {
	return c == failureProvider
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
	failureClass := string(p.FailureClass)
	if failureClass == "" {
		failureClass = "none"
	}
	metrics.RunTerminal.WithLabelValues(string(p.To), failureClass).Inc()
	if run.CreatedAt.Valid && run.FinishedAt.Valid {
		metrics.RunDuration.WithLabelValues(string(p.To)).
			Observe(run.FinishedAt.Time.Sub(run.CreatedAt.Time).Seconds())
	}
}

func (s *Service) recordTransition(
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
	return audit.Log(ctx, tx, audit.Event{
		Actor: actor, Workspace: run.WorkspaceID, Action: action,
		ResourceType: audit.ResourceRun, ResourceID: run.ID, Metadata: meta,
	})
}
