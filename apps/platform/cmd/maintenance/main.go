package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"log/slog"
	"os"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/capacity"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/publishing"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("usage: maintenance purge-accounts|purge-audit|purge-feedback|" +
			"purge-run-artifacts|purge-datasets|purge-deleted-skills|" +
			"collect-objects|check-sources|rotate-partitions|report|approved")
		os.Exit(2)
	}
	if code := runJob(os.Args[1]); code != 0 {
		os.Exit(code)
	}
}

var jobPeriods = map[string][]string{
	"daily": {"purge-accounts", "purge-run-artifacts", "purge-datasets", "purge-deleted-skills",
		"collect-objects", "check-sources", "report"},
	"weekly":   {"purge-audit", "purge-feedback"},
	"monthly":  {"rotate-partitions"},
	"frequent": {"approved"},
}

const (
	day   = 24 * time.Hour
	week  = 7 * day
	month = 31 * day

	frequently = 5 * time.Minute
)

var periodLengths = map[string]time.Duration{"daily": day, "weekly": week, "monthly": month, "frequent": frequently}

func scheduledJobs() []jobruns.Job {
	var jobs []jobruns.Job
	for period, names := range jobPeriods {
		for _, name := range names {
			jobs = append(jobs, jobruns.Job{Name: name, Period: periodLengths[period]})
		}
	}
	return jobs
}

func runJob(job string) int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, purgeDatabaseURL())
	if err != nil {
		slog.Error("database pool", "error", err)
		return 1
	}
	defer pool.Close()

	if err := jobruns.Register(ctx, pool, scheduledJobs()); err != nil {
		slog.Error("maintenance job registry", "error", err)
	}

	known, err := runExclusively(ctx, pool, job)
	if !known {
		slog.Error("unknown job", "job", job)
		return 2
	}
	if err != nil {
		slog.Error("maintenance job failed", "job", job, "error", err)
		return 1
	}
	if err := jobruns.RecordSuccess(ctx, pool, job); err != nil {
		slog.Error("maintenance job succeeded but its success was not recorded", "job", job, "error", err)
		return 1
	}
	return 0
}

func runExclusively(ctx context.Context, pool *pgxpool.Pool, job string) (known bool, err error) {
	if !slices.ContainsFunc(scheduledJobs(), func(j jobruns.Job) bool { return j.Name == job }) {
		return false, nil
	}
	err = jobruns.Exclusively(ctx, pool, job, func() error {
		_, err := runSubcommand(ctx, pool, job)
		return err
	})
	return true, err
}

func runSubcommand(ctx context.Context, pool *pgxpool.Pool, job string) (known bool, err error) {
	switch job {
	case "purge-accounts":
		err = purgeAccounts(ctx, pool)
	case "check-sources":
		err = checkSources(ctx, pool)
	case "purge-feedback":
		err = purgeFeedback(ctx, pool)
	case "purge-audit":
		err = purgeAudit(ctx, pool)
	case "purge-run-artifacts":
		err = purgeRunArtifacts(ctx, pool)
	case "purge-datasets":
		err = purgeDatasets(ctx, pool)
	case "purge-deleted-skills":
		err = purgeDeletedSkills(ctx, pool)
	case "collect-objects":
		err = collectObjects(ctx, pool)
	case "rotate-partitions":
		err = rotatePartitions(ctx, pool)
	case "report":
		err = printCapacityReport(ctx, pool)
	case "approved":
		err = runApproved(ctx, pool)
	default:
		return false, nil
	}
	return true, err
}

func runApproved(ctx context.Context, pool *pgxpool.Pool) error {
	svc := &operations.Service{Pool: pool}
	for {
		proposal, ok, err := svc.ClaimApprovedProposal(ctx)
		if err != nil || !ok {
			return err
		}
		outcome := runProposedJob(ctx, pool, proposal.Action)
		logSweep("approved "+proposal.Action, outcome)
		if err := svc.FinishProposal(ctx, proposal.ID, outcome); err != nil {
			return err
		}
	}
}

func runProposedJob(ctx context.Context, pool *pgxpool.Pool, action string) error {
	job, ok := operations.MaintenanceJobOf(action)
	if !ok || !slices.Contains(operations.ProposableMaintenanceJobs, job) {
		return fmt.Errorf("%s names no maintenance job", action)
	}
	known, err := runExclusively(ctx, pool, job)
	if !known {
		return fmt.Errorf("%s names no maintenance job", action)
	}
	if err != nil {
		return err
	}
	return jobruns.RecordSuccess(ctx, pool, job)
}

