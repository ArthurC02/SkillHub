package run

// SEC-012: the automatic first action of a P1 security incident, and ADR-022
// X-04's drain/suspend, as ONE switch.
//
// 03:SEC-012 makes that sharing an implementation requirement rather than a
// note: 「ADR-022 X-04 的『單節點 ≥50% slot drain／全池 ≥25%（下限 2 筆）暫停整池
// 派送』與 P1 的『停止派送』是同一個開關的兩個觸發者，必須共用同一份狀態與同一條
// 解除路徑。做成兩套的後果是可預期的：一邊暫停、另一邊以為自己還在派送」. So both
// triggers write db/migrations/0030's one table, both releases are the one
// LiftDispatchHalt statement, and every reader below asks the same question of
// the same rows.
//
// Where X-04 was "defined" before this file: ADR-022 第二部分 §6a gives the
// numbers (50% of a node's declared slots, floor 1; 25% of the pool's, floor 2;
// recovery only after two consecutive clear rounds) and infra/observability/
// alerts.yml says in as many words that the actions those numbers trigger are
// 「平台實作，不是本告警的職責」. There was no env var, no file and no column
// holding the switch — the thresholds existed and the thing they were supposed to
// flip did not. This is that thing, built once for both callers.
//
// The three actions 02:SEC-010 requires of a P1, and where each one lives:
//
//	① stop dispatching new Runs — Create (service.go) refuses while the pool is
//	   held for an incident, and driver.dispatch (job.go) leaves runs `queued`.
//	   Two entry points because they are two processes: cmd/api creates, cmd/worker
//	   dispatches, and they restart independently.
//	② drain the affected node — a provider-scoped halt, which takes that provider
//	   out of scheduling while the sandboxes already on it run to their own end.
//	   That is the whole of "drain" the control plane can perform; the node-side
//	   half (cordon the VM, stop sandboxd, rebuild) is a deployment-period action
//	   and is deliberately not simulated here.
//	③ preserve the scene — cleanup and orphan teardown stand down for an incident
//	   halt (cleanup.go), so the evidence a P1 exists to investigate is not the
//	   first thing the platform destroys.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/identity"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/httpx"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/metrics"
)

// The two triggers, as stored in dispatch_halts.source. They are values in one
// column and not two tables, which is the point of 03:SEC-012.
const (
	// HaltSourceIncident is 02:SEC-010's P1: declared by an operator, or by
	// anything that concludes a P1 criterion is met. Never lifted automatically
	// (03:SEC-012 「解除不得是自動的」——自動解除等於讓觸發條件自己決定何時恢復服務).
	HaltSourceIncident = "p1_incident"
	// HaltSourceOrphanThreshold is ADR-022 X-04: a capacity event the reconciler
	// raises and clears on its own.
	HaltSourceOrphanThreshold = "orphan_threshold"
)

// haltPool is the provider value that means "the whole fleet" (0030). Empty
// rather than a sentinel word, so it can never collide with a provider name from
// SKILLHUB_SANDBOX_PROVIDERS.
const haltPool = ""

// haltRecoveryRounds is ADR-022 X-04's 「連續 2 輪維持才自動恢復派送——不做單輪恢復，
// 避免在清理不穩定時來回抖動」.
const haltRecoveryRounds = 2

// ErrDispatchHalted is a run refused because the platform is not dispatching.
// The sentence is the English of ADR-022 X-04 / 02:SEC-010's 「執行環境暫時不可用」:
// it names no incident detail, because the person who cannot start a run is not
// the person the incident is about.
var ErrDispatchHalted = errors.New("the execution environment is temporarily unavailable")

// haltState is the switch as one reader sees it. Built from one query so that
// every decision in a single pass is made against the same snapshot — a Create
// that refused and a dispatch that proceeded on two different reads of the same
// moment is exactly the split answer 03:SEC-012 is about.
type haltState struct {
	// byTarget is keyed by dispatch_halts.provider; haltPool is the fleet-wide
	// entry. Nil means nothing is halted.
	byTarget map[string]gen.DispatchHalt
}

func (h haltState) active(target string) (gen.DispatchHalt, bool) {
	halt, ok := h.byTarget[target]
	return halt, ok
}

