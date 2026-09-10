package run

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

const orphanGrace = 5 * time.Minute

const OrphanScanInterval = 5 * time.Minute

type CleanupArgs struct {
	RunID       string `json:"run_id"`
	WorkspaceID string `json:"workspace_id"`
}

func (CleanupArgs) Kind() string { return "run_cleanup" }

func cleanupInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: liveJobStates},
		MaxAttempts: 5,
	}
}

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

		return nil
	}
	if run.CleanupStatus == gen.RunCleanupStatusCleaned {
		return nil
	}
	return w.Svc.Cleanup(ctx, run)
}

func (s *Service) Cleanup(ctx context.Context, run gen.Run) error {
	defer metrics.ObserveSince(metrics.CleanupDuration, time.Now())

	halts := s.haltsFailClosed(ctx)
	if halts.incidentHeld(haltPool) {

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

	s.settleCredit(ctx, run, attempts)

	var failures []string

	preserved := 0
	for _, attempt := range attempts {

		if s.Gateway != nil {
			if err := s.Gateway.Revoke(ctx, pgconv.UUIDString(attempt.ID)); err != nil {

				metrics.GatewayRevokeFailed.Inc()
				failures = append(failures, fmt.Sprintf("model gateway key for attempt %d: %v", attempt.AttemptNumber, err))
			}
		}
		if attempt.ProviderRunID == nil {

			continue
		}

		if halts.incidentHeld(attempt.Provider) {
			preserved++
			continue
		}
		provider := s.providers().Lookup(attempt.Provider)
		if provider == nil {

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

		return fmt.Errorf("run cleanup incomplete: %s", strings.Join(failures, "; "))
	}
	return nil
}

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

type OrphanScanArgs struct{}

func (OrphanScanArgs) Kind() string { return "run_orphan_scan" }

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

	w.Svc.EvaluateOrphanThresholds(ctx)
	if len(failures) > 0 {
		return errors.New("orphan scan incomplete: " + strings.Join(failures, "; "))
	}
	return nil
}

func (s *Service) scanProvider(ctx context.Context, provider *Provider) error {
	list, err := provider.ListActive(ctx)
	if err != nil {
		return err
	}
	observed := list.ObservedAt
	if observed.IsZero() {
		observed = time.Now()
	}

	preserveScene := s.haltsFailClosed(ctx).incidentHeld(provider.Name)

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

			slog.Error("recording orphan sighting failed", "provider", provider.Name, "error", err)
		}
		slog.Warn("destroying leaked sandbox", "provider", provider.Name, "reason", why,
			"consecutive_rounds", rounds,

			"run_id", entry.RunID)
		if preserveScene {
			continue
		}

		if err := provider.Destroy(ctx, entry.ProviderRunID); err != nil {
			metrics.OrphanSandbox.WithLabelValues(provider.Name, "failed").Inc()
			failures = append(failures, fmt.Sprintf("destroy %s: %v", entry.RunID, err))
			continue
		}
		metrics.OrphanSandbox.WithLabelValues(provider.Name, "destroyed").Inc()
	}

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

func (s *Service) isOrphan(ctx context.Context, entry ProviderRun, observed time.Time) (bool, string) {
	var attemptID pgtype.UUID
	if entry.RunAttemptID == "" || attemptID.Scan(entry.RunAttemptID) != nil {
		return s.orphanByAge(entry, observed, "provider run carries no platform attempt id")
	}
	attempt, err := s.queries().GetRunAttemptForReconcile(ctx, attemptID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):

		return s.orphanByAge(entry, observed, "no platform attempt for this sandbox")
	case err != nil:

		slog.Error("cannot judge sandbox: attempt lookup failed",
			"run_id", entry.RunID, "error", err)
		return false, ""
	}
	if IsTerminal(attempt.Status) {
		return true, "the platform run is already finished"
	}

	if attempt.ProviderRunID == nil {
		return s.orphanByAge(entry, observed, "the attempt never recorded this sandbox's handle")
	}
	return false, ""
}

func (s *Service) orphanByAge(entry ProviderRun, observed time.Time, why string) (bool, string) {
	if entry.CreatedAt == nil {
		return false, ""
	}
	if observed.Sub(*entry.CreatedAt) < orphanGrace {
		return false, ""
	}
	return true, why
}