func printCapacityReport(ctx context.Context, pool *pgxpool.Pool) error {
	rate, err := capacity.ParseRestoreRate(os.Getenv(capacity.RestoreRateEnv))
	if err != nil {
		return err
	}
	now := time.Now()
	report, err := capacity.Store{Pool: pool, Rate: rate}.Report(ctx, now)
	if err != nil {
		return err
	}
	jobs, err := jobruns.List(ctx, pool, now)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(struct {
		Capacity        capacity.Report  `json:"capacity"`
		MaintenanceJobs []jobruns.Status `json:"maintenance_jobs"`
	}{report, jobs})
}

func purgeDatasets(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		return err
	}
	svc := &testlab.Service{Pool: pool, ClearSightings: objreconcile.ClearDatasetSightings}
	n, err := objreconcile.PurgeExpired(ctx, pool, store, objreconcile.RetentionOwner{
		List: func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ExpiredDatasetCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		Mark: svc.MarkDatasetPurged,
	}, wiring.MaintenanceBatch())
	intentN, intentErr := objreconcile.PurgeExpired(ctx, pool, store, objreconcile.RetentionOwner{
		List: func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.DatasetCleanupIntentCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		Mark: svc.MarkDatasetCleanupIntentPurged, Guard: svc.GuardDatasetObjectRemoval,
	}, wiring.MaintenanceBatch())

	err = errors.Join(err, intentErr)
	logSweep("dataset purge", err, "datasets_purged", n, "upload_intents_purged", intentN)
	return err
}

func rotatePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	now := time.Now().UTC()
	traceErr := rotateWithin("TRACE_RETENTION", trace.PartitionedTable, func(retention time.Duration) (partition.Report, error) {
		return trace.MaintainPartitions(ctx, pool, now, retention)
	})
	if os.Getenv("ANALYTICS_RETENTION") == "" {
		slog.Info("ANALYTICS_RETENTION not set; no funnel events are collected and existing analytics partitions are kept until it is set")
		return traceErr
	}
	analyticsErr := rotateWithin("ANALYTICS_RETENTION", analytics.PartitionedTable, func(retention time.Duration) (partition.Report, error) {
		return analytics.MaintainPartitions(ctx, pool, now, retention)
	})
	return errors.Join(traceErr, analyticsErr)
}

func rotateWithin(retentionKey, table string, maintain func(time.Duration) (partition.Report, error)) error {
	retention, err := wiring.MaintenanceDuration(retentionKey)
	if err != nil {
		return err
	}
	report, err := maintain(retention)
	logSweep(table+" partition rotation", err, "created", report.Created, "dropped", report.Dropped)
	return err
}

func purgeAudit(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := wiring.MaintenanceDuration("AUDIT_RETENTION")
	if err != nil {
		return err
	}
	n, err := audit.PurgeExpired(ctx, pool, retention)
	if err == nil {
		slog.Info("audit purge complete", "events_removed", n)
	}
	return err
}

func purgeRunArtifacts(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		return err
	}
	svc := &run.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
	n, err := objreconcile.PurgeExpired(ctx, pool, store, objreconcile.RetentionOwner{
		List: func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ExpiredArtifactCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		Mark: svc.MarkRunOutputPurged, Guard: svc.GuardArtifactUploadIntentRemoval,
	}, wiring.MaintenanceBatch())
	intentN, intentErr := objreconcile.PurgeExpired(ctx, pool, store, objreconcile.RetentionOwner{
		List: func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ArtifactUploadIntentCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		},
		Mark: svc.MarkArtifactUploadIntentPurged, Guard: svc.GuardArtifactUploadIntentRemoval,
	}, wiring.MaintenanceBatch())

	err = errors.Join(err, intentErr)
	logSweep("run artifact purge", err, "artifacts_purged", n, "upload_intents_purged", intentN)
	return err
}

func purgeDeletedSkills(ctx context.Context, pool *pgxpool.Pool) error {
	grace, err := wiring.MaintenanceDuration("SKILL_DELETION_GRACE")
	if err != nil {
		return err
	}
	sweep, err := registryPurger(pool).PurgeDeletedSkills(ctx, grace, wiring.MaintenanceBatch())
	if err == nil {

		slog.Info("deleted skill purge complete",
			"skills_purged", sweep.Purged, "waiting", sweep.Waiting, "kept", sweep.Kept)
	}
	return err
}

