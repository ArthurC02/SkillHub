package halt

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

type NodeReading struct {
	Slots     int
	P02Breach string
}

type Node struct {
	Name string
	Read func(context.Context) (NodeReading, error)
}

type Detector struct {
	Ledger
	Nodes                 []Node
	PersistentAfterRounds int32
	MinRoundGap           time.Duration
	MaskerCanary          func() []string
	MaskingActivity       func(ctx context.Context, recent, since time.Time) (trace.MaskingActivityFacts, error)
	LastOrphanScan        func(context.Context) (time.Time, bool, error)
	Now                   func() time.Time
}

const (
	nodeDrainSlotDivisor   = 2
	nodeDrainFloor         = 1
	poolSuspendSlotDivisor = 4
	poolSuspendFloor       = 2
)

func haltThreshold(slots int, denominator, floor int64) int64 {
	// Ceiling division: rounds a fractional threshold up to the next integer.
	t := (int64(slots) + denominator - 1) / denominator
	if t < floor {
		return floor
	}
	return t
}

func (d Detector) EvaluateOrphanThresholds(ctx context.Context) {
	var poolOrphans, poolSlots int64
	poolKnown := true
	for _, node := range d.Nodes {
		persistent, err := d.queries().CountPersistentOrphans(ctx, gen.CountPersistentOrphansParams{
			Provider: node.Name, PersistentAfterRounds: d.PersistentAfterRounds,
		})
		if err != nil {
			poolKnown = false
			slog.Error("counting persistent orphans for the X-04 threshold failed",
				"provider", node.Name, "error", err)
			continue
		}
		reading, err := node.Read(ctx)
		if err != nil {
			poolKnown = false
			slog.Warn("a provider's slots are unreadable; its leak threshold keeps its last verdict this round",
				"provider", node.Name, "error", err)
			continue
		}
		poolOrphans += persistent
		poolSlots += int64(reading.Slots)

		limit := haltThreshold(reading.Slots, nodeDrainSlotDivisor, nodeDrainFloor)
		d.reconcileThresholdHalt(ctx, node.Name, orphanCount{
			persistent: persistent, threshold: limit,
			reason: fmt.Sprintf(
				"X-04: %d leaked sandboxes on %s have survived two reconciler rounds, at or above the %d that drains a node with %d declared slots",
				persistent, node.Name, limit, reading.Slots),
		})
	}

	if len(d.Nodes) > 0 && poolKnown {
		limit := haltThreshold(int(poolSlots), poolSuspendSlotDivisor, poolSuspendFloor)
		d.reconcileThresholdHalt(ctx, Pool, orphanCount{
			persistent: poolOrphans, threshold: limit,
			reason: fmt.Sprintf(
				"X-04: %d leaked sandboxes fleet-wide have survived two reconciler rounds, at or above the %d that suspends dispatch across %d declared slots",
				poolOrphans, limit, poolSlots),
		})
	}
	_ = d.PublishMetrics(ctx)
}

type orphanCount struct {
	persistent int64
	threshold  int64
	reason     string
}

func (c orphanCount) breached() bool { return c.persistent >= c.threshold }

func (d Detector) reconcileThresholdHalt(ctx context.Context, provider string, count orphanCount) {
	if count.breached() {
		d.declareThresholdHalt(ctx, provider, count.reason)
		return
	}
	d.CountClearRound(ctx, provider, d.MinRoundGap)
}

func (d Detector) declareThresholdHalt(ctx context.Context, provider, reason string) {
	if _, err := d.Declare(ctx, provider, OrphanThreshold, reason, pgtype.UUID{}); err != nil {
		slog.Error("could not halt dispatch on the X-04 threshold",
			"target", Target(provider), "error", err)
		return
	}
	slog.Warn("dispatch halted by the X-04 threshold", "target", Target(provider), "reason", reason)
}

const (
	maskingWindow  = time.Hour
	maskingConfirm = time.Hour
)

const reconcilerStallWindow = 10 * time.Minute

