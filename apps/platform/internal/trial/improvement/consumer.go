package eval

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

const (
	UnevaluatedSweepInterval = 30 * time.Minute // one-number: unevaluatedSweepMinutes

	unevaluatedGrace    = 30 * time.Minute // one-number: unevaluatedGraceMinutes
	unevaluatedLookback = 24 * time.Hour   // one-number: unevaluatedLookbackHours
	unevaluatedPage     = 200
	unevaluatedMaxPages = 20
)

type FinishedRun struct {
	ID          pgtype.UUID
	WorkspaceID pgtype.UUID
	FinishedAt  time.Time
}

type RunEventConsumer struct {
	HasCurrentEvaluation func(ctx context.Context, workspaceID, runID pgtype.UUID) (bool, error)

	Enqueue func(context.Context, JobArgs) error

	Pool *pgxpool.Pool

	FinishedRunsAfter func(ctx context.Context, after FinishedRun, before time.Time, limit int32) ([]FinishedRun, error)
}

func (c *RunEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if event.EventType != outbox.RunSucceeded && event.EventType != outbox.RunFailed {
		return nil
	}

	if c.HasCurrentEvaluation == nil || c.Enqueue == nil {
		return errors.New("evaluation event consumer is not configured")
	}
	hasCurrent, err := c.HasCurrentEvaluation(ctx, event.WorkspaceID, event.AggregateID)
	if err != nil {
		return err
	}
	if hasCurrent {
		return nil
	}

	err = c.Enqueue(ctx, JobArgs{
		RunID:       pgconv.UUIDString(event.AggregateID),
		WorkspaceID: pgconv.UUIDString(event.WorkspaceID),
	})
	return err
}

func (c *RunEventConsumer) EnqueueUnevaluated(ctx context.Context, now time.Time) (enqueued int, err error) {
	if c.Pool == nil || c.FinishedRunsAfter == nil || c.Enqueue == nil {
		return 0, errors.New("unevaluated run sweep is not configured")
	}
	cursor := FinishedRun{FinishedAt: now.Add(-unevaluatedLookback)}
	for range unevaluatedMaxPages {
		runs, err := c.FinishedRunsAfter(ctx, cursor, now.Add(-unevaluatedGrace), unevaluatedPage)
		if err != nil || len(runs) == 0 {
			return enqueued, err
		}
		added, err := c.enqueueThoseWithoutAnEvaluation(ctx, runs)
		enqueued += added
		if err != nil || len(runs) < unevaluatedPage {
			return enqueued, err
		}
		cursor = runs[len(runs)-1]
	}
	return enqueued, nil
}

func (c *RunEventConsumer) enqueueThoseWithoutAnEvaluation(ctx context.Context, runs []FinishedRun) (int, error) {
	ids := make([]pgtype.UUID, len(runs))
	for i, run := range runs {
		ids[i] = run.ID
	}
	evaluated, err := gen.New(c.Pool).ListRunsWithAnEvaluation(ctx, ids)
	if err != nil {
		return 0, err
	}
	has := make(map[pgtype.UUID]bool, len(evaluated))
	for _, id := range evaluated {
		has[id] = true
	}
	enqueued := 0
	for _, run := range runs {
		if has[run.ID] {
			continue
		}
		if err := c.Enqueue(ctx, JobArgs{RunID: pgconv.UUIDString(run.ID), WorkspaceID: pgconv.UUIDString(run.WorkspaceID)}); err != nil {
			return enqueued, err
		}
		enqueued++
	}
	return enqueued, nil
}
