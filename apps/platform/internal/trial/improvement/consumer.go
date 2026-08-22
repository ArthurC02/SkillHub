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

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// Terminal run events this context reacts to, named from the catalogue's own
// constants rather than re-spelled here: a consumer with its own copy of a
// producer's string is a routing bug waiting for a rename, and it would go
// unnoticed because "no event matched" looks exactly like "nothing happened".
//
// outbox.RunCancelled and outbox.RunTimedOut are absent on purpose: such a run
// was stopped before it could produce the thing the criteria are about, and
// paying a judge to say so tells nobody anything.

// RunEventConsumer enqueues one evaluation per finished run, driven by the
// outbox. Its Deliver method has outbox.Worker's Deliver shape.
//
// The two collaborators are function fields rather than a *Service and a
// *river.Client: the composition root cannot build the queue client until its
// workers exist, so the queue has to be settable after construction anyway, and
// plain functions let the redelivery guard be tested without a database.
type RunEventConsumer struct {
	// HasCurrentEvaluation is Eval's owner read for the redelivery guard.
	HasCurrentEvaluation func(context.Context, pgtype.UUID, pgtype.UUID) (bool, error)
	// Insert enqueues a job; (*river.Client[pgx.Tx]).Insert.
	Insert func(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Deliver reacts to one domain event. Anything that is not a terminal run event
// is not this context's business and is not an error.
func (c *RunEventConsumer) Deliver(ctx context.Context, event outbox.Event) error {
	if event.EventType != outbox.RunSucceeded && event.EventType != outbox.RunFailed {
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
	if c.HasCurrentEvaluation == nil || c.Insert == nil {
		return errors.New("evaluation event consumer is not configured")
	}
	hasCurrent, err := c.HasCurrentEvaluation(ctx, event.WorkspaceID, event.AggregateID)
	if err != nil {
		return err // unknown: re-delivered rather than assumed clear
	}
	if hasCurrent {
		return nil // already evaluated
	}

	_, err = c.Insert(ctx, JobArgs{
		RunID:       pgconv.UUIDString(event.AggregateID),
		WorkspaceID: pgconv.UUIDString(event.WorkspaceID),
	}, InsertOpts())
	return err
}
