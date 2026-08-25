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
//	maintenance purge-audit        PDM-006 6: remove audit events older than
//	                               AUDIT_RETENTION. Needs DATABASE_URL.
//	maintenance purge-run-artifacts
//	                               PDM-006 6 / SEC-006: remove the bytes behind
//	                               Run outputs whose expires_at has passed and
//	                               mark the rows purged. Needs DATABASE_URL and
//	                               object storage.
//	maintenance purge-datasets     PDM-006 6 / SEC-006: remove the bytes behind
//	                               uploaded datasets whose expires_at has passed
//	                               and mark the rows deleted. Needs DATABASE_URL
//	                               and object storage.
//	maintenance purge-deleted-skills
//	                               WS-005 / PDM-006 6.1: hard delete the skills a
//	                               user deleted themselves once they are past
//	                               SKILL_DELETION_GRACE, their frozen versions
//	                               included. Needs DATABASE_URL and
//	                               SKILL_DELETION_GRACE.
//	maintenance collect-objects    04 丙-73: remove the package objects whose last
//	                               referencing skill_versions row is gone. Needs
//	                               DATABASE_URL and object storage, and no
//	                               retention window at all — see below.
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
// purge-run-artifacts is the one retention job here that reads no window at all,
// and that is not an omission. The other three sweep tables with no per-row
// deadline, so the window has to be handed in and "unset" honestly means nobody
// decided. A Run output carries its own `expires_at`, written when the run
// settled and already read back by ListReadableRunArtifacts and
// CountUnreadableRunArtifacts; a window from the environment would be a second
// definition of the same date, and the first thing a mismatch does is delete
// rows another statement still calls readable. Same shape as the download
// package sweep: DOWNLOAD_ARTIFACT_RETENTION is read where the row is created,
// never where it is swept.
//
// collect-objects reads no window either, and for a third reason again. The two
// above are swept against a deadline written on the row; this one is swept
// against no deadline at all. It removes package objects that no skill_versions
// row references any more, which is a fact the database answers at sweep time,
// not a policy anybody has to ratify — so there is no variable to fail closed
// on and adding one would gate a job that deletes nothing anybody can reach.
//
// TRACE_RETENTION, ANALYTICS_RETENTION and AUDIT_RETENTION have no defaults on
// purpose: all three are PDM-006 proposals that have not been ratified, and a
// default would make this process enforce a retention nobody agreed to, by
// deleting. Unset means the job refuses to start.
//
// SKILL_DELETION_GRACE joined them on 2026-08-25 and it is the sharpest case of
// the four: PDM-006 6.1's 30 days is unratified, and what this one deletes on
// that unsigned deadline is a user's own content. So the deployment has to say
// the number out loud, and a deployment that has not is refused.
//
// AUDIT_RETENTION arrived last, on 2026-08-25, and the gap it closed was the
// other direction: 0013's column comment, its index name and the DELETE branch
// of enforce_immutable were all written for a 400 day sweep, and the sweep did
// not exist. The consent document told a participant the row goes after 400
// days; nothing deleted it, on the one table whose trigger makes deleting it
// afterwards deliberately hard.
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
		slog.Error("usage: maintenance purge-accounts|purge-analytics|purge-audit|" +
			"purge-run-artifacts|purge-datasets|purge-deleted-skills|collect-objects|" +
			"check-sources|rotate-partitions")
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
		slog.Error("unknown job", "job", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		slog.Error("maintenance job failed", "job", os.Args[1], "error", err)
		os.Exit(1)
	}
}

// purgeDatasets is purgeRunArtifacts for the other file the user put there
// themselves, and it reads no window either, for the same reason: the deadline
// is on the row. `datasets.expires_at` is stamped at upload from
// testlab.DatasetRetention, so a sweep taking its cutoff from an environment
// variable would be a second definition of a date the upload screen has already
// quoted to the person uploading.
//
// That symmetry is why there is no DATASET_RETENTION to fail closed on, and it
// is worth saying plainly rather than leaving as an apparent omission: this job
// deletes, and the thing that decides what it deletes is a column written months
// earlier by a screen that told the user the number.
//
// 0004 built the index for this sweep and named it in a comment. Nothing ran it
// until 2026-08-25, so the 90 days the upload screen and the consent form both
// promise had never once been carried out (04 丙-64) -- the third row of the
// same consent table to be caught the same way inside two days.
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
		svc.MarkDatasetPurged, batch())
	// Logged before the error is dealt with, like every other sweep here: a pass
	// that failed part way still purged the rest, and the count is what tells an
	// operator which case this was.
	slog.Info("dataset purge complete", "datasets_purged", n)
	return err
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

// purgeAudit is its own subcommand rather than a second sweep inside
// purge-analytics, even though both are "delete rows past a retention window".
// They answer to different promises with different numbers (365 days against
// 400) and a deployment must be able to run one while the other is unset --
// which is precisely what fail-closed means here, and what folding them
// together would take away.
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

