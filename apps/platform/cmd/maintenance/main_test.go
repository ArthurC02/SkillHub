package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/jobruns"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/operations"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestPurgeServiceCarriesEveryContextsStep(t *testing.T) {
	svc := reflect.ValueOf(*purgeService(nil))
	for i := range svc.NumField() {
		field := svc.Field(i)
		if (field.Type() == reflect.TypeFor[identity.WorkspacePurge]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceObjectKeys]() ||
			field.Type() == reflect.TypeFor[identity.WorkspaceQuiescence]()) && field.IsNil() {
			t.Errorf("identity.Service.%s is nil: purge-accounts would refuse to run",
				svc.Type().Field(i).Name)
		}
	}
}

func recordPartitionedTablesInMigration(body string, partitioned map[string]bool) {
	var table string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "--") {
			continue
		}

		if fields := strings.Fields(line); len(fields) >= 3 &&
			fields[0] == "CREATE" && fields[1] == "TABLE" && !strings.Contains(line, "PARTITION OF") {
			table = fields[2]
		}
		if strings.Contains(line, "PARTITION BY RANGE") && table != "" {
			partitioned[table] = true
		}
	}
}

func partitionedTablesDeclaredInMigrations(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	partitioned := map[string]bool{}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		recordPartitionedTablesInMigration(string(body), partitioned)
	}
	return partitioned
}

func TestEveryPartitionedTableIsRotated(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "db", "migrations")
	partitioned := partitionedTablesDeclaredInMigrations(t, dir)

	rotated := map[string]bool{
		trace.PartitionedTable:     true,
		analytics.PartitionedTable: true,
	}
	for table := range partitioned {
		if !rotated[table] {
			t.Errorf("db/migrations declares %s PARTITION BY RANGE but rotate-partitions never touches it", table)
		}
	}
	for table := range rotated {
		if !partitioned[table] {
			t.Errorf("rotate-partitions names %s but no migration declares it PARTITION BY RANGE", table)
		}
	}
}

func TestRetentionWindowsHaveNoDefault(t *testing.T) {
	for _, unusable := range []string{"", "90", "0s", "-24h", "ninety days"} {
		t.Setenv("TRACE_RETENTION", unusable)
		if _, err := wiring.MaintenanceDuration("TRACE_RETENTION"); err == nil {
			t.Errorf("TRACE_RETENTION=%q accepted", unusable)
		}
	}
	t.Setenv("TRACE_RETENTION", "2160h")
	if d, err := wiring.MaintenanceDuration("TRACE_RETENTION"); err != nil || d != 2160*time.Hour {
		t.Errorf("positiveDuration = %s, %v", d, err)
	}
}

func TestPurgeDeletedSkillsRefusesWithoutAGracePeriod(t *testing.T) {
	for _, unusable := range []string{"", "30", "0s", "-720h", "thirty days"} {
		t.Setenv("SKILL_DELETION_GRACE", unusable)
		err := purgeDeletedSkills(context.Background(), nil)
		if err == nil {
			t.Errorf("SKILL_DELETION_GRACE=%q started the purge", unusable)
			continue
		}

		if !strings.Contains(err.Error(), "SKILL_DELETION_GRACE") {
			t.Errorf("SKILL_DELETION_GRACE=%q: error does not name the variable: %v", unusable, err)
		}
	}
}

func TestPurgeDatabaseURLFallsBackToTheAPIRole(t *testing.T) {
	t.Setenv("SKILLHUB_PURGE_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://api-role@db/skillhub")
	if got := purgeDatabaseURL(); got != "postgres://api-role@db/skillhub" {
		t.Errorf("purgeDatabaseURL() = %q, want DATABASE_URL's value", got)
	}

	t.Setenv("SKILLHUB_PURGE_DATABASE_URL", "postgres://skillhub_purge@db/skillhub")
	if got := purgeDatabaseURL(); got != "postgres://skillhub_purge@db/skillhub" {
		t.Errorf("purgeDatabaseURL() = %q, want SKILLHUB_PURGE_DATABASE_URL's value", got)
	}
}

