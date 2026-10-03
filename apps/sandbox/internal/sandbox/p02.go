package sandbox

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
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

	Skipped []string

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
	p := &P02Probe{Interval: interval, Timeout: timeout}
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, _, ok := SplitP02Target(t); ok {
			p.Targets = append(p.Targets, t)
		} else {
			p.Skipped = append(p.Skipped, t)
		}
	}
	sort.Strings(p.Targets)
	sort.Strings(p.Skipped)
	if !p.Configured() {
		p.result = P02Result{State: P02NotConfigured}
		return p
	}
	if len(p.Targets) == 0 {
		p.result = p.nothingToDial(time.Time{})
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

func (p *P02Probe) Configured() bool { return len(p.Targets)+len(p.Skipped) > 0 }

func SplitP02Target(target string) (string, int, bool) {
	// LastIndex, so an IPv6 host keeps its own colons and only the port splits off.
	i := strings.LastIndex(target, ":")
	if i <= 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(target[i+1:])
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, false
	}
	return target[:i], port, true
}

func (p *P02Probe) nothingToDial(now time.Time) P02Result {
	return P02Result{State: P02Unknown, CheckedAt: now,
		Detail: "no target is host:port, so nothing was dialled: " + strings.Join(p.Skipped, ", ")}
}

func (p *P02Probe) skippedNote() string {
	if len(p.Skipped) == 0 {
		return ""
	}
	return "; not checked, not host:port: " + strings.Join(p.Skipped, ", ")
}

func (p *P02Probe) Check(ctx context.Context, prober EgressProber, now time.Time) P02Result {
	if !p.Configured() {
		return p.store(P02Result{State: P02NotConfigured, CheckedAt: now})
	}
	if len(p.Targets) == 0 {
		return p.store(p.nothingToDial(now))
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
			Detail: fmt.Sprintf("%d destination(s) unreachable", len(p.Targets)) + p.skippedNote()})
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
	if log != nil && len(p.Skipped) > 0 {
		log.Warn("P-02 targets that are not host:port are not probed", "skipped", strings.Join(p.Skipped, ", "))
	}
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