// incidentHeld reports whether a P1 is in force over this target — the pool halt
// covers every provider, so a node is held by its own halt or by the fleet's.
// The empty provider name asks about the fleet alone.
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

// dispatchPaused reports whether anything at all may be dispatched right now. The
// fleet is paused by a pool halt, and equally by every configured provider being
// drained one at a time: "all the nodes are drained" and "the pool is paused" are
// the same operational fact, and failing runs in the second case while queueing
// them in the first would be two answers to one question.
//
// A deployment with no providers configured is not paused. That is an existing
// state with an existing behaviour (the run is accepted and then fails saying no
// sandbox is configured), and it is not what this switch is about.
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

// activeHalts reads the switch.
//
// Fail-closed, and the direction is chosen rather than inherited: 02:SEC-002 says
// a check that cannot be performed counts as not passed, and ADR-022 opens with
// 「做不到的地方一律走 fail-closed（停用比假裝守得住好）」. So a switch that cannot
// be read is treated as ON, by every caller: creation refuses, dispatch waits, and
// teardown stands down. The alternative — carrying on because the table did not
// answer — is a platform that keeps handing untrusted workloads to a fleet nobody
// can confirm is meant to be running.
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

// haltsFailClosed is activeHalts for the callers that have no error channel: a
// read failure comes back as a fleet-wide incident halt, which is the state that
// makes every one of them do the safe thing.
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

// requireDispatchable is the run-creation gate (SEC-012 action ①, first entry
// point).
//
// It refuses only for an INCIDENT halt of the whole pool, and the narrowness is
// the two acceptance criteria being obeyed rather than averaged:
//
//   - ADR-022 X-04 says a threshold pause leaves runs 「停留 queued 並顯示『執行環境
//     暫時不可用』」, and that release is automatic (two clear rounds, ≤ 10 minutes),
//     so queueing is a wait with an end.
//   - 02:SEC-010 P1 has no automatic release at all. Accepting a run into a queue
//     that may not move until a human has finished an investigation is a worse
//     answer than saying so now, and 03:SEC-012's first action is 「立即停止派送新
//     Run」 rather than "accept and hold".
//
// The difference is read off the halt's own source column. It is not a second
// switch: the same row, the same release path, and dispatch stops either way.
func (s *Service) requireDispatchable(ctx context.Context) error {
	state, err := s.activeHalts(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; refusing run creation", "error", err)
		return ErrDispatchHalted
	}
	if halt, ok := state.active(haltPool); ok && halt.Source == HaltSourceIncident {
		return ErrDispatchHalted
	}
	return nil
}

// --- declaring and lifting ---------------------------------------------------

// DeclareHalt flips the switch on for one target and records who did it, in one
// transaction (iron rule 9): a fleet cannot be halted without the event that
// explains it, nor explained without being halted.
//
// actor is the zero UUID for a halt the platform declared itself — the same
// convention audit.Event uses for platform-initiated events, and what the X-04
// reconciler passes.
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
	if err := audit.Log(ctx, q, audit.Event{
		Actor:        actor,
		Action:       audit.ActionDispatchHalt,
		ResourceType: audit.ResourceDispatch,
		ResourceID:   halt.ID,
		Metadata: map[string]any{
			"target": haltTarget(provider),
			// The stored source, not the requested one: a threshold breach arriving
			// on top of a P1 leaves the halt a P1, and the trail has to say which it
			// ended up being.
			"source":           halt.Source,
			"requested_source": source,
			"reason":           halt.Reason,
		},
	}); err != nil {
		return gen.DispatchHalt{}, err
	}
	return halt, tx.Commit(ctx)
}