func TestPurgeFeedbackRefusesWithoutARetentionWindow(t *testing.T) {
	for _, unusable := range []string{"", "180", "0s", "-4320h", "six months"} {
		t.Setenv("FEEDBACK_RETENTION", unusable)
		err := purgeFeedback(context.Background(), nil)
		if err == nil {
			t.Errorf("FEEDBACK_RETENTION=%q started the purge", unusable)
			continue
		}
		if !strings.Contains(err.Error(), "FEEDBACK_RETENTION") {
			t.Errorf("FEEDBACK_RETENTION=%q: error does not name the variable: %v", unusable, err)
		}
	}
}

func TestTheDeletionSweepCarriesEveryReferenceRead(t *testing.T) {
	svc := reflect.ValueOf(*registryPurger(nil))
	for i := range svc.NumField() {
		field := svc.Field(i)
		if field.Type() == reflect.TypeFor[registry.ReferenceRead]() && field.IsNil() {
			t.Errorf("registry.Service.%s is nil: the deleted-skill sweep and account purge would refuse to run",
				svc.Type().Field(i).Name)
		}
	}
}

type queryFailingTx struct{ pgx.Tx }

var errNoDatabaseHere = errors.New("no database in this test")

func (queryFailingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errNoDatabaseHere
}

func (queryFailingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errNoDatabaseHere
}

func TestAccountPurgeAsksRegistryWhichImportSourcesAreStillUsed(t *testing.T) {
	err := purgeService(nil).PurgeImportSources(context.Background(), queryFailingTx{}, pgtype.UUID{})
	if !errors.Is(err, errNoDatabaseHere) {
		t.Errorf("the ingest purge step stopped before reading its sources: %v", err)
	}
}

func TestAnApprovedProposalThatNamesNoJobFailsWithoutRunningAnything(t *testing.T) {
	for _, action := range []string{"run-purge-everything", "run-approved", "purge-audit", "run-"} {
		err := runProposedJob(context.Background(), nil, action)
		if err == nil || !strings.Contains(err.Error(), "names no maintenance job") {
			t.Errorf("%s: %v, want it refused as naming no job", action, err)
		}
	}
}

func TestEveryJobAnAgentMayProposeIsAScheduledJob(t *testing.T) {
	for _, job := range operations.ProposableMaintenanceJobs {
		if !slices.ContainsFunc(scheduledJobs(), func(j jobruns.Job) bool { return j.Name == job }) {
			t.Errorf("%s may be proposed but maintenance does not run it", job)
		}
	}
}

func TestASubcommandOutsideTheSwitchIsAnErrorNotASilentSuccess(t *testing.T) {
	if err := runSubcommand(context.Background(), nil, "purge-everything"); err == nil {
		t.Error("an unknown subcommand returned no error")
	}
}

func TestEachReconcileRowBecomesTheCandidateItNamesWithinTheBatch(t *testing.T) {
	row := run.ReconcileCandidate{
		ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, WorkspaceID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true}, ObjectKey: "runs/a",
	}
	var asked int32
	list := asCandidates(func(_ context.Context, limit int32) ([]run.ReconcileCandidate, error) {
		asked = limit
		return []run.ReconcileCandidate{row}, nil
	})
	got, err := list(context.Background(), 7)
	want := []objreconcile.Candidate{{ID: row.ID, WorkspaceID: row.WorkspaceID, ObjectKey: "runs/a"}}
	if err != nil || asked != 7 || !reflect.DeepEqual(got, want) {
		t.Errorf("candidates %+v (%v) with limit %d, want %+v with limit 7", got, err, asked, want)
	}

	failing := asCandidates(func(context.Context, int32) ([]run.ReconcileCandidate, error) {
		return nil, errors.New("list failed")
	})
	if _, err := failing(context.Background(), 7); err == nil || err.Error() != "list failed" {
		t.Errorf("a failing list returned %v, want its error", err)
	}
}

type untouchedStore struct{}

func (untouchedStore) Exists(context.Context, string) (bool, error) {
	return false, errors.New("unexpected")
}
func (untouchedStore) Remove(context.Context, string) error { return errors.New("unexpected") }

