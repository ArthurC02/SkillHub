package run

// RUN-007: idempotent cleanup and orphan scanning.
//
// Two facts, kept apart on purpose (ADR-004): "the run failed" lives in
// runs.status, "its sandbox was torn down" lives in runs.cleanup_status. A run can
// be finished and un-cleaned, and that combination is a system event somebody has
// to be able to see.
//
// Everything here is safely repeatable (iron rule 9). DELETE on the provider port
// is idempotent and has no 404 by contract, so a worker that crashed mid-cleanup
// simply runs the whole thing again.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

// orphanGrace is how old an unrecognised sandbox must be, measured against the
// provider's own observed_at, before it is destroyed. It is the window in which a
// dispatch is in flight: the attempt row exists but this scan read the provider's
// list before it did. Killing inside that window would murder healthy new runs.
const orphanGrace = 5 * time.Minute

// OrphanScanInterval is how often every provider is swept. Slower than the
// supervisor: a leaked sandbox costs money by the minute, not by the second, and
// each pass is a round trip to every provider.
const OrphanScanInterval = 5 * time.Minute

// CleanupArgs releases everything one run's attempts hold.
type CleanupArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (CleanupArgs) Kind() string { return "run_cleanup" }

// cleanupInsertOpts keeps one live cleanup per run. Enqueued by the terminal
// transition and again by the supervisor's backlog scan; both must land on the
// same job rather than two workers destroying the same sandbox in parallel.
func cleanupInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
		MaxAttempts: 5,
	}
}

// CleanupWorker tears down the sandboxes of a finished run.
type CleanupWorker struct {
	river.WorkerDefaults[CleanupArgs]
	Svc *Service
}

