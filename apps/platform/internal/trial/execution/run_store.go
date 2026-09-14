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
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
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
		row, err := q.RequestRunCancel(ctx, gen.RequestRunCancelParams{ID: r.row.ID, WorkspaceID: r.row.WorkspaceID})
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
			ID: r.row.ID, WorkspaceID: r.row.WorkspaceID, Provider: r.row.Provider, RuntimeSnapshot: r.row.RuntimeSnapshot,
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
		attempt, err := q.CreateRunAttempt(ctx, gen.CreateRunAttemptParams{
			ID: r.row.ID, WorkspaceID: r.row.WorkspaceID, Provider: event.Provider,
		})
		if err != nil {
			return err
		}
		r.attempts[len(r.attempts)-1] = attempt
		event.AttemptID, event.AttemptNumber = attempt.ID, attempt.AttemptNumber
		r.events[i] = event
		return publishRunEvent(ctx, tx, r, event, attempt.ID)
	case AttemptDispatched:
		a := r.attempt(event.AttemptID)
		updated, err := q.SetAttemptProviderRunID(ctx, gen.SetAttemptProviderRunIDParams{
			ID: a.ID, WorkspaceID: a.WorkspaceID, ProviderRunID: a.ProviderRunID,
		})
		if err != nil {
			return err
		}
		*a = updated
		return publishRunEvent(ctx, tx, r, event, a.ID)
	case AttemptFinished:
		a := r.attempt(event.AttemptID)
		updated, err := q.FinishRunAttempt(ctx, gen.FinishRunAttemptParams{
			ID: a.ID, WorkspaceID: a.WorkspaceID, ErrorClass: a.ErrorClass, ErrorMessage: a.ErrorMessage,
		})
		if err != nil {
			return err
		}
		*a = updated
		return publishRunEvent(ctx, tx, r, event, a.ID)
	case ObjectGrantsRecorded:
		a := r.attempt(event.AttemptID)
		updated, err := q.SetRunAttemptObjectGrantsExpiry(ctx, gen.SetRunAttemptObjectGrantsExpiryParams{
			ExpiresAt: a.ObjectGrantsExpireAt, ID: a.ID, WorkspaceID: a.WorkspaceID,
		})
		if err != nil {
			return err
		}
		if updated != 1 {
			return errors.New("run: the attempt whose object grant expiry was being recorded is not in this workspace")
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
	return s.record(ctx, q, tx, row, nil, pgtype.UUID{}, event.Reason, actor, audit.ActionRunCreate)
}

func (s *Service) writeTransition(ctx context.Context, tx pgx.Tx, q *gen.Queries, r *Run, event StatusChanged, actor pgtype.UUID) error {
	from := gen.RunStatus(event.FromStatus)
	row, err := q.TransitionRun(ctx, gen.TransitionRunParams{
		RunID: r.row.ID, WorkspaceID: r.row.WorkspaceID,
		FromStatus: from, ToStatus: gen.RunStatus(event.ToStatus),
		Reason: nonEmpty(event.Reason), FailureClass: nonEmpty(string(event.failure)),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	r.row = row
	if err := s.record(ctx, q, tx, row, &from, event.attemptID, event.Reason, actor, audit.ActionRunTransition); err != nil {
		return err
	}
	if err := s.recordFailureEvent(ctx, tx, q, row, event.failure, event.Reason); err != nil {
		return err
	}
	if !IsTerminal(row.Status) {
		return nil
	}
	if _, err := q.CloseUnissuedRunAttemptGrants(ctx, gen.CloseUnissuedRunAttemptGrantsParams{
		RunID: row.ID, WorkspaceID: row.WorkspaceID,
	}); err != nil {
		return err
	}
	if s.Queue == nil {
		return nil
	}
	_, err = s.Queue.InsertTx(ctx, tx, CleanupArgs{
		RunID: pgconv.UUIDString(row.ID), WorkspaceID: pgconv.UUIDString(row.WorkspaceID),
	}, cleanupInsertOpts())
	return err
}

func publishRunEvent(ctx context.Context, tx pgx.Tx, r *Run, event Event, causation pgtype.UUID) error {
	return outbox.Insert(ctx, tx, outbox.NewEvent{
		EventType: event.eventType(), EventVersion: outbox.EventVersion1,
		CorrelationID: r.row.ID, CausationID: causation, WorkspaceID: r.row.WorkspaceID,
		AggregateType: outbox.AggregateRun, AggregateID: r.row.ID, Payload: event,
	})
}
