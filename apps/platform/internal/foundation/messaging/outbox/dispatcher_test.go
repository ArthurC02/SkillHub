package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func noopHandler(context.Context, Event) error { return nil }

func failingHandler(err error) Handler {
	return func(context.Context, Event) error { return err }
}

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

	err := NewDispatcher().Validate()
	if err == nil {
		t.Fatal("an empty dispatcher validated, want an error naming every unrouted event type")
	}
	for _, eventType := range EventTypes {
		if !strings.Contains(err.Error(), eventType) {
			t.Errorf("%q is unrouted but not named in the validation error: %v", eventType, err)
		}
	}
	fullyWired(t)
}

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

	if err := d.Deliver(context.Background(), Event{EventType: RunSucceeded}); err == nil {
		t.Fatal("run.succeeded was delivered successfully with no consumer registered")
	}
}

func TestDeliverFansOutToEveryConsumer(t *testing.T) {
	var first, second bool
	d := fullyWired(t).
		On("first", func(context.Context, Event) error { first = true; return nil }, RunSucceeded).
		On("second", func(context.Context, Event) error { second = true; return nil }, RunSucceeded)

	if err := d.Deliver(context.Background(), Event{EventType: RunSucceeded}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !first || !second {
		t.Errorf("fan-out skipped a consumer: first=%v second=%v", first, second)
	}
}

func TestDeliverFailsWhenAnyConsumerFails(t *testing.T) {
	boom := errors.New("boom")
	d := fullyWired(t).
		On("healthy", noopHandler, RunFailed).
		On("broken", failingHandler(boom), RunFailed)

	err := d.Deliver(context.Background(), Event{EventType: RunFailed})
	if !errors.Is(err, boom) {
		t.Fatalf("deliver error = %v, want it to wrap the consumer failure", err)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("deliver error does not name the failing consumer: %v", err)
	}
}

func TestIgnoredEventTypesAreDelivered(t *testing.T) {
	if err := fullyWired(t).Deliver(context.Background(), Event{EventType: RunQueued}); err != nil {
		t.Fatalf("an explicitly ignored event type must be a no-op, got %v", err)
	}
}

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