func (w *CleanupWorker) Work(ctx context.Context, job *river.Job[CleanupArgs]) error {
	var runID, workspaceID pgtype.UUID
	if err := runID.Scan(job.Args.RunID); err != nil {
		return err
	}
	if err := workspaceID.Scan(job.Args.WorkspaceID); err != nil {
		return err
	}

	run, err := w.Svc.Get(ctx, workspaceID, runID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !IsTerminal(run.Status) {
		// The run went back to work, or this job arrived before the transition it
		// belongs to. Either way there is nothing to tear down yet, and the terminal
		// transition will enqueue cleanup again.
		return nil
	}
	if run.CleanupStatus == gen.RunCleanupStatusCleaned {
		return nil // already done; repeating is safe but pointless
	}
	return w.Svc.Cleanup(ctx, run)
}

// Cleanup destroys every sandbox this run's attempts still name, and records the
// outcome. Exported so the supervisor and tests drive the same path.
func (s *Service) Cleanup(ctx context.Context, run gen.Run) error {
	defer metrics.ObserveSince(metrics.CleanupDuration, time.Now())
	// SEC-012 action ③ 「保留現場」, and the reason the halt's source matters here.
	//
	// An incident halt stands teardown down: a P1 is investigated by looking at the
	// sandbox that caused it, and destroying it is the first irreversible thing the
	// platform would otherwise do — 02:SEC-010's automatic action is 「停止派送新 Run
	// 並保留現場」, both halves.
	//
	// An X-04 threshold halt must NOT stop teardown, and this is the one place the
	// two triggers of the shared switch behave differently on purpose: cleanup is
	// exactly what clears the leaked sandboxes that raised it, so suspending it there
	// would make the halt permanent by its own action.
	halts := s.haltsFailClosed(ctx)
	if halts.incidentHeld(haltPool) {
		// Nothing is written, so cleanup_status stays whatever it was and
		// ListRunsNeedingCleanup keeps this run on the supervisor's worklist; the
		// teardown happens on the first sweep after the halt is lifted. Returning nil
		// rather than an error so River does not burn this job's attempts waiting for
		// a human (RUN-007's retries are for transient failures, and an investigation
		// is not one).
		slog.Warn("cleanup held: a P1 halt is preserving the scene", "run_id", pgconv.UUIDString(run.ID))
		return nil
	}
	if _, err := s.queries().SetRunCleanupStatus(ctx, gen.SetRunCleanupStatusParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID, CleanupStatus: gen.RunCleanupStatusCleaningUp,
	}); err != nil {
		return err
	}

	attempts, err := s.queries().ListRunAttempts(ctx, gen.ListRunAttemptsParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID,
	})
	if err != nil {
		return err
	}

	var failures []string
	// Attempts on a node held for an incident are left standing while the rest of
	// the run is released. Tracked rather than aborted, because a run can have
	// attempts on two providers and only one of them may be under investigation.
	preserved := 0
	for _, attempt := range attempts {
		// SEC-005: the model credential is revoked on every terminal outcome,
		// dispatched or not - a key can be minted and the dispatch then fail, and
		// that key would otherwise live out its TTL with a budget attached.
		// Addressed by the attempt's permanent id, so this needs nothing that was
		// held in memory and is safe to repeat (iron rule 9).
		//
		// The object grants have no revocation: a pre-signed URL is valid until it
		// expires. Their bound is the TTL minted in grantsFor, which is the run's
		// own hard wall clock plus slack.
		if s.Gateway != nil {
			if err := s.Gateway.Revoke(ctx, pgconv.UUIDString(attempt.ID)); err != nil {
				// SBX-012: counted apart from the sandbox teardown below. A key that
				// will not revoke needs a human at the gateway; draining the fleet,
				// which is what the sandbox counter escalates to, would do nothing
				// about it (ADR-022 X-03/X-04 6b).
				metrics.GatewayRevokeFailed.Inc()
				failures = append(failures, fmt.Sprintf("model gateway key for attempt %d: %v", attempt.AttemptNumber, err))
			}
		}
		if attempt.ProviderRunID == nil {
			// No handle to address a DELETE to. Usually because nothing was ever
			// provisioned; occasionally because the handle was lost between the
			// provider answering and the platform writing it down. This path cannot
			// tell the two apart and cannot act on either - the orphan scan can, and
			// isOrphan reclaims the second case by age.
			continue
		}
		// The Virtual Key above is revoked either way, and that is deliberate: it is
		// containment, not evidence. 02:SEC-010's own P1 runbook opens with 「撤銷該
		// 時間窗內所有 attempt 的 Virtual Key」, so a halt that skipped it would be
		// preserving a live credential rather than a scene.
		if halts.incidentHeld(attempt.Provider) {
			preserved++
			continue
		}
		provider := s.providers().Lookup(attempt.Provider)
		if provider == nil {
			// The provider was removed from the configuration while a sandbox of its
			// was outstanding. The platform cannot release it and must not pretend
			// otherwise: this is exactly the "cleanup failed" the operator has to see
			// (O11Y-003).
			metrics.SandboxDestroyFailed.WithLabelValues(attempt.Provider).Inc()
			failures = append(failures, "provider "+attempt.Provider+" is no longer configured")
			continue
		}
		if err := provider.Destroy(ctx, *attempt.ProviderRunID); err != nil {
			metrics.SandboxDestroyFailed.WithLabelValues(attempt.Provider).Inc()
			failures = append(failures, fmt.Sprintf("%s: %v", attempt.Provider, err))
		}
	}

	if preserved > 0 {
		// Same reasoning as the pool-wide case above: no status is written, so the run
		// stays on the supervisor's cleanup worklist and is released once the node is
		// no longer under investigation. Recording `cleaned` here would be a claim
		// that a sandbox which is still standing has been released.
		slog.Warn("cleanup partly held: a P1 halt is preserving the scene",
			"run_id", pgconv.UUIDString(run.ID), "attempts_held", preserved)
		return nil
	}
	status := gen.RunCleanupStatusCleaned
	if len(failures) > 0 {
		status = gen.RunCleanupStatusFailed
	}
	metrics.Cleanup.WithLabelValues(string(status)).Inc()
	if err := s.recordCleanup(ctx, run, status, failures); err != nil {
		return err
	}
	if len(failures) > 0 {
		// Returned so River retries. Destroy is idempotent, so retrying costs
		// nothing when the earlier calls did land.
		return fmt.Errorf("run cleanup incomplete: %s", strings.Join(failures, "; "))
	}
	return nil
}