func (d Detector) DetectMaskingStopped(ctx context.Context) {
	if d.incidentAlreadyHeld(ctx) {
		return
	}
	now := time.Now()
	window, err := d.maskingActivity(ctx, now.Add(-maskingWindow), now.Add(-maskingWindow-maskingConfirm))
	if err != nil {
		slog.Error("could not evaluate the trace masking criterion", "error", err)
		return
	}

	if window.RecentEvents == 0 || window.EarlierEvents == 0 || window.MaskedFields > 0 {
		return
	}
	d.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (TraceMaskingStopped): %d trace events were stored over the last %s (%d of them in the last %s) "+
			"and not one field was redacted in any of them; under 0019's CHECK (masked) that is the masker's rules "+
			"failing while every row still claims to be masked (NFR-002, iron rule 11)",
		window.RecentEvents+window.EarlierEvents, maskingWindow+maskingConfirm,
		window.RecentEvents, maskingWindow))
}

func (d Detector) DetectMaskerCanaryFailed(ctx context.Context) {
	survived := d.maskerCanary()
	if len(survived) == 0 {
		return
	}
	if d.incidentAlreadyHeld(ctx) {
		return
	}
	d.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (TraceMaskingStopped, canary): the trace masker was handed one synthetic secret of "+
			"every shape it knows and returned these unredacted: %s. Nothing was inferred from traffic — this is "+
			"the masker itself failing to redact, so anything a sandbox prints from here on lands in trace_events "+
			"in the clear (NFR-002, iron rule 11)",
		strings.Join(survived, ", ")))
}

func (d Detector) DetectP02Breach(ctx context.Context) {
	var breached []string
	for _, node := range d.Nodes {
		reading, err := node.Read(ctx)
		if err != nil {
			continue
		}
		if reading.P02Breach != "" {
			breached = append(breached, node.Name+": "+reading.P02Breach)
		}
	}
	if len(breached) == 0 {
		return
	}
	if d.incidentAlreadyHeld(ctx) {
		return
	}
	d.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (P-02): a node's resident probe opened a connection from a sandbox's own network "+
			"position to an address a sandbox must never reach — %s. This is an architecture "+
			"regression signal: the isolation the whole execution plane rests on (iron rule 2) is not holding "+
			"on that node, and every run it carried could have done the same thing",
		strings.Join(breached, "; ")))
}

func (d Detector) maskerCanary() []string {
	if d.MaskerCanary != nil {
		return d.MaskerCanary()
	}
	return trace.MaskerCanary()
}

func (d Detector) maskingActivity(ctx context.Context, recent, since time.Time) (trace.MaskingActivityFacts, error) {
	if d.MaskingActivity == nil {
		return trace.MaskingActivityFacts{}, errors.New("run: trace service not injected")
	}
	return d.MaskingActivity(ctx, recent, since)
}

func (d Detector) DetectReconcilerStall(ctx context.Context) {
	if d.incidentAlreadyHeld(ctx) {
		return
	}
	if d.LastOrphanScan == nil {
		slog.Error("orphan reconciler monitor is not injected")
		return
	}
	last, found, err := d.LastOrphanScan(ctx)
	if err != nil {
		slog.Error("could not read when the orphan reconciler last ran", "error", err)
		return
	}
	if !found {
		return
	}
	idle := d.Now().Sub(last)
	if idle <= reconcilerStallWindow {
		return
	}
	d.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1: the orphan reconciler last ran %s ago, past the %s X-02 calls stalled (two missed five-minute rounds); "+
			"nothing is counting what has leaked, so nothing may be dispatched",
		idle.Round(time.Second), reconcilerStallWindow))
}

func (d Detector) incidentAlreadyHeld(ctx context.Context) bool {
	state, err := d.Active(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; skipping automatic P1 detection", "error", err)
		return true
	}
	return state.IncidentHeld(Pool)
}

func (d Detector) declareDetectedIncident(ctx context.Context, reason string) {
	if _, err := d.Declare(ctx, Pool, Incident, reason, pgtype.UUID{}); err != nil {
		slog.Error("a P1 criterion was met but dispatch could not be stopped", "reason", reason, "error", err)
		return
	}
	slog.Error("P1 detected: dispatch stopped fleet-wide and the scene is preserved; "+
		"this halt is never lifted automatically", "reason", reason)
}
