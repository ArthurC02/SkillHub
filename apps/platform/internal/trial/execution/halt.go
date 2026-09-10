package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

const (
	HaltSourceIncident = "p1_incident"

	HaltSourceOrphanThreshold = "orphan_threshold"
)

const haltPool = ""

const haltRecoveryRounds = 2

var ErrDispatchHalted = errors.New("the execution environment is temporarily unavailable")

type haltState struct {
	byTarget map[string]gen.DispatchHalt
}

func (h haltState) active(target string) (gen.DispatchHalt, bool) {
	halt, ok := h.byTarget[target]
	return halt, ok
}

func (h haltState) incidentHeld(provider string) bool {
	if halt, ok := h.active(haltPool); ok && halt.Source == HaltSourceIncident {
		return true
	}
	if provider == haltPool {
		return false
	}
	halt, ok := h.active(provider)
	return ok && halt.Source == HaltSourceIncident
}

func (h haltState) dispatchPaused(registry *Registry) bool {
	if _, ok := h.active(haltPool); ok {
		return true
	}
	if len(registry.Providers) == 0 {
		return false
	}
	for _, p := range registry.Providers {
		if _, ok := h.active(p.Name); !ok {
			return false
		}
	}
	return true
}

func (h haltState) incidentPaused(registry *Registry) bool {
	if h.incidentHeld(haltPool) {
		return true
	}
	if len(registry.Providers) == 0 {
		return false
	}
	for _, p := range registry.Providers {
		if !h.incidentHeld(p.Name) {
			return false
		}
	}
	return true
}

func (s *Service) activeHalts(ctx context.Context) (haltState, error) {
	rows, err := s.queries().ListActiveDispatchHalts(ctx)
	if err != nil {
		return haltState{}, err
	}
	state := haltState{byTarget: make(map[string]gen.DispatchHalt, len(rows))}
	for _, row := range rows {
		state.byTarget[row.Provider] = row
	}
	return state, nil
}

func (s *Service) haltsFailClosed(ctx context.Context) haltState {
	state, err := s.activeHalts(ctx)
	if err == nil {
		return state
	}
	slog.Error("dispatch halt state unreadable; failing closed as if a P1 were declared", "error", err)
	return haltState{byTarget: map[string]gen.DispatchHalt{
		haltPool: {Provider: haltPool, Source: HaltSourceIncident, Reason: "halt state unreadable"},
	}}
}

func (s *Service) requireDispatchable(ctx context.Context) error {
	state, err := s.activeHalts(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; refusing run creation", "error", err)
		return ErrDispatchHalted
	}
	if state.incidentPaused(s.providers()) {
		return ErrDispatchHalted
	}
	return nil
}

func (s *Service) DeclareHalt(ctx context.Context, provider, source, reason string, actor pgtype.UUID) (gen.DispatchHalt, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.DispatchHalt{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	halt, err := q.DeclareDispatchHalt(ctx, gen.DeclareDispatchHaltParams{
		Provider: provider, Source: source, Reason: reason, DeclaredBy: actor,
	})
	if err != nil {
		return gen.DispatchHalt{}, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Action:       audit.ActionDispatchHalt,
		ResourceType: audit.ResourceDispatch,
		ResourceID:   halt.ID,
		Metadata: map[string]any{
			"target": haltTarget(provider),

			"source":           halt.Source,
			"requested_source": source,
			"reason":           halt.Reason,
		},
	}); err != nil {
		return gen.DispatchHalt{}, err
	}
	return halt, tx.Commit(ctx)
}