// recordCleanup writes the cleanup outcome and the event announcing it in one
// transaction (iron rule 9).
func (s *Service) recordCleanup(ctx context.Context, run gen.Run, status gen.RunCleanupStatus, failures []string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.queries().WithTx(tx)

	updated, err := q.SetRunCleanupStatus(ctx, gen.SetRunCleanupStatusParams{
		RunID: run.ID, WorkspaceID: run.WorkspaceID, CleanupStatus: status,
	})
	if err != nil {
		return err
	}

	// The column is written on every pass — it has just been through `cleaning_up`
	// and would otherwise stay there — but the trail and the event record the edges
	// only, for the reason INGEST-010's source sweep records them:
	// ListRunsNeedingCleanup keeps everything that is not `cleaned`, so a run whose
	// sandbox can never be released (a provider dropped from the configuration is
	// the usual way) is re-enqueued by the supervisor every 30 seconds, times
	// River's five attempts. A row per pass is thousands of audit_events a day for
	// one stuck run, in the table PDM-006 §6 keeps for 400 days, saying the same
	// thing every time — and burying the two moments that do say something.
	//
	// `run` is the row the caller read before Cleanup marked it `cleaning_up`, so
	// this compares against the previous *outcome*, not against that marker.
	if run.CleanupStatus == status {
		return tx.Commit(ctx)
	}

	meta := map[string]any{"cleanup_status": string(status)}
	if len(failures) > 0 {
		meta["failure_count"] = len(failures)
	}
	payload, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	// ADR-008's CleanupCompleted, in the same shape as the status events: the
	// payload carries counts and identifiers, never a provider handle or an error
	// body that might quote one (iron rule 11).
	//
	// Mapped rather than concatenated, for the reason record() gives: `pending` and
	// `cleaning_up` are not outcomes and must not become event types, and only a
	// mapping that can refuse says so.
	//
	// causation_id stays NULL, and that is a limitation the catalogue now states
	// rather than a field nobody filled in. A cleanup pass releases *every* attempt
	// of the run, so no single attempt id is its cause; its actual cause is the
	// run_cleanup job, whose River identity is a bigint and does not fit a uuid
	// column. Threading the terminal transition's attempt through CleanupArgs would
	// fit the column and be a lie besides: it would also change the job's ByArgs
	// unique key, so the supervisor's backlog enqueue would stop coalescing with
	// this one and two workers would tear down the same sandbox in parallel.
	eventType, err := outbox.CleanupEvent(string(status))
	if err != nil {
		return err
	}
	if err := outbox.Insert(ctx, tx, outbox.NewEvent{
		EventType: eventType, EventVersion: outbox.EventVersion1,
		CorrelationID: updated.ID, WorkspaceID: updated.WorkspaceID,
		AggregateType: outbox.AggregateRun, AggregateID: updated.ID, Payload: payload,
	}); err != nil {
		return err
	}
	// NFR-001's execution trail, closing at the same place the run's resources do.
	// The outbox event above is for consumers and is not the trail: it is delivered,
	// acted on and pruned, while this row is kept for 400 days. Same transaction as
	// the status write for the usual reason (iron rule 9) — and here it also buys
	// the history runs.cleanup_status cannot keep, because that column is
	// overwritten by the next attempt and a teardown that failed twice before it
	// succeeded would otherwise look like it succeeded first time.
	//
	// Actor-less: cleanup is enqueued by a terminal transition or found by the
	// supervisor's backlog scan, never asked for by a user. `meta` is reused as-is
	// — it is already counts and a status, with the failure strings deliberately
	// left out because they can quote a provider handle (iron rule 11).
	if err := audit.Log(ctx, tx, audit.Event{
		Workspace:    updated.WorkspaceID,
		Action:       audit.ActionRunCleanup,
		ResourceType: audit.ResourceRun,
		ResourceID:   updated.ID,
		Metadata:     meta,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// --- orphan scanning ---------------------------------------------------------

// OrphanScanArgs sweeps every configured provider for sandboxes the platform no
// longer has a live attempt for.
type OrphanScanArgs struct{}

func (OrphanScanArgs) Kind() string { return "run_orphan_scan" }

// OrphanScanWorker is the comparison side of RUN-007: whatever a provider is still
// holding that the platform considers finished is a leak, and leaks cost money and
// hold slots.
type OrphanScanWorker struct {
	river.WorkerDefaults[OrphanScanArgs]
	Svc *Service
}

func (w *OrphanScanWorker) Work(ctx context.Context, _ *river.Job[OrphanScanArgs]) error {
	var failures []string
	for _, provider := range w.Svc.providers().Providers {
		if err := w.Svc.scanProvider(ctx, provider); err != nil {
			metrics.OrphanScan.WithLabelValues(provider.Name, "error").Inc()
			failures = append(failures, fmt.Sprintf("%s: %v", provider.Name, err))
			continue
		}
		metrics.OrphanScan.WithLabelValues(provider.Name, "ok").Inc()
	}
	// ADR-022 X-04, evaluated after the pass that wrote the sightings it reads, and
	// before the error return: a provider that failed to answer this round still had
	// its previous rounds counted, and the threshold is about what is sitting on the
	// fleet rather than about whether this call succeeded. It moves the same switch
	// SEC-012's P1 declaration moves (halt.go).
	w.Svc.EvaluateOrphanThresholds(ctx)
	if len(failures) > 0 {
		return errors.New("orphan scan incomplete: " + strings.Join(failures, "; "))
	}
	return nil
}

// scanProvider destroys the sandboxes one provider is holding for runs the
// platform has already finished, plus anything it does not recognise at all, and
// records what it saw in the in-flight orphan table (SBX-012, ADR-022 X-03).
//
// The sighting is recorded *before* the destroy is attempted, and a successful
// destroy does not erase it. That is the threshold's semantics, not sloppiness:
// X-03 asks whether the same resource was still present two rounds running, and a
// resource that survived a full scan interval did exactly that, whichever round
// finally killed it. What clears a sighting is the resource being gone when the
// next round looks.
//
// One failed destroy no longer aborts the pass. It used to, and that made the
// round count wrong in precisely the situation it exists for: with two leaks, the
// first one failing meant the second was never even seen, so it could sit there
// for rounds without ever reaching two.
func (s *Service) scanProvider(ctx context.Context, provider *Provider) error {
	list, err := provider.ListActive(ctx)
	if err != nil {
		return err
	}
	observed := list.ObservedAt
	if observed.IsZero() {
		observed = time.Now()
	}

	// SEC-012 action ③ again, on the last line of defence. Sightings are still
	// recorded — the X-04 count has to keep running, or the incident would silently
	// disable the capacity threshold as well — but nothing is destroyed while a P1
	// holds this node. A leaked sandbox is also the most likely piece of evidence
	// there is.
	preserveScene := s.haltsFailClosed(ctx).incidentHeld(provider.Name)

	// Empty, not nil: it is passed to ForgetClearedOrphans below even when nothing
	// leaked, and "nothing leaked" is exactly when last round's rows must go.
	stillPresent := []string{}
	var failures []string
	for _, entry := range list.Runs {
		leaked, why := s.isOrphan(ctx, entry, observed)
		if !leaked {
			continue
		}
		stillPresent = append(stillPresent, entry.ProviderRunID)
		rounds, err := s.queries().RecordOrphanSighting(ctx, gen.RecordOrphanSightingParams{
			Provider: provider.Name, ProviderRunID: entry.ProviderRunID,
		})
		if err != nil {
			// Bookkeeping failing must not stop the teardown: the sighting table is
			// how the leak gets reported, the destroy below is how it stops costing.
			slog.Error("recording orphan sighting failed", "provider", provider.Name, "error", err)
		}
		slog.Warn("destroying leaked sandbox", "provider", provider.Name, "reason", why,
			"consecutive_rounds", rounds,
			// The platform run_id, never the provider handle, is what identifies this
			// in logs and metrics (iron rule 10).
			"run_id", entry.RunID)
		if preserveScene {
			continue
		}
		// O11Y-003: a `destroyed` above zero is a leak that happened and was
		// contained; a `failed` is a leak that is still burning a slot. The alert
		// rules distinguish the two because only the second needs a human.
		if err := provider.Destroy(ctx, entry.ProviderRunID); err != nil {
			metrics.OrphanSandbox.WithLabelValues(provider.Name, "failed").Inc()
			failures = append(failures, fmt.Sprintf("destroy %s: %v", entry.RunID, err))
			continue
		}
		metrics.OrphanSandbox.WithLabelValues(provider.Name, "destroyed").Inc()
	}

	// Anything this provider held last round and does not hold now. Runs on every
	// pass including the empty one, so a handle that was cleared cannot carry its
	// round count into some future leak.
	if err := s.queries().ForgetClearedOrphans(ctx, gen.ForgetClearedOrphansParams{
		Provider: provider.Name, StillPresent: stillPresent,
	}); err != nil {
		slog.Error("pruning orphan sightings failed", "provider", provider.Name, "error", err)
	}
	persistent, err := s.queries().CountPersistentOrphans(ctx, provider.Name)
	if err != nil {
		slog.Error("counting persistent orphans failed", "provider", provider.Name, "error", err)
	} else {
		metrics.OrphanPersistent.WithLabelValues(provider.Name).Set(float64(persistent))
	}

	if len(failures) > 0 {
		return errors.New(strings.Join(failures, "; "))
	}
	return nil
}

// isOrphan decides whether one live provider-side run should be destroyed.
//
// The judgement is made against the provider's own observed_at, not against the
// platform's clock: a sandbox created after the snapshot was taken must never be
// called leaked on the strength of a list that predates it.
func (s *Service) isOrphan(ctx context.Context, entry ProviderRun, observed time.Time) (bool, string) {
	var attemptID pgtype.UUID
	if entry.RunAttemptID == "" || attemptID.Scan(entry.RunAttemptID) != nil {
		return s.orphanByAge(entry, observed, "provider run carries no platform attempt id")
	}
	attempt, err := s.queries().GetRunAttemptForReconcile(ctx, attemptID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No such attempt: either a sandbox from a database that no longer exists,
		// or one whose attempt row is younger than this snapshot. Both are judged
		// by age below.
		return s.orphanByAge(entry, observed, "no platform attempt for this sandbox")
	case err != nil:
		// The database could not answer. That is not the same statement as "there
		// is no such run", and treating it as one is how a pool timeout during a
		// busy period turns into a sweep that destroys every live sandbox older
		// than the grace window - including the long Runs the grace window exists
		// to protect. A verdict that could not be reached is not a verdict.
		slog.Error("cannot judge sandbox: attempt lookup failed",
			"run_id", entry.RunID, "error", err)
		return false, ""
	}
	if IsTerminal(attempt.Status) {
		return true, "the platform run is already finished"
	}
	// The attempt exists and has no handle, yet here is the sandbox: it *was*
	// dispatched and the write that records the handle did not land (job.go's
	// SetAttemptProviderRunID failure path, where the immediate Destroy also
	// failed). Nothing else reclaims this one. Cleanup skips a NULL
	// provider_run_id as "never dispatched", and the run's own status is no help
	// because the driver has already opened attempt 2 and carried on - so the
	// sandbox would stand, billed and holding a slot, until the *run* reached a
	// terminal state, which is up to the whole wall clock away.
	//
	// Judged by age like every other sandbox the platform cannot account for, and
	// orphanGrace is the right window unchanged: the gap this must not catch is
	// between the provider answering POST /runs and the platform's next statement,
	// one UPDATE later. Five minutes of that gap is not a dispatch in flight.
	if attempt.ProviderRunID == nil {
		return s.orphanByAge(entry, observed, "the attempt never recorded this sandbox's handle")
	}
	return false, ""
}

// orphanByAge is the guarded verdict for a sandbox the platform cannot identify.
// Only age separates "we have never heard of this" from "we are dispatching it
// right now", so unrecognised and recent is left alone.
func (s *Service) orphanByAge(entry ProviderRun, observed time.Time, why string) (bool, string) {
	if entry.CreatedAt == nil {
		return false, "" // undatable and unrecognised: not enough to kill on
	}
	if observed.Sub(*entry.CreatedAt) < orphanGrace {
		return false, ""
	}
	return true, why
}
