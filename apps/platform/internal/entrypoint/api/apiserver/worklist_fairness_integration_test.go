package apiserver_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	packaging "github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
)

type countingObjectStore struct {
	removed    []string
	exists     map[string]bool
	existsCall map[string]int
	removeErr  error
}

func uniqueWorklistLabel(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func shelveWorklistColumn(t *testing.T, pool *pgxpool.Pool, table, key, column, predicate string) {
	t.Helper()
	ctx := context.Background()
	rows, err := pool.Query(ctx, fmt.Sprintf("SELECT %s, %s FROM %s WHERE %s", key, column, table, predicate))
	if err != nil {
		t.Fatal(err)
	}
	type savedTime struct {
		id    pgtype.UUID
		value pgtype.Timestamptz
	}
	var saved []savedTime
	for rows.Next() {
		var item savedTime
		if err := rows.Scan(&item.id, &item.value); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		saved = append(saved, item)
	}
	rows.Close()
	if _, err := pool.Exec(ctx, fmt.Sprintf("UPDATE %s SET %s = now() WHERE %s", table, column, predicate)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, item := range saved {
			if _, err := pool.Exec(context.Background(), fmt.Sprintf("UPDATE %s SET %s = $2 WHERE %s = $1", table, column, key), item.id, item.value); err != nil {
				t.Errorf("restore %s.%s: %v", table, column, err)
			}
		}
	})
}

func shelveExistingWorklists(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	shelveWorklistColumn(t, pool, "artifacts", "id", "retention_attempted_at", "true")
	shelveWorklistColumn(t, pool, "artifacts", "id", "reconcile_checked_at", "true")
	shelveWorklistColumn(t, pool, "datasets", "id", "retention_attempted_at", "true")
	shelveWorklistColumn(t, pool, "datasets", "id", "reconcile_checked_at", "true")
	shelveWorklistColumn(t, pool, "dataset_object_cleanup_intents", "id", "attempted_at", "true")
	shelveWorklistColumn(t, pool, "users", "id", "purge_attempted_at", "true")
	shelveWorklistColumn(t, pool, "search_documents", "skill_id", "enrichment_attempted_at", "true")
	shelveWorklistColumn(t, pool, "runs", "id", "supervision_checked_at", "status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')")
	shelveWorklistColumn(t, pool, "runs", "id", "cleanup_attempted_at", "status IN ('succeeded', 'failed', 'cancelled', 'timed_out')")
}

func (s *countingObjectStore) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	return s.removeErr
}

