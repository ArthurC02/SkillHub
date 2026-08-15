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

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/services/platform/internal/audit"
	"github.com/ArthurC02/skillhub/services/platform/internal/platform/db/gen"
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
	for _, attempt := range attempts {
		if attempt.ProviderRunID == nil {
			continue // never dispatched: nothing was ever provisioned
		}
		provider := s.providers().Lookup(attempt.Provider)
		if provider == nil {
			// The provider was removed from the configuration while a sandbox of its
			// was outstanding. The platform cannot release it and must not pretend
			// otherwise: this is exactly the "cleanup failed" the operator has to see
			// (O11Y-003).
			failures = append(failures, "provider "+attempt.Provider+" is no longer configured")
			continue
		}
		if err := provider.Destroy(ctx, *attempt.ProviderRunID); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", attempt.Provider, err))
		}
	}

	status := gen.RunCleanupStatusCleaned
	if len(failures) > 0 {
		status = gen.RunCleanupStatusFailed
	}
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
	if _, err := q.InsertOutboxEvent(ctx, gen.InsertOutboxEventParams{
		EventType: "run.cleanup_" + string(status), EventVersion: 1,
		CorrelationID: updated.ID, WorkspaceID: updated.WorkspaceID,
		AggregateType: audit.ResourceRun, AggregateID: updated.ID, Payload: payload,
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
			failures = append(failures, fmt.Sprintf("%s: %v", provider.Name, err))
		}
	}
	if len(failures) > 0 {
		return errors.New("orphan scan incomplete: " + strings.Join(failures, "; "))
	}
	return nil
}

// scanProvider destroys the sandboxes one provider is holding for runs the
// platform has already finished, plus anything it does not recognise at all.
func (s *Service) scanProvider(ctx context.Context, provider *Provider) error {
	list, err := provider.ListActive(ctx)
	if err != nil {
		return err
	}
	observed := list.ObservedAt
	if observed.IsZero() {
		observed = time.Now()
	}

	for _, entry := range list.Runs {
		leaked, why := s.isOrphan(ctx, entry, observed)
		if !leaked {
			continue
		}
		slog.Warn("destroying leaked sandbox", "provider", provider.Name, "reason", why,
			// The platform run_id, never the provider handle, is what identifies this
			// in logs and metrics (iron rule 10).
			"run_id", entry.RunID)
		if err := provider.Destroy(ctx, entry.ProviderRunID); err != nil {
			return err
		}
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
	if err != nil {
		// No such attempt: either a sandbox from a database that no longer exists,
		// or one whose attempt row is younger than this snapshot.
		return s.orphanByAge(entry, observed, "no platform attempt for this sandbox")
	}
	if IsTerminal(attempt.Status) {
		return true, "the platform run is already finished"
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