func TestTheIntentPassRunsEvenWhenTheExpiredPassFails(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	pool, err := pgxpool.New(context.Background(), strings.TrimPrefix(unreachableMaintenanceDatabase, "DATABASE_URL="))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	mark := func(context.Context, pgx.Tx, pgtype.UUID) error { return nil }
	intentsListed := false
	passes := retentionPasses{
		sweep: "test purge", purgedKey: "things_purged",
		expired: objreconcile.RetentionOwner{Mark: mark, List: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return nil, errors.New("expired list failed")
		}},
		intents: objreconcile.RetentionOwner{Mark: mark, List: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			intentsListed = true
			return nil, errors.New("intent list failed")
		}},
	}

	err = passes.run(context.Background(), pool, untouchedStore{})
	if !intentsListed || err == nil || !strings.Contains(err.Error(), "expired list failed") || !strings.Contains(err.Error(), "intent list failed") {
		t.Errorf("err %v, intents listed %v; want both passes run and both failures returned", err, intentsListed)
	}
	if out := logged.String(); !strings.Contains(out, "test purge stopped early") || !strings.Contains(out, "things_purged=0") || !strings.Contains(out, "upload_intents_purged=0") {
		t.Errorf("logged %q, want the sweep named with both counts", out)
	}
}

func TestASecondRunOfAJobRefusesWhileTheFirstStillHoldsIt(t *testing.T) {
	dsn := os.Getenv("SKILLHUB_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			t.Fatal("SKILLHUB_REQUIRE_DB=1 but SKILLHUB_TEST_DATABASE_URL is unset")
		}
		t.Skip("SKILLHUB_TEST_DATABASE_URL not set; skipping the job lock")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	const job = "purge-audit"

	holder, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock(hashtextextended($1, 0))", "skillhub:maintenance:"+job); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = holder.Exec(ctx, "SELECT pg_advisory_unlock(hashtextextended($1, 0))", "skillhub:maintenance:"+job)
	}()
	known, err := runExclusively(ctx, pool, job)
	if !known || !errors.Is(err, jobruns.ErrAlreadyRunning) {
		t.Errorf("known=%v err=%v, want a refusal naming the unfinished run before anything is purged", known, err)
	}
}

func TestASweepThatFailedPartWayIsNotLoggedAsComplete(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	logSweep("account purge", errors.New("store unreachable"), "accounts_purged", 3)
	if out := logged.String(); strings.Contains(out, "complete") || !strings.Contains(out, "stopped early") || !strings.Contains(out, "accounts_purged=3") {
		t.Errorf("a failed sweep logged %q", out)
	}
	logged.Reset()
	logSweep("account purge", nil, "accounts_purged", 3)
	if out := logged.String(); !strings.Contains(out, "account purge complete") {
		t.Errorf("a clean sweep logged %q", out)
	}
}

func TestAccountsArePurgedNoSoonerThanTheGraceTheyWerePromised(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"", identity.AccountDeletionGrace, true},
		{identity.AccountDeletionGrace.String(), identity.AccountDeletionGrace, true},
		{(identity.AccountDeletionGrace + time.Hour).String(), identity.AccountDeletionGrace + time.Hour, true},
		{(identity.AccountDeletionGrace - time.Hour).String(), 0, false},
		{"168h", 0, false},
		{"30d", 0, false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Setenv("PURGE_GRACE", tc.raw)
			got, err := wiring.AccountPurgeGrace()
			if (err == nil) != tc.ok || got != tc.want {
				t.Errorf("grace = %s, err = %v; want %s, accepted %v", got, err, tc.want, tc.ok)
			}
		})
	}
}

func TestTracePartitionsRotateWhenAnalyticsIsNotCollected(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	t.Setenv("TRACE_RETENTION", "2160h")
	t.Setenv("ANALYTICS_RETENTION", "")

	err = rotatePartitions(context.Background(), pool)
	if err == nil || strings.Contains(err.Error(), "ANALYTICS_RETENTION") {
		t.Fatalf("rotate-partitions = %v, want only the trace rotation attempted and failing on the unreachable database", err)
	}
}