func TestRetentionCachesASharedRemovalFailureWithoutMarkingRows(t *testing.T) {
	pool := requireDB(t)
	store := &countingObjectStore{removeErr: errors.New("store unavailable")}
	candidates := []objreconcile.Candidate{{ObjectKey: "shared-failure"}, {ObjectKey: "shared-failure"}}
	marks := 0
	n, err := objreconcile.PurgeExpired(context.Background(), pool, store,
		func(context.Context, int32) ([]objreconcile.Candidate, error) { return candidates, nil },
		func(context.Context, pgx.Tx, pgtype.UUID) error { marks++; return nil }, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || marks != 0 {
		t.Fatalf("failed removal reported purged=%d marks=%d, want neither row marked", n, marks)
	}
	if len(store.removed) != 1 {
		t.Fatalf("shared failing key attempted %d times, want once", len(store.removed))
	}
}

func (s *countingObjectStore) Exists(_ context.Context, key string) (bool, error) {
	if s.existsCall == nil {
		s.existsCall = map[string]int{}
	}
	s.existsCall[key]++
	return s.exists == nil || s.exists[key], nil
}

// Bounded maintenance worklists must rotate after claiming a row. Otherwise a
// permanently failing first item occupies every batch and later retention or
// reconciliation work never runs.
func TestBoundedMaintenanceWorklistsRotateClaimedRows(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	tag := uniqueWorklistLabel("fair-worklists")
	f := newFixture(t, a, pool, tag)
	ctx := context.Background()
	q := gen.New(pool)

	insertArtifact := func(key string, expires, created time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO artifacts
			(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at, created_at)
			VALUES ($1, 'download_package', $2, 'application/zip', 1, $2, $2,
			        'available', $3, $4) RETURNING id::text`,
			mustUUID(t, f.workspaceID), key, expires, created).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	assertRotates := func(name string, first, second string) {
		t.Helper()
		if first == second {
			t.Fatalf("%s returned the same claimed row twice: %s", name, first)
		}
	}

	now := time.Now().UTC()
	base := time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)
	claimOld := insertArtifact(tag+"-claim-old", now.Add(time.Hour), base)
	claimNew := insertArtifact(tag+"-claim-new", now.Add(time.Hour), base.Add(time.Minute))
	firstArtifacts, err := q.ListArtifactsClaimingObject(ctx, 1)
	if err != nil || len(firstArtifacts) != 1 {
		t.Fatalf("first artifact claim: rows=%v err=%v", firstArtifacts, err)
	}
	secondArtifacts, err := q.ListArtifactsClaimingObject(ctx, 1)
	if err != nil || len(secondArtifacts) != 1 {
		t.Fatalf("second artifact claim: rows=%v err=%v", secondArtifacts, err)
	}
	assertRotates("artifact reconciliation", uuidText(firstArtifacts[0].ID), uuidText(secondArtifacts[0].ID))
	if uuidText(firstArtifacts[0].ID) != claimOld || uuidText(secondArtifacts[0].ID) != claimNew {
		t.Fatalf("artifact reconciliation claimed %s then %s, want %s then %s", uuidText(firstArtifacts[0].ID), uuidText(secondArtifacts[0].ID), claimOld, claimNew)
	}

	expireOld := insertArtifact(tag+"-expire-old", base, base)
	expireNew := insertArtifact(tag+"-expire-new", base.Add(time.Minute), base.Add(time.Minute))
	firstExpired, err := q.ListArtifactsPastRetention(ctx, 1)
	if err != nil || len(firstExpired) != 1 {
		t.Fatalf("first artifact retention claim: rows=%v err=%v", firstExpired, err)
	}
	secondExpired, err := q.ListArtifactsPastRetention(ctx, 1)
	if err != nil || len(secondExpired) != 1 {
		t.Fatalf("second artifact retention claim: rows=%v err=%v", secondExpired, err)
	}
	assertRotates("artifact retention", uuidText(firstExpired[0].ID), uuidText(secondExpired[0].ID))
	if uuidText(firstExpired[0].ID) != expireOld || uuidText(secondExpired[0].ID) != expireNew {
		t.Fatalf("artifact retention claimed %s then %s, want %s then %s", uuidText(firstExpired[0].ID), uuidText(secondExpired[0].ID), expireOld, expireNew)
	}

	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots
		(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'fairness', '[]', 'fairness-snapshot') RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	insertActiveRun := func(checkedAt *time.Time, created time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, provider,
			 supervision_checked_at, created_at)
			VALUES ($1, $2, $3, 'test', $4, $5) RETURNING id::text`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, snapshotID),
			checkedAt, created).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	recentSupervision := now.Add(-time.Hour)
	oldCheckedRun := insertActiveRun(&recentSupervision, base)
	untriedActiveRun := insertActiveRun(nil, base)
	activeRows, err := q.ListActiveRuns(ctx, 1)
	if err != nil || len(activeRows) != 1 {
		t.Fatalf("active-run worklist: rows=%v err=%v", activeRows, err)
	}
	if got := uuidText(activeRows[0].ID); got != untriedActiveRun || got == oldCheckedRun {
		t.Fatalf("supervisor selected %s, want untried run %s before recently checked %s", got, untriedActiveRun, oldCheckedRun)
	}
	activeRows, err = q.ListActiveRuns(ctx, 1)
	if err != nil || len(activeRows) != 1 {
		t.Fatalf("second active-run worklist claim: rows=%v err=%v", activeRows, err)
	}
	if got := uuidText(activeRows[0].ID); got != oldCheckedRun {
		t.Fatalf("supervisor selected %s twice instead of rotating to %s", got, oldCheckedRun)
	}
	insertRun := func(cleanupStatus string, cleanupAt *time.Time, finished time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `
			INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, status, provider,
			 cleanup_status, cleanup_at, finished_at)
			VALUES ($1, $2, $3, 'failed', 'test', $4, $5, $6) RETURNING id::text`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, snapshotID),
			cleanupStatus, cleanupAt, finished).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	recentAttempt := now.Add(-time.Hour)
	oldFailedRun := insertRun("failed", &recentAttempt, base)
	untriedRun := insertRun("pending", nil, base)
	if _, err := pool.Exec(ctx, `UPDATE runs SET cleanup_attempted_at = $2 WHERE id = $1`,
		mustUUID(t, oldFailedRun), recentAttempt); err != nil {
		t.Fatal(err)
	}
	cleanupRows, err := q.ListRunsNeedingCleanup(ctx, 1)
	if err != nil || len(cleanupRows) != 1 {
		t.Fatalf("cleanup worklist: rows=%v err=%v", cleanupRows, err)
	}
	if got := uuidText(cleanupRows[0].ID); got != untriedRun || got == oldFailedRun {
		t.Fatalf("cleanup selected %s, want untried run %s before recently retried %s", got, untriedRun, oldFailedRun)
	}
	cleanupRows, err = q.ListRunsNeedingCleanup(ctx, 1)
	if err != nil || len(cleanupRows) != 1 {
		t.Fatalf("second cleanup worklist claim: rows=%v err=%v", cleanupRows, err)
	}
	if got := uuidText(cleanupRows[0].ID); got != oldFailedRun {
		t.Fatalf("cleanup selected %s twice instead of rotating to %s", got, oldFailedRun)
	}

	insertRunOutput := func(runID, key string, created time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO artifacts
			(workspace_id, run_id, kind, file_name, content_type, size_bytes,
			 content_hash, object_key, scan_status, expires_at, created_at)
			VALUES ($1, $2, 'run_output', $3, 'application/octet-stream', 1,
			        $3, $3, 'available', $4, $5)`,
			mustUUID(t, f.workspaceID), mustUUID(t, runID), key, created, created); err != nil {
			t.Fatal(err)
		}
	}
	insertRunOutput(oldFailedRun, tag+"-run-output-old", base)
	insertRunOutput(untriedRun, tag+"-run-output-new", base.Add(time.Minute))
	runOutput1, err := q.ListRunOutputsPastRetention(ctx, 1)
	if err != nil || len(runOutput1) != 1 {
		t.Fatalf("first run-output retention claim: rows=%v err=%v", runOutput1, err)
	}
	runOutput2, err := q.ListRunOutputsPastRetention(ctx, 1)
	if err != nil || len(runOutput2) != 1 {
		t.Fatalf("second run-output retention claim: rows=%v err=%v", runOutput2, err)
	}
	assertRotates("run-output retention", uuidText(runOutput1[0].ID), uuidText(runOutput2[0].ID))

	insertDataset := func(key string, expires, created time.Time) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO datasets
			(workspace_id, test_case_id, file_name, content_type, size_bytes,
			 content_hash, object_key, expires_at, created_at)
			VALUES ($1, $2, $3, 'text/plain', 1, $3, $3, $4, $5)`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID), key, expires, created); err != nil {
			t.Fatal(err)
		}
	}
	insertDataset(tag+"-dataset-claim-old", now.Add(time.Hour), base)
	insertDataset(tag+"-dataset-claim-new", now.Add(time.Hour), base.Add(time.Minute))
	datasetClaim1, err := q.ListDatasetsClaimingObject(ctx, 1)
	if err != nil || len(datasetClaim1) != 1 {
		t.Fatalf("first dataset claim: rows=%v err=%v", datasetClaim1, err)
	}
	datasetClaim2, err := q.ListDatasetsClaimingObject(ctx, 1)
	if err != nil || len(datasetClaim2) != 1 {
		t.Fatalf("second dataset claim: rows=%v err=%v", datasetClaim2, err)
	}
	assertRotates("dataset reconciliation", uuidText(datasetClaim1[0].ID), uuidText(datasetClaim2[0].ID))

	insertDataset(tag+"-dataset-expire-old", base, base)
	insertDataset(tag+"-dataset-expire-new", base.Add(time.Minute), base.Add(time.Minute))
	datasetExpiry1, err := q.ListDatasetsPastRetention(ctx, 1)
	if err != nil || len(datasetExpiry1) != 1 {
		t.Fatalf("first dataset retention claim: rows=%v err=%v", datasetExpiry1, err)
	}
	datasetExpiry2, err := q.ListDatasetsPastRetention(ctx, 1)
	if err != nil || len(datasetExpiry2) != 1 {
		t.Fatalf("second dataset retention claim: rows=%v err=%v", datasetExpiry2, err)
	}
	assertRotates("dataset retention", uuidText(datasetExpiry1[0].ID), uuidText(datasetExpiry2[0].ID))

	var intentOld, intentNew string
	if err := pool.QueryRow(ctx, `INSERT INTO dataset_object_cleanup_intents
		(workspace_id, object_key, not_before) VALUES ($1, $2, $3) RETURNING id::text`,
		mustUUID(t, f.workspaceID), tag+"-intent-old", base).Scan(&intentOld); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO dataset_object_cleanup_intents
		(workspace_id, object_key, not_before) VALUES ($1, $2, $3) RETURNING id::text`,
		mustUUID(t, f.workspaceID), tag+"-intent-new", base.Add(time.Minute)).Scan(&intentNew); err != nil {
		t.Fatal(err)
	}
	intent1, err := q.ListDatasetCleanupIntents(ctx, 1)
	if err != nil || len(intent1) != 1 {
		t.Fatalf("first upload-cleanup claim: rows=%v err=%v", intent1, err)
	}
	intent2, err := q.ListDatasetCleanupIntents(ctx, 1)
	if err != nil || len(intent2) != 1 {
		t.Fatalf("second upload-cleanup claim: rows=%v err=%v", intent2, err)
	}
	if uuidText(intent1[0].ID) != intentOld || uuidText(intent2[0].ID) != intentNew {
		t.Fatalf("upload-cleanup claimed %s then %s, want %s then %s", uuidText(intent1[0].ID), uuidText(intent2[0].ID), intentOld, intentNew)
	}

	older := a.login(t, tag+"-purge-old")
	newer := a.login(t, tag+"-purge-new")
	if _, err := pool.Exec(ctx, `UPDATE users SET deletion_requested_at = CASE id
		WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END WHERE id IN ($1, $2)`,
		mustUUID(t, older.userID), mustUUID(t, newer.userID), base, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	account1, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{
		Limit: 1, Cutoff: pgconv.Timestamptz(now),
	})
	if err != nil || len(account1) != 1 {
		t.Fatalf("first account purge claim: rows=%v err=%v", account1, err)
	}
	account2, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{
		Limit: 1, Cutoff: pgconv.Timestamptz(now),
	})
	if err != nil || len(account2) != 1 {
		t.Fatalf("second account purge claim: rows=%v err=%v", account2, err)
	}
	assertRotates("account purge", uuidText(account1[0]), uuidText(account2[0]))

	secondSkill := seedSkill(t, pool, f.workspaceID, tag+"-second-skill")
	seedVersion(t, pool, f.workspaceID, secondSkill, tag+"-second-version")
	if _, err := pool.Exec(ctx, `UPDATE search_documents
		SET enrichment_status = 'pending', enrichment_attempted_at = NULL,
		    updated_at = CASE skill_id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END
		WHERE skill_id IN ($1, $2)`, mustUUID(t, f.skillID), mustUUID(t, secondSkill), base, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	enrichment1, err := q.ListPendingEnrichment(ctx, 1)
	if err != nil || len(enrichment1) != 1 {
		t.Fatalf("first enrichment claim: rows=%v err=%v", enrichment1, err)
	}
	enrichment2, err := q.ListPendingEnrichment(ctx, 1)
	if err != nil || len(enrichment2) != 1 {
		t.Fatalf("second enrichment claim: rows=%v err=%v", enrichment2, err)
	}
	assertRotates("enrichment", uuidText(enrichment1[0].SkillID), uuidText(enrichment2[0].SkillID))
}

func TestWorklistAttemptsResetOnlyWhenWorkBecomesFreshAgain(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	ctx := context.Background()
	q := gen.New(pool)

	account := a.login(t, uniqueWorklistLabel("attempt-reset-account"))
	userID := mustUUID(t, account.userID)
	requested, err := q.RequestAccountDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET purge_attempted_at = now() WHERE id = $1`, userID); err != nil {
		t.Fatal(err)
	}
	requestedAgain, err := q.RequestAccountDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !requestedAgain.PurgeAttemptedAt.Valid || requestedAgain.DeletionRequestedAt != requested.DeletionRequestedAt {
		t.Fatal("an idempotent deletion request reset its existing worklist history")
	}
	cancelled, err := q.CancelAccountDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.DeletionRequestedAt.Valid || cancelled.PurgeAttemptedAt.Valid || cancelled.PurgeStartedAt.Valid {
		t.Fatalf("cancel left deletion worklist state behind: %+v", cancelled)
	}
	requestedFresh, err := q.RequestAccountDeletion(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if requestedFresh.PurgeAttemptedAt.Valid || requestedFresh.PurgeStartedAt.Valid {
		t.Fatalf("fresh deletion request inherited an old attempt: %+v", requestedFresh)
	}

	tag := uniqueWorklistLabel("attempt-reset-enrichment")
	f := newFixture(t, a, pool, tag)
	if _, err := pool.Exec(ctx, `UPDATE search_documents
		SET enrichment_status = 'pending', enrichment_attempted_at = NULL,
		    updated_at = '1700-01-01' WHERE skill_id = $1`, mustUUID(t, f.skillID)); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ListPendingEnrichment(ctx, 1)
	if err != nil || len(claimed) != 1 || uuidText(claimed[0].SkillID) != f.skillID {
		t.Fatalf("enrichment claim = %+v, %v; want fixture %s", claimed, err, f.skillID)
	}
	var attempted bool
	if err := pool.QueryRow(ctx, `SELECT enrichment_attempted_at IS NOT NULL
		FROM search_documents WHERE skill_id = $1`, mustUUID(t, f.skillID)).Scan(&attempted); err != nil {
		t.Fatal(err)
	}
	if !attempted {
		t.Fatal("claim did not stamp the enrichment attempt")
	}
	if err := q.UpsertSearchDocumentEnriched(ctx, gen.UpsertSearchDocumentEnrichedParams{
		SkillID: mustUUID(t, f.skillID), WorkspaceID: mustUUID(t, f.workspaceID),
		Name: "reset", Summary: "reset", TaskExamples: "[]", Tags: []byte(`[]`),
		Limitations: "[]", Scan: []byte(`{}`), EnrichmentStatus: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT enrichment_attempted_at IS NOT NULL
		FROM search_documents WHERE skill_id = $1`, mustUUID(t, f.skillID)).Scan(&attempted); err != nil {
		t.Fatal(err)
	}
	if attempted {
		t.Fatal("new pending enrichment retained the previous attempt timestamp")
	}
}

func TestAutocommitWorklistClaimsLeaseRowsAcrossExternalWork(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	tag := uniqueWorklistLabel("skip-locked")
	f := newFixture(t, a, pool, tag)
	ctx := context.Background()
	base := time.Date(1600, time.January, 1, 0, 0, 0, 0, time.UTC)
	var snapshotID pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO test_case_snapshots
		(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'skip locked', '[]', $3) RETURNING id`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID), tag+"-snapshot").Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	var outputHost pgtype.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO runs
		(workspace_id, skill_version_id, test_case_snapshot_id, provider, supervision_checked_at)
		VALUES ($1, $2, $3, 'test', now()) RETURNING id`, mustUUID(t, f.workspaceID),
		mustUUID(t, f.versionID), snapshotID).Scan(&outputHost); err != nil {
		t.Fatal(err)
	}

	assertConcurrentClaims := func(name string, want map[string]bool, claim func(context.Context, *gen.Queries) (string, error)) {
		t.Helper()
		// Production calls through the pool, so the row lock ends as soon as this
		// statement returns and external work begins. The attempted-at lease, not
		// an artificially open test transaction, must keep worker two away.
		first, err := claim(ctx, gen.New(pool))
		if err != nil {
			t.Fatalf("%s first claim: %v", name, err)
		}
		secondCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		second, err := claim(secondCtx, gen.New(pool))
		if err != nil {
			t.Fatalf("%s second claim blocked instead of skipping: %v", name, err)
		}
		if first == second || !want[first] || !want[second] {
			t.Fatalf("%s claims = %s then %s, want the two fixture rows", name, first, second)
		}
		if third, err := claim(ctx, gen.New(pool)); err == nil || !strings.Contains(err.Error(), "rows=[]") {
			t.Fatalf("%s immediately reclaimed leased row %s (third=%s err=%v)", name, first, third, err)
		}
	}
	insertArtifact := func(kind, key string, expires time.Time, runID *pgtype.UUID) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO artifacts
			(workspace_id, run_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at, created_at)
			VALUES ($1, $2, $3, $4, 'application/octet-stream', 1, $4, $4,
			        'available', $5, $6) RETURNING id::text`,
			mustUUID(t, f.workspaceID), runID, kind, key, expires, base).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}

	artifactRetention := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := insertArtifact("download_package", fmt.Sprintf("%s-artifact-expired-%d", tag, i), base.Add(time.Duration(i)*time.Minute), nil)
		artifactRetention[id] = true
	}
	assertConcurrentClaims("artifact retention", artifactRetention, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListArtifactsPastRetention(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})

	runOutputRetention := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := insertArtifact("run_output", fmt.Sprintf("%s-run-output-%d", tag, i), base.Add(time.Duration(i)*time.Minute), &outputHost)
		runOutputRetention[id] = true
	}
	assertConcurrentClaims("run-output retention", runOutputRetention, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListRunOutputsPastRetention(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})

	artifactReconcile := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := insertArtifact("download_package", fmt.Sprintf("%s-artifact-live-%d", tag, i), time.Now().Add(time.Hour), nil)
		artifactReconcile[id] = true
	}
	assertConcurrentClaims("artifact reconciliation", artifactReconcile, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListArtifactsClaimingObject(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})

	insertDataset := func(key string, expires time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO datasets
			(workspace_id, test_case_id, file_name, content_type, size_bytes,
			 content_hash, object_key, expires_at, created_at)
			VALUES ($1, $2, $3, 'text/plain', 1, $3, $3, $4, $5) RETURNING id::text`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID), key, expires, base).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	datasetRetention := map[string]bool{}
	datasetReconcile := map[string]bool{}
	for i := 0; i < 2; i++ {
		datasetRetention[insertDataset(fmt.Sprintf("%s-dataset-expired-%d", tag, i), base.Add(time.Duration(i)*time.Minute))] = true
		datasetReconcile[insertDataset(fmt.Sprintf("%s-dataset-live-%d", tag, i), time.Now().Add(time.Hour))] = true
	}
	assertConcurrentClaims("dataset retention", datasetRetention, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListDatasetsPastRetention(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})
	assertConcurrentClaims("dataset reconciliation", datasetReconcile, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListDatasetsClaimingObject(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})

	cleanupIntents := map[string]bool{}
	for i := 0; i < 2; i++ {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO dataset_object_cleanup_intents
			(workspace_id, object_key, not_before) VALUES ($1, $2, $3) RETURNING id::text`,
			mustUUID(t, f.workspaceID), fmt.Sprintf("%s-intent-%d", tag, i), base.Add(time.Duration(i)*time.Minute)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		cleanupIntents[id] = true
	}
	assertConcurrentClaims("upload cleanup", cleanupIntents, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListDatasetCleanupIntents(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})

	accounts := map[string]bool{}
	for i := 0; i < 2; i++ {
		account := a.login(t, fmt.Sprintf("%s-account-%d", tag, i))
		accounts[account.userID] = true
		if _, err := pool.Exec(ctx, `UPDATE users SET deletion_requested_at = $2,
			purge_attempted_at = NULL, purge_started_at = NULL WHERE id = $1`,
			mustUUID(t, account.userID), base.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	assertConcurrentClaims("account purge", accounts, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{Limit: 1, Cutoff: pgconv.Timestamptz(time.Now())})
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0]), nil
	})

	secondSkill := seedSkill(t, pool, f.workspaceID, tag+"-second-skill")
	seedVersion(t, pool, f.workspaceID, secondSkill, tag+"-second-version")
	enrichment := map[string]bool{f.skillID: true, secondSkill: true}
	if _, err := pool.Exec(ctx, `UPDATE search_documents SET enrichment_status = 'pending',
		enrichment_attempted_at = NULL, updated_at = CASE skill_id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END
		WHERE skill_id IN ($1, $2)`, mustUUID(t, f.skillID), mustUUID(t, secondSkill), base, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertConcurrentClaims("enrichment", enrichment, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListPendingEnrichment(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].SkillID), nil
	})

	insertRun := func(status string, finished *time.Time) string {
		t.Helper()
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, status, provider,
			 finished_at, cleanup_status, created_at)
			VALUES ($1, $2, $3, $4, 'test', $5, 'pending', $6) RETURNING id::text`,
			mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), snapshotID, status, finished, base).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	activeFirst := insertRun("queued", nil)
	activeSecond := insertRun("queued", nil)
	active := map[string]bool{activeFirst: true, activeSecond: true}
	if _, err := pool.Exec(ctx, `UPDATE runs SET supervision_checked_at = '-infinity', created_at = '-infinity'
		WHERE id IN ($1, $2)`, mustUUID(t, activeFirst), mustUUID(t, activeSecond)); err != nil {
		t.Fatal(err)
	}
	assertConcurrentClaims("active-run supervision", active, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListActiveRuns(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})
	if _, err := pool.Exec(ctx, `UPDATE runs SET supervision_checked_at = now() - interval '31 seconds'
		WHERE id = $1`, mustUUID(t, activeFirst)); err != nil {
		t.Fatal(err)
	}
	if rows, err := gen.New(pool).ListActiveRuns(ctx, 1); err != nil || len(rows) != 1 || uuidText(rows[0].ID) != activeFirst {
		t.Fatalf("active-run lease did not reopen after one supervisor interval: rows=%v err=%v", rows, err)
	}
	finished := base
	cleanup := map[string]bool{insertRun("failed", &finished): true, insertRun("failed", &finished): true}
	assertConcurrentClaims("run cleanup", cleanup, func(ctx context.Context, q *gen.Queries) (string, error) {
		rows, err := q.ListRunsNeedingCleanup(ctx, 1)
		if err != nil || len(rows) != 1 {
			return "", fmt.Errorf("rows=%v: %w", rows, err)
		}
		return uuidText(rows[0].ID), nil
	})
	var cleanupID string
	for id := range cleanup {
		cleanupID = id
		break
	}
	if _, err := pool.Exec(ctx, `UPDATE runs SET cleanup_attempted_at = now() - interval '31 seconds'
		WHERE id = $1`, mustUUID(t, cleanupID)); err != nil {
		t.Fatal(err)
	}
	if rows, err := gen.New(pool).ListRunsNeedingCleanup(ctx, 1); err != nil || len(rows) != 1 || uuidText(rows[0].ID) != cleanupID {
		t.Fatalf("cleanup lease did not reopen after one supervisor interval: rows=%v err=%v", rows, err)
	}
}

func TestRetentionRemovesASharedObjectOnlyOncePerBatch(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("shared-retention-object"))
	ctx := context.Background()
	var candidates []objreconcile.Candidate
	for i := 0; i < 2; i++ {
		var candidate objreconcile.Candidate
		if err := pool.QueryRow(ctx, `
			INSERT INTO artifacts
			(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at)
			VALUES ($1, 'download_package', $2, 'application/zip', 1, $2,
			        'shared-key', 'available', now() - interval '1 hour')
			RETURNING id, workspace_id, object_key`,
			mustUUID(t, c.workspaceID), "shared-row-"+string(rune('a'+i))).Scan(
			&candidate.ID, &candidate.WorkspaceID, &candidate.ObjectKey); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
	}

	store := &countingObjectStore{}
	svc := &packaging.Service{Pool: pool}
	n, err := objreconcile.PurgeExpired(ctx, pool, store,
		func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return candidates, nil
		}, svc.MarkArtifactPurged, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("purged rows = %d, want at least the two shared rows", n)
	}
	sharedRemovals := 0
	for _, key := range store.removed {
		if key == "shared-key" {
			sharedRemovals++
		}
	}
	if sharedRemovals != 1 {
		t.Fatalf("object removals = %v, want shared-key exactly once", store.removed)
	}
}

func TestDownloadRetentionKeepsBytesNeededByANewerArtifact(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("shared-live-retention"))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var expired objreconcile.Candidate
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts
		(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
		 object_key, scan_status, expires_at)
		VALUES ($1, 'download_package', 'old.zip', 'application/zip', 1, 'same',
		        'downloads/shared-live', 'available', now() - interval '1 hour')
		RETURNING id, workspace_id, object_key`, mustUUID(t, c.workspaceID)).Scan(
		&expired.ID, &expired.WorkspaceID, &expired.ObjectKey); err != nil {
		t.Fatal(err)
	}
	var liveID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO artifacts
		(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
		 object_key, scan_status, expires_at)
		VALUES ($1, 'download_package', 'new.zip', 'application/zip', 1, 'same',
		        'downloads/shared-live', 'available', now() + interval '1 hour')
		RETURNING id`, mustUUID(t, c.workspaceID)).Scan(&liveID); err != nil {
		t.Fatal(err)
	}

	// Clean mode has one database connection. Holding the guard transaction and
	// opening another transaction to mark the row would deadlock this call.
	cfg := pool.Config().Copy()
	cfg.MaxConns = 1
	single, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	store := &countingObjectStore{}
	svc := &packaging.Service{Pool: single}
	n, err := objreconcile.PurgeExpired(ctx, single, store,
		func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return []objreconcile.Candidate{expired}, nil
		}, svc.MarkArtifactPurged, svc.GuardArtifactRemoval, 1)
	if err != nil || n != 1 {
		t.Fatalf("retention = %d, %v; want one expired row completed", n, err)
	}
	if len(store.removed) != 0 {
		t.Fatalf("retention removed bytes still used by a live artifact: %v", store.removed)
	}
	var expiredPurged, livePurged bool
	if err := pool.QueryRow(ctx, `SELECT purged_at IS NOT NULL FROM artifacts WHERE id = $1`, expired.ID).Scan(&expiredPurged); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT purged_at IS NOT NULL FROM artifacts WHERE id = $1`, liveID).Scan(&livePurged); err != nil {
		t.Fatal(err)
	}
	if !expiredPurged || livePurged {
		t.Fatalf("purged flags: expired=%v live=%v", expiredPurged, livePurged)
	}
}

