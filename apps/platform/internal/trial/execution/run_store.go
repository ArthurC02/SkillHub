package run

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func loadRun(ctx context.Context, q *gen.Queries, workspaceID, runID pgtype.UUID) (*Run, error) {
	row, err := q.LockRun(ctx, gen.LockRunParams{ID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	attempts, err := q.ListRunAttempts(ctx, gen.ListRunAttemptsParams{RunID: runID, WorkspaceID: workspaceID})
	if err != nil {
		return nil, err
	}
	return &Run{row: row, attempts: attempts}, nil
}

func (s *Service) commandRun(
	ctx context.Context, workspaceID, runID, actor pgtype.UUID, command func(*Run) error,
) (*Run, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	r, err := loadRun(ctx, s.queries().WithTx(tx), workspaceID, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := command(r); err != nil {
		return nil, err
	}
	if err := s.saveRun(ctx, tx, r, actor); err != nil {
		return nil, err
	}
	return r, tx.Commit(ctx)
}

func (s *Service) saveRun(ctx context.Context, tx pgx.Tx, r *Run, actor pgtype.UUID) error {
	if refused, ok := r.Refusal(); ok {
		return refused.err()
	}
	q := s.queries().WithTx(tx)
	for ; r.saved < len(r.events); r.saved++ {
		if err := s.writeRunEvent(ctx, tx, q, r, r.saved, actor); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) writeRunEvent(ctx context.Context, tx pgx.Tx, q *gen.Queries, r *Run, i int, actor pgtype.UUID) error {
	switch event := r.events[i].(type) {
	case StatusChanged:
		if event.FromStatus == "" {
			return s.writeCreated(ctx, tx, q, r, event, actor)
		}
		return s.writeTransition(ctx, tx, q, r, event, actor)
	case CancelRequested:
		row, err := q.RequestRunCancel(ctx, gen.RequestRunCancelParams{
			CancelRequestedAt: r.row.CancelRequestedAt, ID: r.row.ID, WorkspaceID: r.row.WorkspaceID, Status: r.row.Status,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunFinished
		}
		if err != nil {
			return err
		}
		r.row = row
		return publishRunEvent(ctx, tx, r, event, pgtype.UUID{})
	case ProviderAssigned:
		row, err := q.SetRunProvider(ctx, gen.SetRunProviderParams{
			Provider: r.row.Provider, RuntimeSnapshot: r.row.RuntimeSnapshot,
			ID: r.row.ID, WorkspaceID: r.row.WorkspaceID, Status: r.row.Status,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRunFinished
		}
		if err != nil {
			return err
		}
		r.row = row
		return publishRunEvent(ctx, tx, r, event, pgtype.UUID{})
	case AttemptStarted:
		started := r.attempts[len(r.attempts)-1]
		attempt, err := q.CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
			RunID: started.RunID, WorkspaceID: started.WorkspaceID, AttemptNumber: started.AttemptNumber,
			Provider: started.Provider, ObjectGrantsState: started.ObjectGrantsState,
			ObjectGrantsExpireAt: started.ObjectGrantsExpireAt,
		})
		if err != nil {
			return err
		}
		r.attempts[len(r.attempts)-1] = attempt
		event.AttemptID = attempt.ID
		r.events[i] = event
		return publishRunEvent(ctx, tx, r, event, attempt.ID)
	case AttemptDispatched:
		a := r.attempt(event.AttemptID)
		updated, err := q.SetAttemptProviderRunID(ctx, gen.SetAttemptProviderRunIDParams{
			ProviderRunID: a.ProviderRunID, StartedAt: a.StartedAt, ID: a.ID, WorkspaceID: a.WorkspaceID,
		})
		if err != nil {
			return err
		}
		*a = updated
		return publishRunEvent(ctx, tx, r, event, a.ID)
	case AttemptFinished:
		a := r.attempt(event.AttemptID)
		updated, err := q.FinishRunAttempt(ctx, gen.FinishRunAttemptParams{
			FinishedAt: a.FinishedAt, ErrorClass: a.ErrorClass, ErrorMessage: a.ErrorMessage,
			ObjectGrantsState: a.ObjectGrantsState, ObjectGrantsExpireAt: a.ObjectGrantsExpireAt,
			ID: a.ID, WorkspaceID: a.WorkspaceID,
		})
		if err != nil {
			return err
		}
		*a = updated
		return publishRunEvent(ctx, tx, r, event, a.ID)
	case ObjectGrantsRecorded:
		a := r.attempt(event.AttemptID)
		if err := writeObjectGrants(ctx, q, *a); err != nil {
			return err
		}
		if err := q.RememberRunArtifactUploadIntent(ctx, artifactUploadIntent(*a)); err != nil {
			return err
		}
		return publishRunEvent(ctx, tx, r, event, a.ID)
	}
	return fmt.Errorf("run event %T has nothing to write", r.events[i])
}

func (s *Service) writeCreated(ctx context.Context, tx pgx.Tx, q *gen.Queries, r *Run, event StatusChanged, actor pgtype.UUID) error {
	row, err := q.CreateRun(ctx, gen.CreateRunParams{
		WorkspaceID: r.row.WorkspaceID, SkillVersionID: r.row.SkillVersionID,
		TestCaseSnapshotID: r.row.TestCaseSnapshotID, Provider: r.row.Provider,
		RuntimeSnapshot: r.row.RuntimeSnapshot, PolicySnapshot: r.row.PolicySnapshot, Status: r.row.Status,
	})
	if err != nil {
		return err
	}
	r.row = row
	if err := s.recordTransition(ctx, q, tx, row, nil, pgtype.UUID{}, event.Reason, actor, audit.ActionRunCreate); err != nil {
		return err
	}
	return publishRunEvent(ctx, tx, r, event, pgtype.UUID{})
}

func (s *Service) writeTransition(ctx context.Context, tx pgx.Tx, q *gen.Queries, r *Run, event StatusChanged, actor pgtype.UUID) error {
	from := gen.RunStatus(event.FromStatus)
	row, err := q.TransitionRun(ctx, gen.TransitionRunParams{
		ToStatus: r.row.Status, Reason: nonEmpty(event.Reason), FailureClass: r.row.FailureClass,
		StartedAt: r.row.StartedAt, FinishedAt: r.row.FinishedAt,
		RunID: r.row.ID, WorkspaceID: r.row.WorkspaceID, FromStatus: from,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	r.row = row
	if err := s.recordTransition(ctx, q, tx, row, &from, event.attemptID, event.Reason, actor, audit.ActionRunTransition); err != nil {
		return err
	}
	if err := publishRunEvent(ctx, tx, r, event, event.attemptID); err != nil {
		return err
	}
	if err := s.recordFailureEvent(ctx, tx, q, row, event.failure, event.Reason); err != nil {
		return err
	}
	if !IsTerminal(row.Status) {
		return nil
	}
	for _, id := range event.closedGrants {
		if err := writeObjectGrants(ctx, q, *r.attempt(id)); err != nil {
			return err
		}
	}
	if s.Queue == nil {
		return nil
	}
	return s.Queue.CleanInTx(ctx, tx, RunWork{RunID: row.ID, WorkspaceID: row.WorkspaceID})
}

func writeObjectGrants(ctx context.Context, q *gen.Queries, a gen.RunAttempt) error {
	updated, err := q.SetRunAttemptObjectGrants(ctx, gen.SetRunAttemptObjectGrantsParams{
		ObjectGrantsState: a.ObjectGrantsState, ObjectGrantsExpireAt: a.ObjectGrantsExpireAt,
		ID: a.ID, WorkspaceID: a.WorkspaceID,
	})
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("run: the attempt whose object grants were being written is not in this workspace")
	}
	return nil
}

func publishRunEvent(ctx context.Context, tx pgx.Tx, r *Run, event Event, causation pgtype.UUID) error {
	return outbox.Insert(ctx, tx, outbox.NewEvent{
		EventType: event.eventType(), EventVersion: outbox.EventVersion1,
		CorrelationID: r.row.ID, CausationID: causation, WorkspaceID: r.row.WorkspaceID,
		AggregateType: outbox.AggregateRun, AggregateID: r.row.ID, Payload: event,
	})
}