func (s *Service) LiftHalt(
	ctx context.Context, provider, reason string, actor pgtype.UUID, sources []string,
) (gen.DispatchHalt, bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return gen.DispatchHalt{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	halt, err := q.LiftDispatchHalt(ctx, gen.LiftDispatchHaltParams{
		Provider: provider, LiftReason: &reason, LiftedBy: actor, Sources: sources,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.DispatchHalt{}, false, nil
	}
	if err != nil {
		return gen.DispatchHalt{}, false, err
	}
	if err := audit.Log(ctx, tx, audit.Event{
		Actor:        actor,
		Action:       audit.ActionDispatchResume,
		ResourceType: audit.ResourceDispatch,
		ResourceID:   halt.ID,
		Metadata: map[string]any{
			"target":       haltTarget(provider),
			"source":       halt.Source,
			"halt_reason":  halt.Reason,
			"lift_reason":  reason,
			"clear_rounds": halt.ClearRounds,
		},
	}); err != nil {
		return gen.DispatchHalt{}, false, err
	}
	return halt, true, tx.Commit(ctx)
}

func haltTarget(provider string) string {
	if provider == haltPool {
		return "pool"
	}
	return provider
}

func haltThreshold(slots int, numerator, denominator, floor int64) int64 {
	// Ceiling division: rounds a fractional threshold up to the next integer.
	threshold := (int64(slots)*numerator + denominator - 1) / denominator
	if threshold < floor {
		return floor
	}
	return threshold
}

func (s *Service) EvaluateOrphanThresholds(ctx context.Context) {
	registry := s.providers()
	var poolOrphans, poolSlots int64
	for _, provider := range registry.Providers {
		persistent, err := s.queries().CountPersistentOrphans(ctx, provider.Name)
		if err != nil {
			slog.Error("counting persistent orphans for the X-04 threshold failed",
				"provider", provider.Name, "error", err)
			continue
		}
		slots := 0
		if capability, err := registry.Capability(ctx, provider); err == nil {
			slots = capability.Availability.ConcurrentRunSlots
		}
		poolOrphans += persistent
		poolSlots += int64(slots)

		threshold := haltThreshold(slots, 1, 2, 1)
		s.reconcileThresholdHalt(ctx, provider.Name, persistent >= threshold, fmt.Sprintf(
			"ADR-022 X-04: %d leaked sandboxes on %s have survived two reconciler rounds, at or above the %d that drains a node with %d declared slots",
			persistent, provider.Name, threshold, slots))
	}

	if len(registry.Providers) > 0 {
		threshold := haltThreshold(int(poolSlots), 1, 4, 2)
		s.reconcileThresholdHalt(ctx, haltPool, poolOrphans >= threshold, fmt.Sprintf(
			"ADR-022 X-04: %d leaked sandboxes fleet-wide have survived two reconciler rounds, at or above the %d that suspends dispatch across %d declared slots",
			poolOrphans, threshold, poolSlots))
	}
	s.publishHaltMetrics(ctx)
}

func (s *Service) reconcileThresholdHalt(ctx context.Context, provider string, breached bool, reason string) {
	if breached {

		if _, err := s.DeclareHalt(ctx, provider, HaltSourceOrphanThreshold, reason, pgtype.UUID{}); err != nil {
			slog.Error("could not halt dispatch on the X-04 threshold",
				"target", haltTarget(provider), "error", err)
			return
		}
		slog.Warn("dispatch halted by the X-04 threshold", "target", haltTarget(provider), "reason", reason)
		return
	}

	rounds, err := s.queries().SetDispatchHaltClearRounds(ctx, gen.SetDispatchHaltClearRoundsParams{
		Clear: true, Provider: provider,
	})
	if errors.Is(err, pgx.ErrNoRows) {

		return
	}
	if err != nil {
		slog.Error("counting clear rounds failed", "target", haltTarget(provider), "error", err)
		return
	}
	if int(rounds) < haltRecoveryRounds {
		return
	}

	if _, lifted, err := s.LiftHalt(ctx, provider, fmt.Sprintf(
		"ADR-022 X-04: below the threshold for %d consecutive reconciler rounds", rounds),
		pgtype.UUID{}, []string{HaltSourceOrphanThreshold}); err != nil {
		slog.Error("could not lift the X-04 halt", "target", haltTarget(provider), "error", err)
	} else if lifted {
		slog.Info("dispatch resumed: X-04 threshold clear", "target", haltTarget(provider), "rounds", rounds)
	}
}

func (s *Service) publishHaltMetrics(ctx context.Context) {
	state, err := s.activeHalts(ctx)
	if err != nil {

		slog.Error("could not publish dispatch halt metrics", "error", err)
		return
	}
	metrics.DispatchHalted.Reset()
	for target, halt := range state.byTarget {
		metrics.DispatchHalted.WithLabelValues(haltTarget(target), halt.Source).Set(1)
	}
}

const maxHaltNoteBytes = 1000

type haltRequest struct {
	Provider string `json:"provider"`
	Note     string `json:"note"`
}

func sessionActor(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return user, false
	}
	return user, true
}

func (h *Handler) Halts(w http.ResponseWriter, r *http.Request) {
	if _, ok := sessionActor(w, r); !ok {
		return
	}
	state, err := h.Svc.activeHalts(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch halt lookup failed")
		return
	}
	halts := make([]map[string]any, 0, len(state.byTarget))
	for target, halt := range state.byTarget {
		halts = append(halts, map[string]any{
			"target":       haltTarget(target),
			"source":       halt.Source,
			"reason":       halt.Reason,
			"declared_at":  pgconv.RFC3339(halt.DeclaredAt),
			"clear_rounds": halt.ClearRounds,

			"automatic_recovery": halt.Source == HaltSourceOrphanThreshold,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"dispatching": len(state.byTarget) == 0 || !state.dispatchPaused(h.Svc.providers()),
		"halts":       halts,
	})
}

func (h *Handler) DeclareHalt(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeHaltRequest(w, r)
	if !ok {
		return
	}
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}
	halt, err := h.Svc.DeclareHalt(r.Context(), body.Provider, HaltSourceIncident, body.Note, user.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch halt failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"target":      haltTarget(halt.Provider),
		"source":      halt.Source,
		"reason":      halt.Reason,
		"declared_at": pgconv.RFC3339(halt.DeclaredAt),
		"note": "new runs are refused and nothing is dispatched to this target; " +
			"cleanup and orphan teardown stand down so the scene is preserved. " +
			"This halt is never lifted automatically.",
	})
}