func TestReconciliationChecksASharedObjectOnlyOncePerBatch(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, uniqueWorklistLabel("shared-reconcile-object"))
	ctx := context.Background()
	var candidates []objreconcile.Candidate
	for i := 0; i < 2; i++ {
		var candidate objreconcile.Candidate
		if err := pool.QueryRow(ctx, `
			INSERT INTO artifacts
			(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at)
			VALUES ($1, 'download_package', $2, 'application/zip', 1, $2,
			        'shared-live-key', 'available', now() + interval '1 hour')
			RETURNING id, workspace_id, object_key`,
			mustUUID(t, c.workspaceID), "shared-live-row-"+string(rune('a'+i))).Scan(
			&candidate.ID, &candidate.WorkspaceID, &candidate.ObjectKey); err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
	}
	datasetCandidates := make([]objreconcile.Candidate, len(candidates))
	for i, candidate := range candidates {
		datasetCandidates[i] = candidate
		datasetCandidates[i].ObjectKey = "shared-dataset-key"
	}
	store := &countingObjectStore{exists: map[string]bool{
		"shared-live-key": true, "shared-dataset-key": true,
	}}
	svc := &packaging.Service{Pool: pool}
	noCandidates := func(context.Context, int32) ([]objreconcile.Candidate, error) { return nil, nil }
	sweep := &objreconcile.Service{
		Pool: pool, Store: store,
		ListExpiredArtifacts: noCandidates,
		ListDownloadIntents:  noCandidates,
		ListClaimedArtifacts: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return candidates, nil
		},
		ListClaimedDatasets: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return datasetCandidates, nil
		},
		RecordArtifactPurged:       svc.MarkArtifactPurged,
		RecordDownloadIntentPurged: func(context.Context, pgx.Tx, pgtype.UUID) error { return nil },
		RecordDatasetLost:          func(context.Context, pgx.Tx, pgtype.UUID) error { return nil },
		GuardArtifactRemoval:       svc.GuardArtifactRemoval,
	}
	if err := sweep.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if got := store.existsCall["shared-live-key"]; got != 1 {
		t.Fatalf("shared object HEAD calls = %d, want 1", got)
	}
	if got := store.existsCall["shared-dataset-key"]; got != 1 {
		t.Fatalf("shared dataset HEAD calls = %d, want 1", got)
	}
}
