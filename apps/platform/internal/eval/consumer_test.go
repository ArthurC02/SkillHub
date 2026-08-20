package eval

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

// recorder is a consumer wired to counters instead of a database and a queue.
// `found` is whether the run already has a standing evaluation.
type recorder struct {
	found    bool
	lookups  int
	inserted []JobArgs
	opts     []*river.InsertOpts
	err      error
}

func (r *recorder) consumer() *RunEventConsumer {
	return &RunEventConsumer{
		Current: func(context.Context, gen.GetCurrentEvaluationParams) (gen.Evaluation, error) {
			r.lookups++
			if r.err != nil {
				return gen.Evaluation{}, r.err
			}
			if !r.found {
				return gen.Evaluation{}, pgx.ErrNoRows
			}
			return gen.Evaluation{}, nil
		},
		Insert: func(_ context.Context, args river.JobArgs, opts *river.InsertOpts,
		) (*rivertype.JobInsertResult, error) {
			r.inserted = append(r.inserted, args.(JobArgs))
			r.opts = append(r.opts, opts)
			// The first insert is what produces the evaluation row the guard reads.
			r.found = true
			return nil, nil
		},
	}
}

func runEvent(eventType string) gen.OutboxEvent {
	var runID, workspaceID pgtype.UUID
	_ = runID.Scan("11111111-1111-4111-8111-111111111111")
	_ = workspaceID.Scan("22222222-2222-4222-8222-222222222222")
	return gen.OutboxEvent{
		EventType: eventType, AggregateType: "run",
		AggregateID: runID, WorkspaceID: workspaceID,
	}
}

// The at-least-once contract in one test: the same event twice must not cost a
// second judge call.
func TestRunEventConsumerEnqueuesOncePerRun(t *testing.T) {
	for _, eventType := range []string{"run.succeeded", "run.failed"} {
		r := &recorder{}
		c := r.consumer()
		for range 2 {
			if err := c.Deliver(context.Background(), runEvent(eventType)); err != nil {
				t.Fatalf("%s: deliver: %v", eventType, err)
			}
		}
		if len(r.inserted) != 1 {
			t.Fatalf("%s redelivered enqueued %d evaluations, want 1", eventType, len(r.inserted))
		}
		if r.inserted[0].RunID != "11111111-1111-4111-8111-111111111111" ||
			r.inserted[0].WorkspaceID != "22222222-2222-4222-8222-222222222222" {
			t.Errorf("%s enqueued %+v, want the event's own identifiers", eventType, r.inserted[0])
		}
		// The other redelivery window: the guard cannot see a row that the first
		// job has not written yet, so the unique key has to cover it.
		if r.opts[0] == nil || !r.opts[0].UniqueOpts.ByArgs {
			t.Errorf("%s enqueued without the per-run unique key", eventType)
		}
	}
}

// Everything else on the stream belongs to somebody else, including the run
// states that are deliberately not evaluated.
func TestRunEventConsumerIgnoresOtherEvents(t *testing.T) {
	for _, eventType := range []string{
		"run.queued", "run.running", "run.cancelled", "run.timed_out",
		"run.cleanup_cleaned", "skill_version.created",
	} {
		r := &recorder{}
		if err := r.consumer().Deliver(context.Background(), runEvent(eventType)); err != nil {
			t.Fatalf("%s: deliver: %v", eventType, err)
		}
		if r.lookups != 0 || len(r.inserted) != 0 {
			t.Errorf("%s caused %d lookups and %d inserts, want none", eventType, r.lookups, len(r.inserted))
		}
	}
}

// A lookup that fails says nothing about whether the run was evaluated. Guessing
// "not yet" would double-charge; the event is left for the next delivery.
func TestRunEventConsumerRefusesToGuessWhenTheLookupFails(t *testing.T) {
	r := &recorder{err: errors.New("connection refused")}
	err := r.consumer().Deliver(context.Background(), runEvent("run.succeeded"))
	if err == nil {
		t.Fatal("a failed lookup was reported as a successful delivery")
	}
	if len(r.inserted) != 0 {
		t.Errorf("enqueued %d evaluations despite an unreadable lookup", len(r.inserted))
	}
}