func (h *Handler) LiftHalt(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeHaltRequest(w, r)
	if !ok {
		return
	}
	user, ok := sessionActor(w, r)
	if !ok {
		return
	}
	if _, _, err := h.Svc.LiftHalt(r.Context(), body.Provider, body.Note, user.ID,
		[]string{HaltSourceIncident, HaltSourceOrphanThreshold}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch resume failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) decodeHaltRequest(w http.ResponseWriter, r *http.Request) (haltRequest, bool) {
	var body haltRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "body must be JSON with a note")
		return haltRequest{}, false
	}
	body.Note = strings.TrimSpace(body.Note)
	body.Provider = strings.TrimSpace(body.Provider)
	if body.Note == "" {
		httpx.WriteError(w, http.StatusBadRequest,
			"note is required: halting or resuming the fleet is an operator action nobody can explain later otherwise")
		return haltRequest{}, false
	}
	if len(body.Note) > maxHaltNoteBytes {
		httpx.WriteError(w, http.StatusBadRequest, "note is too long")
		return haltRequest{}, false
	}
	if body.Provider != haltPool && h.Svc.providers().Lookup(body.Provider) == nil {
		httpx.WriteError(w, http.StatusBadRequest,
			"unknown provider; omit it to halt the whole pool")
		return haltRequest{}, false
	}
	return body, true
}

const (
	maskingWindow  = time.Hour
	maskingConfirm = time.Hour
)

const reconcilerStallWindow = 10 * time.Minute

const reconcilerLastRun = `SELECT max(coalesce(finalized_at, attempted_at)) FROM river_job WHERE kind = $1`

func (s *Service) detectMaskingStopped(ctx context.Context) {
	if s.incidentAlreadyHeld(ctx) {
		return
	}
	now := time.Now()
	window, err := s.maskingActivity(ctx, now.Add(-maskingWindow), now.Add(-maskingWindow-maskingConfirm))
	if err != nil {
		slog.Error("could not evaluate the trace masking criterion", "error", err)
		return
	}

	if window.RecentEvents == 0 || window.EarlierEvents == 0 || window.MaskedFields > 0 {
		return
	}
	s.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (TraceMaskingStopped): %d trace events were stored over the last %s (%d of them in the last %s) "+
			"and not one field was redacted in any of them; under 0019's CHECK (masked) that is the masker's rules "+
			"failing while every row still claims to be masked (NFR-002, iron rule 11)",
		window.RecentEvents+window.EarlierEvents, maskingWindow+maskingConfirm,
		window.RecentEvents, maskingWindow))
}

