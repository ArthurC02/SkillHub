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
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/halt"
)

var ErrDispatchHalted = errors.New("the execution environment is temporarily unavailable")

func (s *Service) halts() halt.Ledger { return halt.Ledger{Pool: s.Pool} }

func (s *Service) requireDispatchable(ctx context.Context) error {
	state, err := s.halts().Active(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; refusing run creation", "error", err)
		return ErrDispatchHalted
	}
	if state.IncidentPaused(s.providers().Names()) {
		return ErrDispatchHalted
	}
	return nil
}

const (
	nodeDrainSlotDivisor   = 2
	nodeDrainFloor         = 1
	poolSuspendSlotDivisor = 4
	poolSuspendFloor       = 2
)

func haltThreshold(slots int, denominator, floor int64) int64 {
	// Ceiling division: rounds a fractional threshold up to the next integer.
	threshold := (int64(slots) + denominator - 1) / denominator
	if threshold < floor {
		return floor
	}
	return threshold
}

func (s *Service) EvaluateOrphanThresholds(ctx context.Context) {
	registry := s.providers()
	var poolOrphans, poolSlots int64
	poolKnown := true
	for _, provider := range registry.Providers {
		persistent, err := s.queries().CountPersistentOrphans(ctx, gen.CountPersistentOrphansParams{
			Provider: provider.Name(), PersistentAfterRounds: OrphanPersistsAfterRounds,
		})
		if err != nil {
			poolKnown = false
			slog.Error("counting persistent orphans for the X-04 threshold failed",
				"provider", provider.Name(), "error", err)
			continue
		}
		capability, err := registry.Capability(ctx, provider)
		if err != nil {
			poolKnown = false
			slog.Warn("a provider's slots are unreadable; its leak threshold keeps its last verdict this round",
				"provider", provider.Name(), "error", err)
			continue
		}
		slots := capability.Availability.ConcurrentRunSlots
		poolOrphans += persistent
		poolSlots += int64(slots)

		threshold := haltThreshold(slots, nodeDrainSlotDivisor, nodeDrainFloor)
		s.reconcileThresholdHalt(ctx, provider.Name(), orphanCount{
			persistent: persistent, threshold: threshold,
			reason: fmt.Sprintf(
				"X-04: %d leaked sandboxes on %s have survived two reconciler rounds, at or above the %d that drains a node with %d declared slots",
				persistent, provider.Name(), threshold, slots),
		})
	}

	if len(registry.Providers) > 0 && poolKnown {
		threshold := haltThreshold(int(poolSlots), poolSuspendSlotDivisor, poolSuspendFloor)
		s.reconcileThresholdHalt(ctx, halt.Pool, orphanCount{
			persistent: poolOrphans, threshold: threshold,
			reason: fmt.Sprintf(
				"X-04: %d leaked sandboxes fleet-wide have survived two reconciler rounds, at or above the %d that suspends dispatch across %d declared slots",
				poolOrphans, threshold, poolSlots),
		})
	}
	s.halts().PublishMetrics(ctx)
}

type orphanCount struct {
	persistent int64
	threshold  int64
	reason     string
}

func (c orphanCount) breached() bool { return c.persistent >= c.threshold }

func (s *Service) reconcileThresholdHalt(ctx context.Context, provider string, count orphanCount) {
	if count.breached() {
		s.declareThresholdHalt(ctx, provider, count.reason)
		return
	}
	s.halts().CountClearRound(ctx, provider, minRoundGap)
}

func (s *Service) declareThresholdHalt(ctx context.Context, provider, reason string) {
	if _, err := s.halts().Declare(ctx, provider, halt.OrphanThreshold, reason, pgtype.UUID{}); err != nil {
		slog.Error("could not halt dispatch on the X-04 threshold",
			"target", halt.Target(provider), "error", err)
		return
	}
	slog.Warn("dispatch halted by the X-04 threshold", "target", halt.Target(provider), "reason", reason)
}

const maxHaltNoteChars = limits.DeclareDispatchHaltBodyNoteMaxLength

