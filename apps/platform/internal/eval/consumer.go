package eval

// The event-driven entry point (DDD-005). A finished run's evaluation used to be
// enqueued by internal/run inside the terminal transition, which made the run
// context depend on this one to know what happens next. Now the run context only
// announces `run.succeeded` / `run.failed`, and evaluation is the side that
// decides it cares — the direction ADR-032 appendix A requires, and the "one
// trigger source" rule of contracts/events/domain-events.md §4.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// Terminal run events this context reacts to. `cancelled` and `timed_out` are
// absent on purpose: such a run was stopped before it could produce the thing
// the criteria are about, and paying a judge to say so tells nobody anything.
const (
	eventRunSucceeded = "run.succeeded"
	eventRunFailed    = "run.failed"
)

// RunEventConsumer enqueues one evaluation per finished run, driven by the
// outbox. Its Deliver method has outbox.Worker's Deliver shape.
//
// The two collaborators are function fields rather than a *Service and a
// *river.Client: the composition root cannot build the queue client until its
// workers exist, so the queue has to be settable after construction anyway, and
// plain functions let the redelivery guard be tested without a database.
type RunEventConsumer struct {
	// Current reads the run's standing evaluation; (*gen.Queries).GetCurrentEvaluation.
	Current func(context.Context, gen.GetCurrentEvaluationParams) (gen.Evaluation, error)
	// Insert enqueues a job; (*river.Client[pgx.Tx]).Insert.
	Insert func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Deliver reacts to one domain event. Anything that is not a terminal run event
// is not this context's business and is not an error.
func (c *RunEventConsumer) Deliver(ctx context.Context, event gen.OutboxEvent) error {
	if event.EventType != eventRunSucceeded && event.EventType != eventRunFailed {
		return nil
	}

	// Idempotency, which the outbox demands of every consumer: delivery is
	// at-least-once, so this event will arrive twice whenever a publisher dies
	// between handing it on and marking it published.
	//
	// InsertOpts()'s unique key alone is not enough. It covers only *live* job
	// states — deliberately, so a run stays re-evaluatable (ADR-026) — and River
	// deletes completed jobs after its retention window, so a redelivery arriving
	// after the first evaluation finished would insert a second job and buy a
	// second judge call. Widening the state set would trade that bug for a worse
	// one: it would make re-evaluation impossible for as long as the old job row
	// survives, and still expire with it.
	//
	// So the guard reads the fact rather than the job: an evaluation row is
	// permanent, and its presence is exactly what "this event has already been
	// acted on" means. The unique key still earns its place for the other window —
	// a redelivery while the first job is queued or running, before any row exists.
	// Deliberate re-evaluation is unaffected: it does not come through here.
	// (debt ledger LLM-EVAL-005)
	_, err := c.Current(ctx, gen.GetCurrentEvaluationParams{
		RunID: event.AggregateID, WorkspaceID: event.WorkspaceID,
	})
	if err == nil {
		return nil // already evaluated
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err // unknown: re-delivered rather than assumed clear
	}

	_, err = c.Insert(ctx, JobArgs{
		RunID:       uuidString(event.AggregateID),
		WorkspaceID: uuidString(event.WorkspaceID),
	}, InsertOpts())
	return err
}
