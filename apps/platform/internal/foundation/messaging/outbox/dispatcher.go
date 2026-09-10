package outbox

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

type Handler func(context.Context, Event) error

type namedHandler struct {
	name    string
	deliver Handler
}

type Dispatcher struct {
	handlers map[string][]namedHandler
	ignored  map[string]string

	errs []error
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: map[string][]namedHandler{}, ignored: map[string]string{}}
}

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

func (d *Dispatcher) Ignore(reason string, eventTypes ...string) *Dispatcher {
	if reason == "" {
		d.errs = append(d.errs, errors.New("ignored event types need a reason"))
	}
	for _, t := range eventTypes {
		d.ignored[t] = reason
	}
	return d
}

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

func (d *Dispatcher) Deliver(ctx context.Context, event Event) error {
	handlers, handled := d.handlers[event.EventType]
	if !handled {
		if _, ignored := d.ignored[event.EventType]; ignored {
			return nil
		}
		return fmt.Errorf("no consumer registered for event type %q", event.EventType)
	}

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