// purgeRunArtifacts is SEC-006's retention half for Run outputs: the bytes of an
// expired output go, the row stays and says it expired. PDM-006 §6 and the
// consent document §3 both tell a beta participant 30 days; until this
// subcommand existed the number lived only in a column nothing acted on — the
// same shape of promise-without-a-sweeper that purge-audit closed on the audit
// table, found the same way.
//
// This subcommand's composition root. The worklist and the row write are run's,
// because `artifacts` has two owner contexts and neither may write the other's
// rows (ADR-033); the object-then-row ordering is the generic sweep's, shared
// with the download package half rather than written a second time here.
//
// Not folded into the hourly objreconcile sweep in cmd/worker, which already
// does the download half: that Service is packaging's and testlab's by
// construction, and a run-owned worklist bolted onto it would be a third
// context's rows reached through their injection points. A cron subcommand is
// also what the other three retention sweeps are.
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
		svc.MarkRunOutputPurged, batch())
	// Logged before the error is dealt with, like purgeAccounts: a pass that
	// failed part way still purged the rest, and the count is what tells an
	// operator which case this was. Bounded by MAINTENANCE_BATCH, so a backlog
	// drains over several runs — a sweep is not a migration.
	slog.Info("run artifact purge complete", "artifacts_purged", n)
	return err
}

// purgeDeletedSkills is the sweep behind WS-005's grace period. Until
// 2026-08-25 the screen that confirmed a deletion told the user, verbatim, that
// version snapshots were "retained for the 30-day grace period, then purged",
// and nothing purged them: the only hard delete of a skill took a workspace id
// and ran from account deletion alone (04 丙-63). The sentence went; this is
// what lets a deployment mean it again.
//
// Fail-closed on SKILL_DELETION_GRACE, and the reason is stronger here than for
// the other three windows: those delete the platform's records about a user,
// this one deletes the user's own content, on a deadline (PDM-006 6.1's 30 days)
// that is still unratified. Unset therefore refuses rather than picking 30 days
// -- the number has to come from whoever signed it.
//
// This subcommand's composition root: one Service, one field. The purge opens
// its own transaction, because `SET LOCAL skillhub.purge = 'on'` -- the 0005
// trigger's one exemption -- lasts exactly as long as the transaction it runs
// in; see registry.PurgeDeletedSkills.
func purgeDeletedSkills(ctx context.Context, pool *pgxpool.Pool) error {
	grace, err := positiveDuration("SKILL_DELETION_GRACE")
	if err != nil {
		return err
	}
	sweep, err := (&registry.Service{Pool: pool}).PurgeDeletedSkills(ctx, grace, batch())
	if err == nil {
		// All three numbers, not just the one that changed. A purge of 0 is the
		// normal case and says nothing on its own: `waiting` shrinking is the
		// backlog draining, `kept` standing still is the provenance rule holding
		// rows forever, correctly, and an operator asking "did it work" is asking
		// which of those two they are looking at.
		slog.Info("deleted skill purge complete",
			"skills_purged", sweep.Purged, "waiting", sweep.Waiting, "kept", sweep.Kept)
	}
	return err
}

// collectObjects is the other half of the grace purge above and of the account
// purge: the bytes. Package objects are content-addressed and shared with every
// fork, so no delete path may remove them at the moment it removes rows —
// whether an object may go is only knowable after the rows are gone, and until
// then a fork may still be reading it. `object_collection_queue` (0039) is what
// carries the key across that gap and this is what drains it.
//
// This subcommand's composition root: one Service, one store, no window. The
// missing variable is the point rather than an oversight — see the package
// comment and registry.CollectOrphanObjects.
//
// It collects nothing today. The enqueue that fills the worklist has to run
// inside each purge's own transaction and neither purge can name the skills it
// is about to take, so the producer is still unwritten (04 丙-73); the sweep
// lands first because everything it does — deciding what is unreferenced,
// sparing a fork's bytes, surviving a re-run — is what has to be right before
// anything is allowed to enqueue.
func collectObjects(ctx context.Context, pool *pgxpool.Pool) error {
	store, err := objstore.FromEnv()
	if err != nil {
		return err
	}
	c, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, batch())
	// Logged before the error is dealt with, like every other sweep here: a pass
	// that failed part way still collected the rest. `depth` is the number that
	// says whether to look — bounded by MAINTENANCE_BATCH, so a backlog draining
	// over several runs is normal and a depth that never moves is not.
	slog.Info("orphan object collection complete",
		"objects_collected", c.Collected, "entries_dropped", c.Dropped, "queue_depth", c.Depth)
	metrics.OrphanObjectQueueDepth.Set(float64(c.Depth))
	return err
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
