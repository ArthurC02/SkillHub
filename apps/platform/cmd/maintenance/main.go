package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/metrics"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("usage: maintenance purge-accounts|purge-audit|purge-feedback|" +
			"purge-run-artifacts|purge-datasets|purge-deleted-skills|purge-credit|" +
			"collect-objects|check-sources|rotate-partitions")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, purgeDatabaseURL())
	if err != nil {
		slog.Error("database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	switch os.Args[1] {
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
	case "purge-credit":
		err = purgeCredit(ctx, pool)
	case "collect-objects":
		err = collectObjects(ctx, pool)
	case "rotate-partitions":
		err = rotatePartitions(ctx, pool)
	default:
		slog.Error("unknown job", "job", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		slog.Error("maintenance job failed", "job", os.Args[1], "error", err)
		os.Exit(1)
	}
}

func purgeDatasets(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	svc := &testlab.Service{Pool: pool}
	n, err := objreconcile.PurgeExpired(ctx, pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
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
		svc.MarkDatasetPurged, nil, batch())
	intentN, intentErr := objreconcile.PurgeExpired(ctx, pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
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
		svc.MarkDatasetCleanupIntentPurged, svc.GuardDatasetObjectRemoval, batch())

	slog.Info("dataset purge complete", "datasets_purged", n, "upload_intents_purged", intentN)
	return errors.Join(err, intentErr)
}

func rotatePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	traceRetention, err := positiveDuration("TRACE_RETENTION")
	if err != nil {
		return err
	}
	analyticsRetention, err := positiveDuration("ANALYTICS_RETENTION")
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	traceReport, traceErr := trace.MaintainPartitions(ctx, pool, now, traceRetention)
	logRotation(trace.PartitionedTable, traceReport)

	analyticsReport, analyticsErr := analytics.MaintainPartitions(ctx, pool, now, analyticsRetention)
	logRotation(analytics.PartitionedTable, analyticsReport)

	return errors.Join(traceErr, analyticsErr)
}

func logRotation(table string, report partition.Report) {
	slog.Info("partitions rotated", "table", table,
		"created", report.Created, "dropped", report.Dropped)
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
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	svc := &run.Service{Pool: pool}
	n, err := objreconcile.PurgeExpired(ctx, pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
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
		svc.MarkRunOutputPurged, svc.GuardArtifactUploadIntentRemoval, batch())
	intentN, intentErr := objreconcile.PurgeExpired(ctx, pool, store,
		func(ctx context.Context, limit int32) ([]objreconcile.Candidate, error) {
			rows, err := svc.ArtifactUploadIntentCandidates(ctx, limit)
			if err != nil {
				return nil, err
			}
			out := make([]objreconcile.Candidate, len(rows))
			for i, row := range rows {
				out[i] = objreconcile.Candidate{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: row.ObjectKey}
			}
			return out, nil
		}, svc.MarkArtifactUploadIntentPurged, svc.GuardArtifactUploadIntentRemoval, batch())

	slog.Info("run artifact purge complete", "artifacts_purged", n, "upload_intents_purged", intentN)
	return errors.Join(err, intentErr)
}

func purgeDeletedSkills(ctx context.Context, pool *pgxpool.Pool) error {
	grace, err := positiveDuration("SKILL_DELETION_GRACE")
	if err != nil {
		return err
	}
	sweep, err := (&registry.Service{Pool: pool}).PurgeDeletedSkills(ctx, grace, batch())
	if err == nil {

		slog.Info("deleted skill purge complete",
			"skills_purged", sweep.Purged, "waiting", sweep.Waiting, "kept", sweep.Kept)
	}
	return err
}

func collectObjects(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	c, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, batch())

	slog.Info("orphan object collection complete",
		"objects_collected", c.Collected, "entries_dropped", c.Dropped, "queue_depth", c.Depth)
	metrics.OrphanObjectQueueDepth.Set(float64(c.Depth))
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

func purgeCredit(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := positiveDuration("CREDIT_RETENTION")
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	entries, events, err := credit.NewPostgresStore(pool).SweepExpiredRows(ctx, tx, time.Now().Add(-retention))
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("credit purge complete", "entries_removed", entries, "cost_events_removed", events)
	return nil
}

func purgeAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	svc := purgeService(pool)
	n, purgeErr := svc.PurgeExpiredAccounts(ctx, store, grace(), batch())

	slog.Info("account purge complete", "accounts_purged", n)

	sessions, sessionsErr := svc.CleanupExpiredSessions(ctx)
	if sessionsErr == nil {
		slog.Info("expired sessions removed", "sessions", sessions)
	}
	return errors.Join(purgeErr, sessionsErr)
}

func purgeService(pool *pgxpool.Pool) *identity.Service {
	ids := &identity.Service{Pool: pool}

	creditSvc := &credit.Service{Store: credit.NewPostgresStore(pool)}
	purgeCredit := func(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
		userID, err := ids.WorkspaceOwnerIn(ctx, tx, workspaceID)
		if err != nil {
			return err
		}
		return creditSvc.PurgeUser(ctx, tx, userID)
	}
	analyticsSvc := &analytics.Service{Pool: pool}
	testlabSvc := &testlab.Service{Pool: pool}
	runSvc := &run.Service{Pool: pool}
	packagingSvc := &packaging.Service{Pool: pool}
	registrySvc := &registry.Service{Pool: pool}
	ingestSvc := &ingest.Service{Pool: pool}
	return &identity.Service{
		Pool:                       pool,
		PurgeAnalytics:             analyticsSvc.PurgeWorkspace,
		PurgeTestData:              testlabSvc.PurgeWorkspace,
		PurgeRunArtifacts:          runSvc.PurgeWorkspace,
		PurgeDownloads:             packagingSvc.PurgeWorkspace,
		PurgeSkills:                registrySvc.PurgeWorkspace,
		PurgeImportSources:         ingestSvc.PurgeWorkspace,
		PurgeCreation:              (&creation.Service{Pool: pool}).PurgeWorkspace,
		PurgeCredit:                purgeCredit,
		DatasetObjectKeys:          testlabSvc.WorkspaceObjectKeys,
		RunArtifactObjectKeys:      runSvc.WorkspaceObjectKeys,
		DownloadArtifactObjectKeys: packagingSvc.WorkspaceObjectKeys,
		WorkspaceQuiescent:         runSvc.PurgeQuiescent,
	}
}

func checkSources(ctx context.Context, pool *pgxpool.Pool) error {
	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts()}}
	checked, unavailable, changed, err := svc.CheckSources(ctx, batch())
	if err != nil {
		return err
	}
	slog.Info("source check complete", "checked", checked, "unavailable", unavailable, "changed", changed)
	return nil
}

func positiveDuration(key string) (time.Duration, error) {
	d, err := time.ParseDuration(os.Getenv(key))
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration", key)
	}
	return d, nil
}

func grace() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("PURGE_GRACE")); err == nil && d > 0 {
		return d
	}
	return identity.AccountDeletionGrace
}

func purgeDatabaseURL() string {
	if url := os.Getenv("SKILLHUB_PURGE_DATABASE_URL"); url != "" {
		return url
	}
	slog.Info("SKILLHUB_PURGE_DATABASE_URL not set; purging under the API role (DATABASE_URL)")
	return os.Getenv("DATABASE_URL")
}

func batch() int32 {
	if n, err := strconv.Atoi(os.Getenv("MAINTENANCE_BATCH")); err == nil && n > 0 {
		return int32(n)
	}
	return 100
}
