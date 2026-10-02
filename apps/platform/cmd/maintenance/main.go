package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/runtime/envx"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
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
			"collect-objects|check-sources|rotate-partitions")
		os.Exit(2)
	}
	if code := runJob(os.Args[1]); code != 0 {
		os.Exit(code)
	}
}

func runJob(job string) int {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, purgeDatabaseURL())
	if err != nil {
		slog.Error("database pool", "error", err)
		return 1
	}
	defer pool.Close()

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
	default:
		slog.Error("unknown job", "job", job)
		return 2
	}
	if err != nil {
		slog.Error("maintenance job failed", "job", job, "error", err)
		return 1
	}
	return 0
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
	}, batch())
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
	}, batch())

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
	retention, err := positiveDuration(retentionKey)
	if err != nil {
		return err
	}
	report, err := maintain(retention)
	logSweep(table+" partition rotation", err, "created", report.Created, "dropped", report.Dropped)
	return err
}

func purgeAudit(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := positiveDuration("AUDIT_RETENTION")
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
	}, batch())
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
	}, batch())

	err = errors.Join(err, intentErr)
	logSweep("run artifact purge", err, "artifacts_purged", n, "upload_intents_purged", intentN)
	return err
}

func purgeDeletedSkills(ctx context.Context, pool *pgxpool.Pool) error {
	grace, err := positiveDuration("SKILL_DELETION_GRACE")
	if err != nil {
		return err
	}
	sweep, err := registryPurger(pool).PurgeDeletedSkills(ctx, grace, batch())
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
	c, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, batch())

	logSweep("orphan object collection", err,
		"objects_collected", c.Collected, "entries_dropped", c.Dropped, "queue_depth", c.Depth)
	return err
}

func purgeFeedback(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := positiveDuration("FEEDBACK_RETENTION")
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
	grace, err := accountPurgeGrace()
	if err != nil {
		return err
	}
	svc := purgeService(pool)
	n, purgeErr := svc.PurgeExpiredAccounts(ctx, store, grace, batch())

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
	sweep, err := svc.CheckSources(ctx, batch())
	if err != nil {
		return err
	}
	slog.Info("source check complete", "checked", sweep.Checked, "unavailable", sweep.Unavailable, "changed", sweep.Changed)
	return nil
}

func positiveDuration(key string) (time.Duration, error) {
	d, err := time.ParseDuration(os.Getenv(key))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return d, nil
}

func accountPurgeGrace() (time.Duration, error) {
	raw := os.Getenv("PURGE_GRACE")
	if raw == "" {
		return identity.AccountDeletionGrace, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < identity.AccountDeletionGrace {
		return 0, fmt.Errorf("PURGE_GRACE must be a Go duration of at least %s, the grace a deleting account is promised", identity.AccountDeletionGrace)
	}
	return d, nil
}

func purgeDatabaseURL() string {
	if url := os.Getenv("SKILLHUB_PURGE_DATABASE_URL"); url != "" {
		return url
	}
	slog.Info("SKILLHUB_PURGE_DATABASE_URL not set; purging under the API role (DATABASE_URL)")
	return os.Getenv("DATABASE_URL")
}

func batch() int32 {
	return envx.PositiveInt32("MAINTENANCE_BATCH", 100)
}
