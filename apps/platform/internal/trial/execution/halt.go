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
	if err := audit.Log(ctx, tx, audit.Event{
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

// operatorUser is the dispatch handlers' own check that a session actually
// reached them — the second line of defence the authz matrix says every private
// handler has. Until 2026-08-25 the two writes below took `user, _` and carried
// on with a zero UUID, so a wrapper weakened to RequireSession, or a new
// operator route mounted without one, would not have refused the halt: it would
// have stopped the fleet and recorded 02:SEC-011's 「誰做的」 as a user that does
// not exist. The read did not look at the session at all.
//
// 404, the same answer RequireOperator gives everybody else, so the second check
// does not disclose the endpoint the first one hides (SEC-011 不揭露端點存在).
func operatorUser(w http.ResponseWriter, r *http.Request) (identity.User, bool) {
	user, ok := identity.SessionUser(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not found")
		return user, false
	}
	return user, true
}

// Halts handles GET /admin/dispatch. 03:SEC-012's whole objection to two switches
// is that 「現在到底有沒有在派送」 stops having a single answer; this is where that
// answer is read.
func (h *Handler) Halts(w http.ResponseWriter, r *http.Request) {
	if _, ok := operatorUser(w, r); !ok {
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
	user, ok := operatorUser(w, r)
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
	user, ok := operatorUser(w, r)
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

// --- the two P1 criteria this platform can see for itself ---------------------
//
// 02:SEC-010's P1 row lists five criteria and 03:SEC-012's verb is 「偵測到」, not
// 「被宣告」. Three of the five are not knowable from inside this process — 逃逸疑慮
// is a judgement, the P-02 probe and the gVisor advisory cron run elsewhere — and
// they keep the operator endpoint above as their entry point. The other two are
// signals the platform already measures, and they are wired here.
//
// Both write the same switch as the operator endpoint and the X-04 reconciler, and
// neither has a matching automatic lift. That asymmetry is the requirement rather
// than an omission: 03:SEC-012 「解除不得是自動的（自動解除等於讓觸發條件自己決定何時
// 恢復服務）」.

// maskingWindow is 02:SEC-010's `TraceMaskingStopped` criterion measured with
// infra/observability/alerts.yml's own numbers, both of them: that rule reads its
// two halves over `[1h]` and then holds them `for: 1h`, and the P1 row names the
// criterion by the rule's name, so a different figure here would mean the alert and
// the halt disagreed about what "stopped" means.
//
// Both, and not just the expression, because `for` is part of this threshold rather
// than pager hygiene layered on top. The rule's premise — 「正常流量下 tool_call 的
// arguments 與 script_log 的 message 幾乎必然有東西被遮」 — is a claim about volume,
// and a quiet hour that legitimately carried nothing worth redacting satisfies the
// expression on its own. 02:SEC-010's escalation rule 「不確定屬 P1 或 P2 時一律以 P1
// 處理」 says which way to resolve an event you have; it is not licence to lower the
// bar for having one, and halting the fleet is only cheap while it is rare.
//
// maskingConfirm is the `for`, expressed as "there was traffic on the far side of
// the window too". Continuously true for an hour over a rolling hour is exactly
// traffic at both ends with no redaction anywhere between, which one aggregate
// answers without keeping state between sweeps.
const (
	maskingWindow  = time.Hour
	maskingConfirm = time.Hour
)

// reconcilerStallWindow is 02:SEC-010's fifth criterion verbatim — 「Reconciler
// 停擺 > 10 分鐘（ADR-022 X-02）」 — which is also the window of alerts.yml's
// OrphanScanNotRunning. X-02 sweeps every five minutes and two missed rounds is
// ten.
//
// That rule's own `for: 10m` is NOT added on top, and the asymmetry with
// maskingConfirm above is the difference between the two signals rather than an
// inconsistency. 「停擺 > 10 分鐘」 is written into 02:SEC-010's P1 row as the
// criterion itself, and the evidence is not statistical: either a row says the scan
// ran inside the window or no row does. There is no quiet-hour reading of it to
// guard against.
const reconcilerStallWindow = 10 * time.Minute

// reconcilerLastRun asks River when the orphan scan last got as far as running.
//
// Raw SQL and not a generated query because river_job is River's own table, applied
// by queue.EnsureSchema rather than by db/migrations, so sqlc cannot see it (see the
// header of 0016). No new migration either: 「上次掃描時間」 already exists here, and
// a second copy of it would be a second thing that can be wrong.
//
// coalesce(finalized_at, attempted_at) is the row's last sign of life, and the pair
// is what makes this agree with the counter the alert reads: skillhub_orphan_scan_total
// is incremented per provider whether that provider answered or not, so a round that
// ended in an error still proves the reconciler is alive. Keying on finalized_at
// alone would call a spell of provider flakiness — a P2 — a P1.
const reconcilerLastRun = `SELECT max(coalesce(finalized_at, attempted_at)) FROM river_job WHERE kind = $1`

// detectMaskingStopped is `TraceMaskingStopped` as a decision instead of a notice:
// events kept landing and the masker redacted nothing across two hours of them,
// which under 0019's `CHECK (masked)` is the only shape a masking failure can still
// take — the rules stopped matching while every row went on claiming to be masked.
// That is iron rule 11 failing silently, and NFR-002 has no other detector for it.
//
// Called at the end of one supervisor sweep, so it costs one aggregate every 30
// seconds over a bounded slice of one partition.
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
	// Nothing coming in is not a masker that stopped; it is a platform with nothing
	// to mask. Traffic at both ends and no redaction between them is the rule's
	// expression and its `for` together — see maskingWindow for why both.
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

// detectMaskerCanaryFailed is the same P1 criterion asked directly instead of
// inferred: the platform makes up a secret of every shape the masker knows, runs
// it through the masker, and stops the fleet if any of them came back intact.
//
// Why both this and detectMaskingStopped, rather than this one replacing it.
// They are not two measurements of one thing — they fail in different places and
// neither sees the other's failure:
//
//   - The canary reads the RULES, in this process, with no traffic. It is the only
//     one of the two that works on the corpus this deployment actually has: 2,444
//     sandbox events with a masked total of zero, because synthetic data carries no
//     secrets (runbook §5). The traffic-based criterion has never been evaluable
//     here and would not be until real beta traffic arrived, which is not a
//     detector, it is a promise of one.
//   - The traffic criterion reads the PATH, in production, over what really
//     happened. The canary calls Masker.Mask itself, so it cannot see a caller that
//     stopped calling the masker — and there are two such callers ([Service.Ingest]
//     and RecordOrchestratorEvent, each constructing its own Masker), plus the
//     grant of Known values that Ingest passes and the canary supplies for itself.
//     Delete the mask step from an ingestion path and this canary stays green
//     forever; only the count of redactions over real traffic falls to zero.
//
// So: the canary answers 「遮罩器還活著嗎」 and the traffic criterion answers
// 「有一整批不該是零的東西是零嗎」. Removing the second one would trade a detector
// that is currently unevaluable for no detector at all on the wiring, and the
// thresholds it is measured with are 02:SEC-010's to change, not this file's.
//
// The failure is not rate-limited or retried: the masker is a pure function of
// compiled-in rules, so it does not flake. One intact sample is a deployment
// running with iron rule 11 partly disabled, and 02:SEC-010's escalation rule
// (「不確定屬 P1 或 P2 時一律以 P1 處理」) settles the rest.
func (s *Service) detectMaskerCanaryFailed(ctx context.Context) {
	// The probe first, and the halt table only if it failed: this rides a 30 second
	// sweep, and the healthy case must not cost a query. detectMaskingStopped can
	// afford to check first because it has to read the database either way.
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

// maskerCanary is trace.MaskerCanary, behind a field so an integration test can
// simulate a masker that stopped — there is no other seam, because the real one is
// a pure function over compiled-in rules and a test cannot break those from
// outside the package. Nil means the real probe: a composition root that never
// heard of this field still gets the detector, which is the direction a
// fail-closed switch has to default in.
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

// DetectReconcilerStall is 02:SEC-010's 「Reconciler 停擺 > 10 分鐘」.
//
// Exported because its caller is cmd/api, and cmd/api is the caller for the reason
// the criterion exists at all: a reconciler that has stopped cannot be the thing
// that notices it stopped, and River's periodic jobs are precisely what stops when a
// worker dies. Hanging this off the supervisor would give the detector the same fate
// as the thing it watches.
func (s *Service) DetectReconcilerStall(ctx context.Context) {
	if s.incidentAlreadyHeld(ctx) {
		return
	}
	var last pgtype.Timestamptz
	if err := s.Pool.QueryRow(ctx, reconcilerLastRun, OrphanScanArgs{}.Kind()).Scan(&last); err != nil {
		slog.Error("could not read when the orphan reconciler last ran", "error", err)
		return
	}
	// Never run at all is not a stall: it is a deployment whose worker has not
	// started yet, and it is the same case alerts.yml gets for free — a counter that
	// has never been observed has no series, so OrphanScanNotRunning does not fire
	// on it either. A worker that never starts shows up as nothing being dispatched.
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

// WatchReconciler polls DetectReconcilerStall until ctx is done. Started by cmd/api.
//
// It ticks at the scan's own interval so that no second cadence is invented; the
// price is up to one interval of lag on top of reconcilerStallWindow, the same order
// as the alert's own evaluation delay. The first tick is one interval in rather than
// immediate, because at start-up the worker may still be booting and 「上次掃描是很久
// 以前」 would then be a statement about the restart, not about a reconciler that
// stopped.
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

// incidentAlreadyHeld is what makes both detectors idempotent. DeclareHalt is an
// upsert, so re-declaring does not stack a second halt — but it does write a second
// audit event and reset clear_rounds, and a criterion that stays true for the hours
// an investigation takes would otherwise write one of each per sweep and bury the
// declaration that mattered under its own repetitions.
//
// An unreadable table counts as held, which is not a reversal of the fail-closed
// rule this file follows elsewhere: every decision path already refuses to dispatch
// while the table cannot be read (haltsFailClosed, requireDispatchable), so the fleet
// is already stopped. All that is left to decide is whether to write a row, and
// writing one per sweep on the strength of a read that failed is trail noise, not
// safety.
func (s *Service) incidentAlreadyHeld(ctx context.Context) bool {
	state, err := s.activeHalts(ctx)
	if err != nil {
		slog.Error("dispatch halt state unreadable; skipping automatic P1 detection", "error", err)
		return true
	}
	return state.incidentHeld(haltPool)
}

// declareDetectedIncident flips the switch for a criterion the platform concluded on
// its own. The actor is the zero UUID, the same convention the X-04 reconciler uses,
// so the trail can tell a P1 the platform declared from one an operator did — and the
// lift is a person either way.
func (s *Service) declareDetectedIncident(ctx context.Context, reason string) {
	if _, err := s.DeclareHalt(ctx, haltPool, HaltSourceIncident, reason, pgtype.UUID{}); err != nil {
		slog.Error("a P1 criterion was met but dispatch could not be stopped", "reason", reason, "error", err)
		return
	}
	slog.Error("P1 detected: dispatch stopped fleet-wide and the scene is preserved; "+
		"this halt is never lifted automatically", "reason", reason)
}
