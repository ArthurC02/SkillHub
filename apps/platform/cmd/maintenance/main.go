// Command maintenance runs the operator-invoked retention and content-health
// jobs. Same shape as cmd/reindex: bounded, idempotent, re-runnable, and
// scheduled by whatever the deployment already uses for cron. Deliberately not
// a scheduler — iron rule 6 keeps the "when" decision outside the code, and a
// second scheduler beside the queue is exactly the moving part nobody pages
// themselves for.
//
//	maintenance purge-accounts     CORE-007: hard delete the private content of
//	                               accounts past their 30-day grace period and
//	                               de-identify what has to be retained. Needs
//	                               DATABASE_URL and object storage.
//	maintenance purge-analytics    ADR-029 決策 5: remove funnel events older than
//	                               ANALYTICS_RETENTION, including the ones sitting
//	                               in the default partition. Needs DATABASE_URL.
//	maintenance check-sources      INGEST-010: probe recorded import source URLs
//	                               and mark the ones that no longer resolve.
//	                               Needs DATABASE_URL and network egress.
//	maintenance rotate-partitions  Keep trace_events and analytics_events'
//	                               monthly partitions in step: pre-create the
//	                               months about to be written to, drop the ones
//	                               past retention. Needs DATABASE_URL,
//	                               TRACE_RETENTION and ANALYTICS_RETENTION.
//
// PURGE_GRACE (Go duration, default 720h) and MAINTENANCE_BATCH (default 100)
// tune one run. A shortened grace applies to requests already in flight.
//
// TRACE_RETENTION and ANALYTICS_RETENTION have no defaults on purpose: both are
// PDM-006 proposals that have not been ratified, and a default would make this
// process enforce a retention nobody agreed to, by deleting. Unset means the job
// refuses to start.
//
// This process has one composition root per subcommand — the function that runs
// it — and that is the shape, not an oversight: each job builds the single
// Service it needs and reads only the configuration that service uses, so
// check-sources runs on a deployment whose object storage is misconfigured and
// purge-accounts refuses to start on one whose storage it cannot reach. A shared
// root would make every job depend on every job's configuration. (ADR-032 §5:
// apiserver.NewApp is the API's root, not the platform's.)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/partition"
"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
)

func main() {
	if len(os.Args) != 2 {
		slog.Error("usage: maintenance purge-accounts|purge-analytics|check-sources|rotate-partitions")
		os.Exit(2)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
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
	case "purge-analytics":
		err = purgeAnalytics(ctx, pool)
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

// rotatePartitions is this subcommand's composition root. Both partitioned
// tables are rolled by one invocation because they need the same thing at the
// same cadence and a deployment that wires up one cron entry and forgets the
// other has a silent hole; the DDL for each still belongs to its owner, which is
// why this function names two packages and no table.
//
// Both windows are read before any statement runs. Fail-closed is the whole
// point: an unset window is not "use a sensible default", it is "this deployment
// has not decided what to delete", and this job deletes.
func rotatePartitions(ctx context.Context, pool *pgxpool.Pool) error {
	traceRetention, err := positiveDuration("TRACE_RETENTION")
	if err != nil {
		return err
	}
	analyticsRetention, err := positiveDuration("ANALYTICS_RETENTION")
	if err != nil {
		return err
	}
	// One `now` for both, so a run that straddles midnight on the first of a
	// month does not give the two tables different ideas of which month it is.
	now := time.Now().UTC()

	traceReport, traceErr := trace.MaintainPartitions(ctx, pool, now, traceRetention)
	logRotation(trace.PartitionedTable, traceReport)
	// analytics runs even when trace failed, for the reason purgeAccounts runs
	// its second sweep: the two tables have nothing to do with each other, and
	// returning early would make one table's stuck month quietly suspend the
	// other table's retention.
	analyticsReport, analyticsErr := analytics.MaintainPartitions(ctx, pool, now, analyticsRetention)
	logRotation(analytics.PartitionedTable, analyticsReport)

	return errors.Join(traceErr, analyticsErr)
}

// logRotation prints what actually happened, including the common case of
// nothing: "created=[] dropped=[]" on a re-run is the evidence the job is
// idempotent, and it is what an operator needs to see the month it stops being
// idempotent.
func logRotation(table string, report partition.Report) {
	slog.Info("partitions rotated", "table", table,
		"created", report.Created, "dropped", report.Dropped)
}

func purgeAnalytics(ctx context.Context, pool *pgxpool.Pool) error {
	retention, err := positiveDuration("ANALYTICS_RETENTION")
	if err != nil {
		return err
	}
	n, err := (&analytics.Service{Pool: pool, Retention: retention}).PurgeExpired(ctx)
	if err == nil {
		slog.Info("analytics purge complete", "events_removed", n)
	}
	return err
}

func purgeAccounts(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	svc := purgeService(pool)
	n, purgeErr := svc.PurgeExpiredAccounts(ctx, store, grace(), batch())
	// Logged before the error is dealt with: a batch that failed on some accounts
	// still purged the rest, and the count is what tells an operator which case
	// this was.
	slog.Info("account purge complete", "accounts_purged", n)

	// Expired sessions are the other retention sweep this command owns; it is
	// one statement and already idempotent (ADR-020). It runs even when accounts
	// failed, because the two sweeps have nothing to do with each other and
	// returning early here would make one account's failure silently skip the
	// other sweep as well - the same shape of quiet omission as the swallowed
	// error above.
	sessions, sessionsErr := svc.CleanupExpiredSessions(ctx)
	if sessionsErr == nil {
		slog.Info("expired sessions removed", "sessions", sessions)
	}
	return errors.Join(purgeErr, sessionsErr)
}

// purgeService is this subcommand's slice of the composition root, split out of
// purgeAccounts only so main_test.go can check it without a database or object
// storage. This process, not the API, is what actually runs the purge, so this
// is where the six owning contexts' steps have to be handed over: each context
// decides what an account deletion means for its own rows, and identity owns
// only the transaction they share (ADR-034). A step left out here is refused,
// not skipped — see identity.requirePurgeSteps.
func purgeService(pool *pgxpool.Pool) *identity.Service {
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
		DatasetObjectKeys:          testlabSvc.WorkspaceObjectKeys,
		RunArtifactObjectKeys:      runSvc.WorkspaceObjectKeys,
		DownloadArtifactObjectKeys: packagingSvc.WorkspaceObjectKeys,
	}
}

func checkSources(ctx context.Context, pool *pgxpool.Pool) error {
	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{Allowed: ingest.DefaultAllowedHosts()}}
	checked, unavailable, err := svc.CheckSources(ctx, batch())
	if err != nil {
		return err
	}
	slog.Info("source check complete", "checked", checked, "unavailable", unavailable)
	return nil
}

// positiveDuration reads a retention window that has no default. The error names
// the variable rather than the job, because the operator's next action is to set
// it and the job name is already on the failure line main prints.
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

func batch() int32 {
	if n, err := strconv.Atoi(os.Getenv("MAINTENANCE_BATCH")); err == nil && n > 0 {
		return int32(n)
	}
	return 100
}