type haltRequest struct {
	Provider string `json:"provider"`
	Note     string `json:"note"`
}

type dispatchHaltResponse struct {
	Target            string `json:"target"`
	Source            string `json:"source"`
	Reason            string `json:"reason"`
	DeclaredAt        string `json:"declared_at"`
	ClearRounds       int32  `json:"clear_rounds"`
	AutomaticRecovery bool   `json:"automatic_recovery"`
}

type dispatchHaltsResponse struct {
	Dispatching bool                   `json:"dispatching"`
	Halts       []dispatchHaltResponse `json:"halts"`
}

type declaredHaltResponse struct {
	Target     string `json:"target"`
	Source     string `json:"source"`
	Reason     string `json:"reason"`
	DeclaredAt string `json:"declared_at"`
	Note       string `json:"note"`
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
	state, err := h.Svc.halts().Active(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch halt lookup failed")
		return
	}
	active := state.Halts()
	halts := make([]dispatchHaltResponse, 0, len(active))
	for _, held := range active {
		halts = append(halts, dispatchHaltResponse{
			Target: halt.Target(held.Provider), Source: string(held.Source), Reason: held.Reason,
			DeclaredAt: formatSecond(held.DeclaredAt), ClearRounds: held.ClearRounds,
			AutomaticRecovery: held.Source.RecoversAutomatically(),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, dispatchHaltsResponse{
		Dispatching: len(active) == 0 || !state.DispatchPaused(h.Svc.providers().Names()), Halts: halts,
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
	declared, err := h.Svc.halts().Declare(r.Context(), body.Provider, halt.Incident, body.Note, user.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch halt failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, declaredHaltResponse{
		Target: halt.Target(declared.Provider), Source: string(declared.Source), Reason: declared.Reason,
		DeclaredAt: formatTime(declared.DeclaredAt), Note: "new runs are refused and nothing is dispatched to this target; " +
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
	if _, _, err := h.Svc.halts().Lift(r.Context(), body.Provider, body.Note, user.ID, halt.AllSources()); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch resume failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) decodeHaltRequest(w http.ResponseWriter, r *http.Request) (haltRequest, bool) {
	var body haltRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)).Decode(&body); err != nil {
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
	if utf8.RuneCountInString(body.Note) > maxHaltNoteChars {
		httpx.WriteError(w, http.StatusBadRequest, "note is too long")
		return haltRequest{}, false
	}
	if body.Provider != halt.Pool && h.Svc.providers().Lookup(body.Provider) == nil {
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
			breached = append(breached, provider.Name()+": "+detail)
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
			"position to an address a sandbox must never reach — %s. This is an architecture "+
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
	if s.LastOrphanScan == nil {
		slog.Error("orphan reconciler monitor is not injected")
		return
	}
	last, found, err := s.LastOrphanScan(ctx)
	if err != nil {
		slog.Error("could not read when the orphan reconciler last ran", "error", err)
		return
	}
	if !found {
		return
	}
	idle := s.now().Sub(last)
	if idle <= reconcilerStallWindow {
		return
	}
	s.declareDetectedIncident(ctx, fmt.Sprintf(
		"02:SEC-010 P1: the orphan reconciler last ran %s ago, past the %s X-02 calls stalled (two missed five-minute rounds); "+
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
	state, err := s.halts().Active(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; skipping automatic P1 detection", "error", err)
		return true
	}
	return state.IncidentHeld(halt.Pool)
}

func (s *Service) declareDetectedIncident(ctx context.Context, reason string) {
	if _, err := s.halts().Declare(ctx, halt.Pool, halt.Incident, reason, pgtype.UUID{}); err != nil {
		slog.Error("a P1 criterion was met but dispatch could not be stopped", "reason", reason, "error", err)
		return
	}
	slog.Error("P1 detected: dispatch stopped fleet-wide and the scene is preserved; "+
		"this halt is never lifted automatically", "reason", reason)
}

func formatSecond(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