func (s *Service) detectMaskerCanaryFailed(ctx context.Context) {

	survived := s.maskerCanary()
	if len(survived) == 0 {
		return
	}
	if s.incidentAlreadyHeld(ctx) {
		return
	}
	s.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (TraceMaskingStopped, canary): the trace masker was handed one synthetic secret of "+
			"every shape it knows and returned these unredacted: %s. Nothing was inferred from traffic — this is "+
			"the masker itself failing to redact, so anything a sandbox prints from here on lands in trace_events "+
			"in the clear (NFR-002, iron rule 11)",
		strings.Join(survived, ", ")))
}

func (s *Service) detectP02Breach(ctx context.Context) {
	registry := s.providers()
	var breached []string
	for _, provider := range registry.Providers {
		capability, err := registry.Capability(ctx, provider)
		if err != nil {

			continue
		}
		if ok, detail := capability.P02Breach(); ok {
			breached = append(breached, provider.Name+": "+detail)
		}
	}
	if len(breached) == 0 {
		return
	}
	if s.incidentAlreadyHeld(ctx) {
		return
	}
	s.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1 (P-02): a node's resident probe opened a connection from a sandbox's own network "+
			"position to an address a sandbox must never reach — %s. ADR-022 T10 calls this an architecture "+
			"regression signal: the isolation the whole execution plane rests on (iron rule 2) is not holding "+
			"on that node, and every run it carried could have done the same thing",
		strings.Join(breached, "; ")))
}

func (s *Service) maskerCanary() []string {
	if s.MaskerCanary != nil {
		return s.MaskerCanary()
	}
	return trace.MaskerCanary()
}

func (s *Service) maskingActivity(ctx context.Context, recent, since time.Time) (trace.MaskingActivityFacts, error) {
	if s.Trace == nil {
		return trace.MaskingActivityFacts{}, errors.New("run: trace service not injected")
	}
	return s.Trace.MaskingActivity(ctx, recent, since)
}

func (s *Service) DetectReconcilerStall(ctx context.Context) {
	if s.incidentAlreadyHeld(ctx) {
		return
	}
	var last pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, reconcilerLastRun, OrphanScanArgs{}.Kind()).Scan(&last); err != nil {
		slog.Error("could not read when the orphan reconciler last ran", "error", err)
		return
	}

	if !last.Valid {
		return
	}
	idle := time.Since(last.Time)
	if idle <= reconcilerStallWindow {
		return
	}
	s.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1: the orphan reconciler last ran %s ago, past the %s ADR-022 X-02 calls stalled (two missed five-minute rounds); "+
			"nothing is counting what has leaked, so nothing may be dispatched",
		idle.Round(time.Second), reconcilerStallWindow))
}

func (s *Service) WatchReconciler(ctx context.Context) {
	ticker := time.NewTicker(OrphanScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.DetectReconcilerStall(ctx)
		}
	}
}

func (s *Service) incidentAlreadyHeld(ctx context.Context) bool {
	state, err := s.activeHalts(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; skipping automatic P1 detection", "error", err)
		return true
	}
	return state.incidentHeld(haltPool)
}

func (s *Service) declareDetectedIncident(ctx context.Context, reason string) {
	if _, err := s.DeclareHalt(ctx, haltPool, HaltSourceIncident, reason, pgtype.UUID{}); err != nil {
		slog.Error("a P1 criterion was met but dispatch could not be stopped", "reason", reason, "error", err)
		return
	}
	slog.Error("P1 detected: dispatch stopped fleet-wide and the scene is preserved; "+
		"this halt is never lifted automatically", "reason", reason)
}