// LiftHalt is the single release path (03:SEC-012 「同一條解除路徑」). `sources`
// bounds what this caller may release: the reconciler passes only
// HaltSourceOrphanThreshold, so its automatic recovery can never resume a fleet
// that was stopped for a security incident.
//
// Returns ok=false when nothing was halted, which is not an error: the caller's
// intent (dispatch must not be halted) is already satisfied.
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
	if err := audit.Log(ctx, q, audit.Event{
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

// haltTarget names a halt for a human. The stored value for the fleet is the
// empty string, and an audit event whose target reads "" would be indistinguishable
// from a missing field.
func haltTarget(provider string) string {
	if provider == haltPool {
		return "pool"
	}
	return provider
}

// --- the X-04 trigger --------------------------------------------------------

// haltThreshold is ADR-022 X-04 §6a expressed once: a proportion of declared
// slots with a floor, so one formula serves the closed beta (4 slots) and early
// growth (20) alike.
//
// A provider that declared no slots — or one that is unreachable, so nobody knows
// what it declared — gets the floor. That errs towards draining a node the
// platform cannot reason about, which is the same direction as everything else in
// this file.
func haltThreshold(slots int, numerator, denominator, floor int64) int64 {
	// Ceiling division: 「≥ 25%」 of 6 slots is 1.5 resources, and the first integer
	// count that satisfies it is 2.
	threshold := (int64(slots)*numerator + denominator - 1) / denominator
	if threshold < floor {
		return floor
	}
	return threshold
}

// EvaluateOrphanThresholds applies ADR-022 X-04 to what the reconciler just saw
// and moves the shared switch accordingly. Called at the end of one orphan scan,
// so it reads the sightings that pass wrote.
//
// It never fails the scan: the teardown the scan performs is worth more than the
// bookkeeping here, and a threshold that could not be evaluated this round is
// evaluated again in five minutes (ADR-022 X-02).
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

		// 「單一節點上的遺留資源 ≥ 該節點宣告 slot 數的 50%（下限 1 筆）→ 該節點停止
		// 接受新 Run（drain），其他節點不受影響」.
		threshold := haltThreshold(slots, 1, 2, 1)
		s.reconcileThresholdHalt(ctx, provider.Name, persistent >= threshold, fmt.Sprintf(
			"ADR-022 X-04: %d leaked sandboxes on %s have survived two reconciler rounds, at or above the %d that drains a node with %d declared slots",
			persistent, provider.Name, threshold, slots))
	}

	// 「全池遺留資源 ≥ 總 slot 數的 25%（下限 2 筆）→ 暫停整個節點池派送新 Run」.
	if len(registry.Providers) > 0 {
		threshold := haltThreshold(int(poolSlots), 1, 4, 2)
		s.reconcileThresholdHalt(ctx, haltPool, poolOrphans >= threshold, fmt.Sprintf(
			"ADR-022 X-04: %d leaked sandboxes fleet-wide have survived two reconciler rounds, at or above the %d that suspends dispatch across %d declared slots",
			poolOrphans, threshold, poolSlots))
	}
	s.publishHaltMetrics(ctx)
}

// reconcileThresholdHalt is one target's half of X-04: raise the halt while the
// threshold is breached, and release it only after two consecutive clear rounds.
func (s *Service) reconcileThresholdHalt(ctx context.Context, provider string, breached bool, reason string) {
	if breached {
		// Declaring again on an already-halted target is the same upsert, and it
		// resets clear_rounds — which is what makes the two clear rounds consecutive.
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
		// Nothing this round can release: either no halt at all, or one this caller
		// is not allowed to lift (a P1). Both are the ordinary case.
		return
	}
	if err != nil {
		slog.Error("counting clear rounds failed", "target", haltTarget(provider), "error", err)
		return
	}
	if int(rounds) < haltRecoveryRounds {
		return
	}
	// Only the threshold source: a P1 declared over the same target is never
	// released here (03:SEC-012 「解除不得是自動的」).
	if _, lifted, err := s.LiftHalt(ctx, provider, fmt.Sprintf(
		"ADR-022 X-04: below the threshold for %d consecutive reconciler rounds", rounds),
		pgtype.UUID{}, []string{HaltSourceOrphanThreshold}); err != nil {
		slog.Error("could not lift the X-04 halt", "target", haltTarget(provider), "error", err)
	} else if lifted {
		slog.Info("dispatch resumed: X-04 threshold clear", "target", haltTarget(provider), "rounds", rounds)
	}
}

