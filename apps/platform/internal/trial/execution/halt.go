package run

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/httpx"
	limits "github.com/ArthurC02/skillhub/apps/platform/internal/shared/contractlimits/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/halt"
)

var ErrDispatchHalted = errors.New("the execution environment is temporarily unavailable")

func (s *Service) halts() halt.Ledger { return halt.Ledger{Pool: s.Pool} }

type DispatchGate struct {
	ledger    halt.Ledger
	providers func() []string
	now       func() time.Time
}

func (s *Service) DispatchGate() DispatchGate {
	return DispatchGate{ledger: s.halts(), providers: func() []string { return s.providers().Names() }, now: s.now}
}

func (g DispatchGate) requireDispatchable(ctx context.Context) error {
	state, err := g.ledger.Active(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; refusing run creation", "error", err)
		return ErrDispatchHalted
	}
	if state.IncidentPaused(g.providers()) {
		return ErrDispatchHalted
	}
	return nil
}

func (g DispatchGate) ModelCallsHalted(ctx context.Context) bool {
	return errors.Is(g.requireDispatchable(ctx), ErrDispatchHalted)
}

func (g DispatchGate) Paused(ctx context.Context) bool {
	return g.ledger.FailClosed(ctx).DispatchPaused(g.providers())
}

func (g DispatchGate) heldWithin(ctx context.Context, from, to time.Time) time.Duration {
	held, err := g.ledger.PausedWithin(ctx, g.providers(), from, to)
	if err != nil {
		slog.Error("dispatch halt history unreadable; the queued run keeps waiting", "error", err)
		return to.Sub(from)
	}
	return held
}

func (s *Service) detector() halt.Detector {
	registry := s.providers()
	nodes := make([]halt.Node, len(registry.Providers))
	for i, provider := range registry.Providers {
		nodes[i] = halt.Node{Name: provider.Name(), Read: func(ctx context.Context) (halt.NodeReading, error) {
			capability, err := registry.Capability(ctx, provider)
			if err != nil {
				return halt.NodeReading{}, err
			}
			_, breach := capability.P02Breach()
			return halt.NodeReading{Slots: capability.Availability.ConcurrentRunSlots, P02Breach: breach}, nil
		}}
	}
	d := halt.Detector{
		Ledger: s.halts(), Nodes: nodes,
		PersistentAfterRounds: OrphanPersistsAfterRounds, MinRoundGap: minRoundGap,
		MaskerCanary: s.MaskerCanary, LastOrphanScan: s.LastOrphanScan, Now: s.now,
	}
	if s.Trace != nil {
		d.MaskingActivity = s.Trace.MaskingActivity
	}
	return d
}

func (s *Service) EvaluateOrphanThresholds(ctx context.Context) {
	s.detector().EvaluateOrphanThresholds(ctx)
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
	if !httpx.DecodeJSON(w, r, maxRequestBodyBytes, &body, "body must be JSON with a note") {
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

func (s *Service) DetectReconcilerStall(ctx context.Context) { s.detector().DetectReconcilerStall(ctx) }

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

func formatSecond(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
