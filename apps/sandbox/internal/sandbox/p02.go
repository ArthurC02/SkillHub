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

// P-02 of the threat model: a sandbox must not be able to open a connection to
// the core database or to the internal control services, and ADR-022 promotes
// it out of the declarative audit into its own acceptance item T10.
//
// The reason it is its own item is worth keeping next to the code. Every other
// P-zone check is configuration a single snapshot satisfies - the node is
// single-tenant, it was built by IaC, its gVisor is at the baseline version.
// P-02 says in so many words that the block must be verified *by a probe, not
// by reading configuration*, and that the probe is resident. A one-time audit
// reporting into the same field would let a snapshot impersonate continuous
// monitoring, which is the one thing this check is not allowed to be.
//
// WHERE THE PROBE DIALS FROM IS THE WHOLE MEASUREMENT
//
// sandboxd runs on the node with the node's own network access. It can reach
// the database; that is not a finding, it is its job. A probe that dialled from
// this process would answer a question nobody asked and would answer it `fail`
// forever. So the attempt is made from inside a sandbox, on the same network a
// run gets, through the driver - which is why EgressProber is a driver
// capability and not a function in this file.

// P02State is what the node currently knows about itself.
type P02State string

const (
	// P02Pass: the probe ran and could not get through.
	P02Pass P02State = "pass"
	// P02Fail: a connection SUCCEEDED. This is the architecture regression
	// signal of 02:SEC-010's P1 row.
	P02Fail P02State = "fail"
	// P02Unknown: the probe could not complete. ADR-022 §3 counts unknown as
	// fail for acceptance, and this code does not merge the two anyway: `fail`
	// is evidence of a hole and `unknown` is the absence of evidence either way.
	// They earn different actions below.
	P02Unknown P02State = "unknown"
	// P02NotConfigured: nobody told this node which addresses it must not reach.
	// Reported rather than omitted: a field that disappears reads as `pass` to
	// anything scanning for problems.
	P02NotConfigured P02State = "not_configured"
)

// P02Result is one reading, and CheckedAt is not decoration. A resident probe
// that stopped running keeps answering `pass` with a timestamp that stops
// moving, and from outside the node that is the only way to see it.
type P02Result struct {
	State     P02State  `json:"state"`
	CheckedAt time.Time `json:"checked_at"`
	Detail    string    `json:"detail,omitempty"`
}

// EgressProber is the driver's half of T10: open a TCP connection to each
// target from a sandbox's own network position and report which ones answered.
//
// It is an optional interface rather than a Driver method on purpose. Driver is
// implemented by every fake in this package's tests, and a probe those fakes had
// to grow would be a probe written to satisfy them. A driver that does not
// implement this cannot answer the question, and the probe says `unknown` -
// which is exactly what it is.
type EgressProber interface {
	ProbeEgress(ctx context.Context, targets []string) (reached []string, err error)
}

// P02Probe is the resident half. One goroutine, one reading, no history: the
// interesting states are "right now" and "when was that", and a node that keeps
// a log of its own breaches is a node that already had one.
type P02Probe struct {
	// Targets are `host:port` addresses a sandbox must never reach - the core
	// database and the internal control services. They come from node
	// configuration and never from a RunRequest: the list of what must not be
	// reachable cannot be supplied by the plane being tested.
	Targets []string
	// Interval between readings. ADR-022 does not fix one; X-02's five minutes
	// for the reconciler is the nearest sibling and the default matches it.
	Interval time.Duration
	// Timeout bounds one round. A hung dial must not stop the probe: it makes
	// the reading `unknown`, which is a state, rather than freezing the last
	// `pass` in place forever.
	Timeout time.Duration

	mu     sync.RWMutex
	result P02Result
}

const (
	defaultP02Interval = 5 * time.Minute
	defaultP02Timeout  = 30 * time.Second
)

// NewP02Probe builds the probe for a node. An empty target list is not an error
// - it is `not_configured`, and main() decides whether that is allowed to run.
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
	// Before the first round completes the node has taken no reading, and
	// `unknown` is what that is. Starting at `pass` would mean a node that
	// crashed its probe on boot answered `pass` for its whole life.
	p.result = P02Result{State: P02Unknown, Detail: "no reading taken yet"}
	return p
}

// Result is the current reading, for GET /capability.
func (p *P02Probe) Result() P02Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.result
}

// Configured reports whether this node was given anything to check.
func (p *P02Probe) Configured() bool { return len(p.Targets) > 0 }

// Check takes one reading. Exported so a node can be probed once before it
// joins the pool as well as continuously afterwards - ADR-022 T10 asks for
// both, and they are the same measurement.
//
// `now` is passed in rather than read here so a test can assert on CheckedAt
// without a clock.
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
		// Not `pass`. A probe that could not run has not shown the block holds,
		// and answering `pass` here is how a resident check becomes decorative.
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

// Run takes readings until ctx ends, calling onBreach the first time a reading
// comes back `fail` and on every reading that stays `fail`.
//
// It takes a reading immediately rather than after the first tick. A node that
// waited out its interval before its first check would serve runs for five
// minutes on a claim it had not tested, and the boot of a freshly rebuilt node
// (P-03, every 7 days) is exactly when the ruleset is most likely to be wrong.
func (p *P02Probe) Run(ctx context.Context, prober EgressProber, onBreach func(P02Result), log *slog.Logger) {
	tick := time.NewTicker(p.Interval)
	defer tick.Stop()
	for {
		r := p.Check(ctx, prober, time.Now().UTC())
		switch r.State {
		case P02Fail:
			if log != nil {
				// Iron rule 11: an address and a port, never a payload.
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