// publishHaltMetrics republishes the O11Y-003 gauge from the table. Reset first,
// because a target that stopped being halted has to stop being reported — a gauge
// that only ever counts up is the mistake ADR-022 already called out for the
// orphan counters.
func (s *Service) publishHaltMetrics(ctx context.Context) {
	state, err := s.activeHalts(ctx)
	if err != nil {
		// Deliberately not published as "halted": this gauge answers what the table
		// says, and a failed read is a gap, not a value. The fail-closed behaviour
		// lives on the decision paths, where it changes what the platform does.
		slog.Error("could not publish dispatch halt metrics", "error", err)
		return
	}
	metrics.DispatchHalted.Reset()
	for target, halt := range state.byTarget {
		metrics.DispatchHalted.WithLabelValues(haltTarget(target), halt.Source).Set(1)
	}
}

// --- the operator surface ----------------------------------------------------

// maxHaltNoteBytes caps the free-text note, same ceiling and same reason as the
// SEC-011 restriction note: it is operator prose that reaches the audit trail,
// and the trail is meant to stay cheap to keep for 400 days.
const maxHaltNoteBytes = 1000

type haltRequest struct {
	// Provider drains one node; absent or empty is the whole pool. A name that is
	// not in SKILLHUB_SANDBOX_PROVIDERS is refused rather than stored: a halt on a
	// misspelled node is a halt that protects nothing and reads, on the status
	// endpoint, exactly like one that does.
	Provider string `json:"provider"`
	Note     string `json:"note"`
}

// Halts handles GET /admin/dispatch. 03:SEC-012's whole objection to two switches
// is that 「現在到底有沒有在派送」 stops having a single answer; this is where that
// answer is read.
func (h *Handler) Halts(w http.ResponseWriter, r *http.Request) {
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
			"declared_at":  rfc3339(halt.DeclaredAt),
			"clear_rounds": halt.ClearRounds,
			// An incident halt waits for a person; a threshold halt clears itself.
			// Saying which is the difference between "this will come back" and
			// "somebody has to come back to it".
			"automatic_recovery": halt.Source == HaltSourceOrphanThreshold,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"dispatching": len(state.byTarget) == 0 || !state.dispatchPaused(h.Svc.providers()),
		"halts":       halts,
	})
}

// DeclareHalt handles PUT /admin/dispatch/halt: 02:SEC-010's P1 first action,
// performed by hand.
//
// This endpoint exists because most P1 criteria are not things the control plane
// can conclude on its own — 逃逸疑慮 is a judgement, the P-02 probe and the gVisor
// advisory cron both live outside this process, and a Reconciler that has stopped
// cannot be the thing that notices it stopped. What the platform CAN do is make
// the response one request rather than a database session, and 02:SEC-010's
// escalation rule 「不確定屬 P1 或 P2 時一律以 P1 處理」 only works if declaring a P1
// is cheaper than deciding it is not one.
//
// Idempotent, like the SEC-011 operator routes: re-declaring an active halt
// rewrites its reason and writes a second audit event, because an operator
// repeating an action is not an error.
func (h *Handler) DeclareHalt(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeHaltRequest(w, r)
	if !ok {
		return
	}
	user, _ := identity.SessionUser(r.Context())
	halt, err := h.Svc.DeclareHalt(r.Context(), body.Provider, HaltSourceIncident, body.Note, user.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "dispatch halt failed")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"target":      haltTarget(halt.Provider),
		"source":      halt.Source,
		"reason":      halt.Reason,
		"declared_at": rfc3339(halt.DeclaredAt),
		"note": "new runs are refused and nothing is dispatched to this target; " +
			"cleanup and orphan teardown stand down so the scene is preserved. " +
			"This halt is never lifted automatically.",
	})
}

// LiftHalt handles DELETE /admin/dispatch/halt. The lift is manual by
// construction — 03:SEC-012 「解除不得是自動的（自動解除等於讓觸發條件自己決定何時恢復
// 服務）」 — and it releases a threshold halt too, because an operator who has
// finished dealing with a leak should not have to wait out the reconciler.
//
// Idempotent: lifting nothing answers 204, since the caller's intent is already
// true.
func (h *Handler) LiftHalt(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeHaltRequest(w, r)
	if !ok {
		return
	}
	user, _ := identity.SessionUser(r.Context())
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
