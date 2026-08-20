package outbox

// What these prove is a wiring property, not a database one: an event type that
// nobody claimed must not be reported as delivered. No Postgres — the bug this
// guards against is a composition root that forgot a consumer, and that mistake
// is fully visible in the routing table.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/platform/db/gen"
)

func noopHandler(context.Context, gen.OutboxEvent) error { return nil }

func failingHandler(err error) Handler {
	return func(context.Context, gen.OutboxEvent) error { return err }
}

// fullyWired is the shape a healthy composition root produces: every catalogue
// entry either routed or explicitly disclaimed.
func fullyWired(t *testing.T) *Dispatcher {
	t.Helper()
	d := NewDispatcher().
		On("evaluation", noopHandler, RunSucceeded, RunFailed).
		Ignore("no consumer in this process", RunQueued, RunProvisioning, RunPreparing,
			RunRunning, RunEvaluating, RunCancelled, RunTimedOut,
			RunCleanupCleaned, RunCleanupFailed)
	if err := d.Validate(); err != nil {
		t.Fatalf("a fully wired dispatcher must validate: %v", err)
	}
	return d
}

func TestValidateRefusesAnUnaccountedEventType(t *testing.T) {
	// The empty dispatcher: every single catalogue entry is a gap, and each one
	// must be named. "Some events are unrouted" would leave the operator guessing
	// which.
	err := NewDispatcher().Validate()
	if err == nil {
		t.Fatal("an empty dispatcher validated, want an error naming every unrouted event type")
	}
	for _, eventType := range EventTypes {
		if !strings.Contains(err.Error(), eventType) {
			t.Errorf("%q is unrouted but not named in the validation error: %v", eventType, err)
		}
	}
	fullyWired(t) // and the complete wiring passes
}

// The report's named case (DDD review P2). A worker built without the evaluation
// consumer must not sail through: the whole point is that `run.succeeded` stops
// being quietly marked published while no evaluation is ever enqueued.
func TestMissingEvaluationWiringIsRefusedAndRunSucceededIsNotConsumed(t *testing.T) {
	d := NewDispatcher().
		Ignore("no consumer in this process", RunQueued, RunProvisioning, RunPreparing,
			RunRunning, RunEvaluating, RunCancelled, RunTimedOut,
			RunCleanupCleaned, RunCleanupFailed)

	err := d.Validate()
	if err == nil {
		t.Fatal("a dispatcher with no evaluation consumer validated")
	}
	if !strings.Contains(err.Error(), RunSucceeded) || !strings.Contains(err.Error(), RunFailed) {
		t.Errorf("validation error does not name the terminal events left unrouted: %v", err)
	}

	// And if the process started anyway, delivery still refuses. This is the half
	// that matters at runtime: an error here keeps the event unpublished and pushes
	// it onto the retry-and-dead-letter path instead of dropping it silently.
	if err := d.Deliver(context.Background(), gen.OutboxEvent{EventType: RunSucceeded}); err == nil {
		t.Fatal("run.succeeded was delivered successfully with no consumer registered")
	}
}

func TestDeliverFansOutToEveryConsumer(t *testing.T) {
	var first, second bool
	d := fullyWired(t).
		On("first", func(context.Context, gen.OutboxEvent) error { first = true; return nil }, RunSucceeded).
		On("second", func(context.Context, gen.OutboxEvent) error { second = true; return nil }, RunSucceeded)

	if err := d.Deliver(context.Background(), gen.OutboxEvent{EventType: RunSucceeded}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !first || !second {
		t.Errorf("fan-out skipped a consumer: first=%v second=%v", first, second)
	}
}

// One consumer failing means the event was not delivered, full stop. Publish
// reads that error and leaves the row unpublished, so the consumers that did
// succeed will see it again — which is why each of them has to be idempotent.
func TestDeliverFailsWhenAnyConsumerFails(t *testing.T) {
	boom := errors.New("boom")
	d := fullyWired(t).
		On("healthy", noopHandler, RunFailed).
		On("broken", failingHandler(boom), RunFailed)

	err := d.Deliver(context.Background(), gen.OutboxEvent{EventType: RunFailed})
	if !errors.Is(err, boom) {
		t.Fatalf("deliver error = %v, want it to wrap the consumer failure", err)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("deliver error does not name the failing consumer: %v", err)
	}
}

func TestIgnoredEventTypesAreDelivered(t *testing.T) {
	if err := fullyWired(t).Deliver(context.Background(), gen.OutboxEvent{EventType: RunQueued}); err != nil {
		t.Fatalf("an explicitly ignored event type must be a no-op, got %v", err)
	}
}

// Registration mistakes that the type system cannot catch: a consumer wired to
// an event type that does not exist hears nothing, and looks perfectly healthy
// while doing so.
func TestValidateRejectsRegistrationMistakes(t *testing.T) {
	for name, build := range map[string]func() *Dispatcher{
		"unknown event type": func() *Dispatcher {
			return fullyWired(t).On("typo", noopHandler, "run.suceeded")
		},
		"nil handler": func() *Dispatcher {
			return fullyWired(t).On("empty", nil, RunSucceeded)
		},
		"handled and ignored": func() *Dispatcher {
			return fullyWired(t).Ignore("nobody listens", RunSucceeded)
		},
		"ignored with no reason": func() *Dispatcher {
			return fullyWired(t).Ignore("", RunQueued)
		},
	} {
		if err := build().Validate(); err == nil {
			t.Errorf("%s: validated, want an error", name)
		}
	}
}

// The removed fallback (DDD review P2 recommendation 1): no destination is a
// refusal, and logging to the console is something a developer opts into.
func TestPublishRefusesWithoutADestination(t *testing.T) {
	if _, err := (&Worker{}).delivery(); err == nil {
		t.Error("a worker with no Deliver and no LogOnlyDelivery accepted the publish path")
	}
	if _, err := (&Worker{LogOnlyDelivery: true}).delivery(); err != nil {
		t.Errorf("development log-only delivery was refused: %v", err)
	}
	if _, err := (&Worker{Deliver: noopHandler}).delivery(); err != nil {
		t.Errorf("a wired worker was refused: %v", err)
	}
}