func collectObjects(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		return err
	}
	c, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, wiring.MaintenanceBatch())

	logSweep("orphan object collection", err,
		"objects_collected", c.Collected, "entries_dropped", c.Dropped, "queue_depth", c.Depth)
	return err
}

func purgeFeedback(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := wiring.MaintenanceDuration("FEEDBACK_RETENTION")
	if err != nil {
		return err
	}
	n, err := (&analytics.Service{Pool: pool}).PurgeExpiredFeedback(ctx, retention)
	if err == nil {
		slog.Info("feedback purge complete", "reports_removed", n)
	}
	return err
}

func purgeAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := wiring.ObjectStoreFromEnv()
	if err != nil {
		return err
	}
	grace, err := wiring.AccountPurgeGrace()
	if err != nil {
		return err
	}
	svc := purgeService(pool)
	n, purgeErr := svc.PurgeExpiredAccounts(ctx, store, grace, wiring.MaintenanceBatch())

	logSweep("account purge", purgeErr, "accounts_purged", n)

	sessions, sessionsErr := svc.CleanupExpiredSessions(ctx)
	if sessionsErr == nil {
		slog.Info("expired sessions removed", "sessions", sessions)
	}
	return errors.Join(purgeErr, sessionsErr)
}

func logSweep(sweep string, err error, counts ...any) {
	if err != nil {
		slog.Error(sweep+" stopped early; the counts are what it finished", append(counts, "error", err)...)
		return
	}
	slog.Info(sweep+" complete", counts...)
}

func registryPurger(pool *pgxpool.Pool) *registry.Service {
	return &registry.Service{
		Pool:                pool,
		VersionsInRuns:      run.SkillVersionsInRuns,
		VersionsInDownloads: packaging.SkillVersionsInDownloads,
		VersionsInBundles:   publishing.SkillVersionsInBundles,
		SkillsWithTestCases: testlab.SkillsWithTestCases,
	}
}

func purgeService(pool *pgxpool.Pool) *identity.Service {
	testlabSvc := &testlab.Service{Pool: pool, ClearSightings: objreconcile.ClearDatasetSightings}
	runSvc := &run.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
	packagingSvc := &packaging.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
	registrySvc := registryPurger(pool)
	ingestSvc := &ingest.Service{Pool: pool, SourcesInVersions: registry.SourcesInVersions}
	return &identity.Service{
		Pool:                       pool,
		PurgeAnalytics:             analytics.PurgeWorkspace,
		PurgeTestData:              testlabSvc.PurgeWorkspace,
		PurgeRunArtifacts:          runSvc.PurgeWorkspace,
		PurgeDownloads:             packagingSvc.PurgeWorkspace,
		PurgeSkills:                registrySvc.PurgeWorkspace,
		PurgeImportSources:         ingestSvc.PurgeWorkspace,
		PurgeCreation:              creation.PurgeWorkspace,
		PurgePublications:          publishing.PurgeWorkspace,
		DatasetObjectKeys:          testlab.WorkspaceObjectKeys,
		RunArtifactObjectKeys:      run.WorkspaceObjectKeys,
		DownloadArtifactObjectKeys: packaging.WorkspaceObjectKeys,
		WorkspaceQuiescent:         run.PurgeQuiescent,
	}
}

func checkSources(ctx context.Context, pool *pgxpool.Pool) error {
	svc := &ingest.Service{Pool: pool, Fetcher: wiring.ImportFetcher(wiring.PostureFromEnv())}
	sweep, err := svc.CheckSources(ctx, wiring.MaintenanceBatch())
	if err != nil {
		return err
	}
	slog.Info("source check complete", "checked", sweep.Checked, "unavailable", sweep.Unavailable, "changed", sweep.Changed)
	return nil
}

func purgeDatabaseURL() string {
	if url := os.Getenv("SKILLHUB_PURGE_DATABASE_URL"); url != "" {
		return url
	}
	slog.Info("SKILLHUB_PURGE_DATABASE_URL not set; purging under the API role (DATABASE_URL)")
	return os.Getenv("DATABASE_URL")
}
