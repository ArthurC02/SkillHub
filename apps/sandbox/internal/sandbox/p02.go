package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

type P02State string

const (
	P02Pass P02State = "pass"

	P02Fail P02State = "fail"

	P02Unknown P02State = "unknown"

	P02NotConfigured P02State = "not_configured"
)

type P02Result struct {
	State     P02State  `json:"state"`
	CheckedAt time.Time `json:"checked_at"`
	Detail    string    `json:"detail,omitempty"`
}

type EgressProber interface {
	ProbeEgress(ctx context.Context, targets []string) (reached []string, err error)
}

type P02Probe struct {
	Targets []string

	Interval time.Duration

	Timeout time.Duration

	mu     sync.RWMutex
	result P02Result
}

const (
	defaultP02Interval = 5 * time.Minute
	defaultP02Timeout  = 30 * time.Second
)

func NewP02Probe(targets []string, interval, timeout time.Duration) *P02Probe {
	if interval <= 0 {
		interval = defaultP02Interval
	}
	if timeout <= 0 {
		timeout = defaultP02Timeout
	}
	clean := make([]string, 0, len(targets))
	for _, t := range targets {
		if t = strings.TrimSpace(t); t != "" {
			clean = append(clean, t)
		}
	}
	sort.Strings(clean)
	p := &P02Probe{Targets: clean, Interval: interval, Timeout: timeout}
	if len(clean) == 0 {
		p.result = P02Result{State: P02NotConfigured}
		return p
	}

	p.result = P02Result{State: P02Unknown, Detail: "no reading taken yet"}
	return p
}

func (p *P02Probe) Result() P02Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.result
}

func (p *P02Probe) Configured() bool { return len(p.Targets) > 0 }

func (p *P02Probe) Check(ctx context.Context, prober EgressProber, now time.Time) P02Result {
	if !p.Configured() {
		return p.store(P02Result{State: P02NotConfigured, CheckedAt: now})
	}
	if prober == nil {
		return p.store(P02Result{State: P02Unknown, CheckedAt: now,
			Detail: "this driver cannot dial from a sandbox's network position"})
	}
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	reached, err := prober.ProbeEgress(ctx, p.Targets)
	switch {
	case err != nil:

		return p.store(P02Result{State: P02Unknown, CheckedAt: now,
			Detail: "probe did not complete: " + err.Error()})
	case len(reached) > 0:
		return p.store(P02Result{State: P02Fail, CheckedAt: now,
			Detail: "reachable from a sandbox: " + strings.Join(reached, ", ")})
	default:
		return p.store(P02Result{State: P02Pass, CheckedAt: now,
			Detail: fmt.Sprintf("%d destination(s) unreachable", len(p.Targets))})
	}
}

func (p *P02Probe) store(r P02Result) P02Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.result = r
	return r
}

func (p *P02Probe) Run(ctx context.Context, prober EgressProber, onBreach func(P02Result), log *slog.Logger) {
	tick := time.NewTicker(p.Interval)
	defer tick.Stop()
	for {
		r := p.Check(ctx, prober, time.Now().UTC())
		switch r.State {
		case P02Fail:
			if log != nil {

				log.Error("P-02 breach: a sandbox can reach an address it must not",
					"detail", r.Detail, "action", "terminating live runs and refusing new ones")
			}
			if onBreach != nil {
				onBreach(r)
			}
		case P02Unknown:
			if log != nil {
				log.Warn("P-02 probe could not complete; this node reports itself unhealthy",
					"detail", r.Detail)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
