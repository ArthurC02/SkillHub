package outbox

// Fan-out for the drained backlog (DDD review P2).
//
// Worker.Deliver is one function, which was fine while one context listened. It
// is not fine as a wiring contract: a composition root that forgets a consumer
// still delivers "successfully", and the event is marked published having been
// acted on by nobody. That is how `run.succeeded` gets consumed without an
// evaluation ever being enqueued, with nothing red anywhere.
//
// So routing becomes data rather than a closure: every event type in EventTypes
// is either handled by named consumers or explicitly declared as uninteresting
// to this process, and Validate refuses a wiring that leaves one unaccounted
// for. Silence is no longer a valid answer — a type nobody claimed is a wiring
// bug, and it fails at startup rather than at 3am.
//
// This package still knows nothing about who cares about what (ADR-032 §1): the
// names and the handlers come from the composition root.

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// Handler is one consumer's reaction to one event; the shape Worker.Deliver has.
type Handler func(context.Context, Event) error

type namedHandler struct {
	name    string
	deliver Handler
}

// Dispatcher routes one event to every consumer registered for its type.
//
// Delivery stays at-least-once and is at-least-once *per consumer*: there is no
// per-consumer offset, so one consumer failing means the whole event is
// re-delivered and the consumers that already succeeded see it again. Every
// handler must therefore be idempotent on event_id (catalogue §4 rule 6) — the
// same obligation Worker.Deliver always carried, now owed by each of them
// independently.
//
// Not safe for concurrent registration: build it at the composition root, call
// Validate, then hand Deliver to the Worker. After that it is read-only, and the
// publisher's single locked pass is its only caller.
type Dispatcher struct {
	handlers map[string][]namedHandler
	ignored  map[string]string // event type -> why nobody in this process listens
	// errs collects registration mistakes so the call site stays a list of
	// declarations instead of a list of error checks. Validate is where they
	// surface, and Validate is the one call the composition root cannot skip.
	errs []error
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: map[string][]namedHandler{}, ignored: map[string]string{}}
}

// On registers a named consumer for one or more event types. Several consumers
// may claim the same type; all of them run, in registration order.
//
// The name is for the failure message: "delivery failed" without saying whose
// delivery failed is a log line that costs an hour of grepping.
func (d *Dispatcher) On(name string, deliver Handler, eventTypes ...string) *Dispatcher {
	switch {
	case deliver == nil:
		d.errs = append(d.errs, fmt.Errorf("consumer %q registered with a nil handler", name))
		return d
	case len(eventTypes) == 0:
		d.errs = append(d.errs, fmt.Errorf("consumer %q registered for no event types", name))
	}
	for _, t := range eventTypes {
		d.handlers[t] = append(d.handlers[t], namedHandler{name: name, deliver: deliver})
	}
	return d
}

// Ignore declares that no consumer in this process cares about these event
// types, and says why. It is what keeps the closed set closed: a type that is
// neither handled nor ignored is a gap, not a default.
func (d *Dispatcher) Ignore(reason string, eventTypes ...string) *Dispatcher {
	if reason == "" {
		d.errs = append(d.errs, errors.New("ignored event types need a reason"))
	}
	for _, t := range eventTypes {
		d.ignored[t] = reason
	}
	return d
}

// Validate reports whether this wiring accounts for the whole vocabulary. Call
// it at the composition root before the worker starts: a missing consumer should
// stop the process at boot, not be discovered a week later as a pile of finished
// runs that never got a verdict.
func (d *Dispatcher) Validate() error {
	errs := slices.Clone(d.errs)
	for _, t := range EventTypes {
		_, handled := d.handlers[t]
		reason, ignored := d.ignored[t]
		switch {
		case handled && ignored:
			errs = append(errs, fmt.Errorf("event type %q is both handled by %v and ignored as %q", t, d.consumerNames(t), reason))
		case !handled && !ignored:
			errs = append(errs, fmt.Errorf("event type %q has no consumer and is not explicitly ignored", t))
		}
	}
	// A registration outside the catalogue is a typo, and a typo here fails in the
	// worst way available: the consumer is wired, it simply never hears anything.
	for t := range d.handlers {
		if !slices.Contains(EventTypes, t) {
			errs = append(errs, fmt.Errorf("consumer registered for %q, which is not in outbox.EventTypes", t))
		}
	}
	for t := range d.ignored {
		if !slices.Contains(EventTypes, t) {
			errs = append(errs, fmt.Errorf("ignored event type %q is not in outbox.EventTypes", t))
		}
	}
	return errors.Join(errs...)
}

// Deliver has Worker.Deliver's shape: assign it to that field.
//
// An unrouted event is an error rather than a no-op, so a wiring that skipped
// Validate still fails loudly instead of marking the event published. The event
// then takes the normal failure path — retried, counted, eventually isolated —
// which is the visible version of what used to be a silent success.
func (d *Dispatcher) Deliver(ctx context.Context, event Event) error {
	handlers, handled := d.handlers[event.EventType]
	if !handled {
		if _, ignored := d.ignored[event.EventType]; ignored {
			return nil
		}
		return fmt.Errorf("no consumer registered for event type %q", event.EventType)
	}
	// All of them, or the event has not been delivered. Stopping at the first
	// failure is deliberate: the retry replays the whole event anyway, so running
	// the rest would only buy duplicate work ahead of the same redelivery.
	for _, h := range handlers {
		if err := h.deliver(ctx, event); err != nil {
			return fmt.Errorf("consumer %q: %w", h.name, err)
		}
	}
	return nil
}

func (d *Dispatcher) consumerNames(eventType string) []string {
	names := make([]string, 0, len(d.handlers[eventType]))
	for _, h := range d.handlers[eventType] {
		names = append(names, h.name)
	}
	return names
}
