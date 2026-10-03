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

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/queue"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objreconcile"
	packaging "github.com/ArthurC02/skillhub/apps/platform/internal/skill/delivery"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
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
	n, err := objreconcile.PurgeExpired(context.Background(), pool, store, objreconcile.RetentionOwner{
		List: func(context.Context, int32) ([]objreconcile.Candidate, error) { return candidates, nil },
		Mark: func(context.Context, pgx.Tx, pgtype.UUID) error { marks++; return nil },
	}, 10)
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

func TestBoundedMaintenanceWorklistsRotateClaimedRows(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	tag := uniqueWorklistLabel("fair-worklists")
	w := &rotationWorld{pool: pool, q: gen.New(pool), f: newFixture(t, a, pool, tag), tag: tag}
	w.now = time.Now().UTC()
	w.base = time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

	w.artifactReconciliationRotatesOldestFirst(t)
	w.artifactRetentionRotatesOldestFirst(t)
	w.snapshotID = w.seedSnapshot(t)
	w.activeRunSupervisionRotates(t)
	oldFailedRun, untriedRun := w.runCleanupRotates(t)
	w.runOutputRetentionRotates(t, oldFailedRun, untriedRun)
	w.datasetWorklistsRotate(t)
	w.uploadCleanupRotatesOldestFirst(t)
	w.accountPurgeRotates(t, a)
	w.enrichmentRotates(t)
}

type rotationWorld struct {
	pool       *pgxpool.Pool
	q          *gen.Queries
	f          fixture
	tag        string
	now, base  time.Time
	snapshotID string
}

func claimTwice[T any](t *testing.T, name string, claim func() ([]T, error)) (first, second T) {
	t.Helper()
	firstRows, err := claim()
	if err != nil || len(firstRows) != 1 {
		t.Fatalf("first %s: rows=%v err=%v", name, firstRows, err)
	}
	secondRows, err := claim()
	if err != nil || len(secondRows) != 1 {
		t.Fatalf("second %s: rows=%v err=%v", name, secondRows, err)
	}
	return firstRows[0], secondRows[0]
}

func assertClaimsRotate(t *testing.T, name string, first, second string) {
	t.Helper()
	if first == second {
		t.Fatalf("%s returned the same claimed row twice: %s", name, first)
	}
}

func sweepClaim() pgtype.Interval { return pgconv.Interval(queue.SweepClaimLease) }

func (w *rotationWorld) insertArtifact(t *testing.T, key string, expires, created time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
			INSERT INTO artifacts
			(workspace_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at, created_at)
			VALUES ($1, 'download_package', $2, 'application/zip', 1, $2, $2,
			        'available', $3, $4) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), key, expires, created).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) artifactReconciliationRotatesOldestFirst(t *testing.T) {
	claimOld := w.insertArtifact(t, w.tag+"-claim-old", w.now.Add(time.Hour), w.base)
	claimNew := w.insertArtifact(t, w.tag+"-claim-new", w.now.Add(time.Hour), w.base.Add(time.Minute))
	first, second := claimTwice(t, "artifact claim", func() ([]gen.ListArtifactsClaimingObjectRow, error) {
		return w.q.ListArtifactsClaimingObject(context.Background(), gen.ListArtifactsClaimingObjectParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "artifact reconciliation", uuidText(first.ID), uuidText(second.ID))
	if uuidText(first.ID) != claimOld || uuidText(second.ID) != claimNew {
		t.Fatalf("artifact reconciliation claimed %s then %s, want %s then %s", uuidText(first.ID), uuidText(second.ID), claimOld, claimNew)
	}
}

func (w *rotationWorld) artifactRetentionRotatesOldestFirst(t *testing.T) {
	expireOld := w.insertArtifact(t, w.tag+"-expire-old", w.base, w.base)
	expireNew := w.insertArtifact(t, w.tag+"-expire-new", w.base.Add(time.Minute), w.base.Add(time.Minute))
	first, second := claimTwice(t, "artifact retention claim", func() ([]gen.ListArtifactsPastRetentionRow, error) {
		return w.q.ListArtifactsPastRetention(context.Background(), gen.ListArtifactsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "artifact retention", uuidText(first.ID), uuidText(second.ID))
	if uuidText(first.ID) != expireOld || uuidText(second.ID) != expireNew {
		t.Fatalf("artifact retention claimed %s then %s, want %s then %s", uuidText(first.ID), uuidText(second.ID), expireOld, expireNew)
	}
}

func (w *rotationWorld) seedSnapshot(t *testing.T) string {
	t.Helper()
	var snapshotID string
	if err := w.pool.QueryRow(context.Background(), `
		INSERT INTO test_case_snapshots
		(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'fairness', '[]', 'fairness-snapshot') RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.testCaseID)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	return snapshotID
}

func (w *rotationWorld) insertActiveRun(t *testing.T, checkedAt *time.Time, created time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
			INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, provider,
			 supervision_checked_at, created_at)
			VALUES ($1, $2, $3, 'test', $4, $5) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.versionID), mustUUID(t, w.snapshotID),
		checkedAt, created).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) activeRunSupervisionRotates(t *testing.T) {
	ctx := context.Background()
	recentSupervision := w.now.Add(-time.Hour)
	oldCheckedRun := w.insertActiveRun(t, &recentSupervision, w.base)
	untriedActiveRun := w.insertActiveRun(t, nil, w.base)
	activeRows, err := w.q.ListActiveRuns(ctx, run.ActiveRunClaim(1))
	if err != nil || len(activeRows) != 1 {
		t.Fatalf("active-run worklist: rows=%v err=%v", activeRows, err)
	}
	if got := uuidText(activeRows[0].ID); got != untriedActiveRun || got == oldCheckedRun {
		t.Fatalf("supervisor selected %s, want untried run %s before recently checked %s", got, untriedActiveRun, oldCheckedRun)
	}
	activeRows, err = w.q.ListActiveRuns(ctx, run.ActiveRunClaim(1))
	if err != nil || len(activeRows) != 1 {
		t.Fatalf("second active-run worklist claim: rows=%v err=%v", activeRows, err)
	}
	if got := uuidText(activeRows[0].ID); got != oldCheckedRun {
		t.Fatalf("supervisor selected %s twice instead of rotating to %s", got, oldCheckedRun)
	}
}

func (w *rotationWorld) insertFailedRun(t *testing.T, cleanupStatus string, cleanupAt *time.Time, finished time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
			INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, status, provider,
			 cleanup_status, cleanup_at, finished_at)
			VALUES ($1, $2, $3, 'failed', 'test', $4, $5, $6) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.versionID), mustUUID(t, w.snapshotID),
		cleanupStatus, cleanupAt, finished).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) runCleanupRotates(t *testing.T) (oldFailedRun, untriedRun string) {
	ctx := context.Background()
	recentAttempt := w.now.Add(-time.Hour)
	oldFailedRun = w.insertFailedRun(t, "failed", &recentAttempt, w.base)
	untriedRun = w.insertFailedRun(t, "pending", nil, w.base)
	if _, err := w.pool.Exec(ctx, `UPDATE runs SET cleanup_attempted_at = $2 WHERE id = $1`,
		mustUUID(t, oldFailedRun), recentAttempt); err != nil {
		t.Fatal(err)
	}
	cleanupRows, err := w.q.ListRunsNeedingCleanup(ctx, run.CleanupClaim(1))
	if err != nil || len(cleanupRows) != 1 {
		t.Fatalf("cleanup worklist: rows=%v err=%v", cleanupRows, err)
	}
	if got := uuidText(cleanupRows[0].ID); got != untriedRun || got == oldFailedRun {
		t.Fatalf("cleanup selected %s, want untried run %s before recently retried %s", got, untriedRun, oldFailedRun)
	}
	cleanupRows, err = w.q.ListRunsNeedingCleanup(ctx, run.CleanupClaim(1))
	if err != nil || len(cleanupRows) != 1 {
		t.Fatalf("second cleanup worklist claim: rows=%v err=%v", cleanupRows, err)
	}
	if got := uuidText(cleanupRows[0].ID); got != oldFailedRun {
		t.Fatalf("cleanup selected %s twice instead of rotating to %s", got, oldFailedRun)
	}
	return oldFailedRun, untriedRun
}

func (w *rotationWorld) insertRunOutput(t *testing.T, runID, key string, created time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
			INSERT INTO artifacts
			(workspace_id, run_id, kind, file_name, content_type, size_bytes,
			 content_hash, object_key, scan_status, expires_at, created_at)
			VALUES ($1, $2, 'run_output', $3, 'application/octet-stream', 1,
			        $3, $3, 'available', $4, $5) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, runID), key, created, created).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) runOutputRetentionRotates(t *testing.T, oldFailedRun, untriedRun string) {
	outputOld := w.insertRunOutput(t, oldFailedRun, w.tag+"-run-output-old", w.base)
	outputNew := w.insertRunOutput(t, untriedRun, w.tag+"-run-output-new", w.base.Add(time.Minute))
	first, second := claimTwice(t, "run-output retention claim", func() ([]gen.ListRunOutputsPastRetentionRow, error) {
		return w.q.ListRunOutputsPastRetention(context.Background(), gen.ListRunOutputsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "run-output retention", uuidText(first.ID), uuidText(second.ID))
	if uuidText(first.ID) != outputOld || uuidText(second.ID) != outputNew {
		t.Fatalf("run-output retention claimed %s then %s, want %s then %s", uuidText(first.ID), uuidText(second.ID), outputOld, outputNew)
	}
}

func (w *rotationWorld) insertDataset(t *testing.T, key string, expires, created time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `
			INSERT INTO datasets
			(workspace_id, test_case_id, file_name, content_type, size_bytes,
			 content_hash, object_key, expires_at, created_at)
			VALUES ($1, $2, $3, 'text/plain', 1, $3, $3, $4, $5) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.testCaseID), key, expires, created).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) datasetWorklistsRotate(t *testing.T) {
	claimOld := w.insertDataset(t, w.tag+"-dataset-claim-old", w.now.Add(time.Hour), w.base)
	claimNew := w.insertDataset(t, w.tag+"-dataset-claim-new", w.now.Add(time.Hour), w.base.Add(time.Minute))
	claim1, claim2 := claimTwice(t, "dataset claim", func() ([]gen.ListDatasetsClaimingObjectRow, error) {
		return w.q.ListDatasetsClaimingObject(context.Background(), gen.ListDatasetsClaimingObjectParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "dataset reconciliation", uuidText(claim1.ID), uuidText(claim2.ID))
	if uuidText(claim1.ID) != claimOld || uuidText(claim2.ID) != claimNew {
		t.Fatalf("dataset reconciliation claimed %s then %s, want %s then %s", uuidText(claim1.ID), uuidText(claim2.ID), claimOld, claimNew)
	}

	expireOld := w.insertDataset(t, w.tag+"-dataset-expire-old", w.base, w.base)
	expireNew := w.insertDataset(t, w.tag+"-dataset-expire-new", w.base.Add(time.Minute), w.base.Add(time.Minute))
	expiry1, expiry2 := claimTwice(t, "dataset retention claim", func() ([]gen.ListDatasetsPastRetentionRow, error) {
		return w.q.ListDatasetsPastRetention(context.Background(), gen.ListDatasetsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "dataset retention", uuidText(expiry1.ID), uuidText(expiry2.ID))
	if uuidText(expiry1.ID) != expireOld || uuidText(expiry2.ID) != expireNew {
		t.Fatalf("dataset retention claimed %s then %s, want %s then %s", uuidText(expiry1.ID), uuidText(expiry2.ID), expireOld, expireNew)
	}
}

func (w *rotationWorld) insertCleanupIntent(t *testing.T, key string, notBefore time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `INSERT INTO dataset_object_cleanup_intents
		(workspace_id, object_key, not_before) VALUES ($1, $2, $3) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), key, notBefore).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *rotationWorld) uploadCleanupRotatesOldestFirst(t *testing.T) {
	intentOld := w.insertCleanupIntent(t, w.tag+"-intent-old", w.base)
	intentNew := w.insertCleanupIntent(t, w.tag+"-intent-new", w.base.Add(time.Minute))
	intent1, intent2 := claimTwice(t, "upload-cleanup claim", func() ([]gen.ListDatasetCleanupIntentsRow, error) {
		return w.q.ListDatasetCleanupIntents(context.Background(), gen.ListDatasetCleanupIntentsParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	if uuidText(intent1.ID) != intentOld || uuidText(intent2.ID) != intentNew {
		t.Fatalf("upload-cleanup claimed %s then %s, want %s then %s", uuidText(intent1.ID), uuidText(intent2.ID), intentOld, intentNew)
	}
}

func (w *rotationWorld) accountPurgeRotates(t *testing.T, a *api) {
	older := a.login(t, w.tag+"-purge-old")
	newer := a.login(t, w.tag+"-purge-new")
	if _, err := w.pool.Exec(context.Background(), `UPDATE users SET deletion_requested_at = CASE id
		WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END WHERE id IN ($1, $2)`,
		mustUUID(t, older.userID), mustUUID(t, newer.userID), w.base, w.base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	account1, account2 := claimTwice(t, "account purge claim", func() ([]pgtype.UUID, error) {
		return w.q.ListAccountsPastGrace(context.Background(), gen.ListAccountsPastGraceParams{
			Cutoff: pgconv.Timestamptz(w.now), ClaimLease: sweepClaim(), BatchSize: 1,
		})
	})
	assertClaimsRotate(t, "account purge", uuidText(account1), uuidText(account2))
	if uuidText(account1) != older.userID || uuidText(account2) != newer.userID {
		t.Fatalf("account purge claimed %s then %s, want %s then %s", uuidText(account1), uuidText(account2), older.userID, newer.userID)
	}
}

func (w *rotationWorld) enrichmentRotates(t *testing.T) {
	secondSkill := seedSkill(t, w.pool, w.f.workspaceID, w.tag+"-second-skill")
	seedVersion(t, w.pool, w.f.workspaceID, secondSkill, w.tag+"-second-version")
	if _, err := w.pool.Exec(context.Background(), `UPDATE search_documents
		SET enrichment_status = 'pending', enrichment_attempted_at = NULL,
		    updated_at = CASE skill_id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END
		WHERE skill_id IN ($1, $2)`, mustUUID(t, w.f.skillID), mustUUID(t, secondSkill), w.base, w.base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	enrichment1, enrichment2 := claimTwice(t, "enrichment claim", func() ([]gen.ListPendingEnrichmentRow, error) {
		return w.q.ListPendingEnrichment(context.Background(), gen.ListPendingEnrichmentParams{ClaimLease: sweepClaim(), BatchSize: 1})
	})
	assertClaimsRotate(t, "enrichment", uuidText(enrichment1.SkillID), uuidText(enrichment2.SkillID))
	if uuidText(enrichment1.SkillID) != w.f.skillID || uuidText(enrichment2.SkillID) != secondSkill {
		t.Fatalf("enrichment claimed %s then %s, want %s then %s", uuidText(enrichment1.SkillID), uuidText(enrichment2.SkillID), w.f.skillID, secondSkill)
	}
}

func TestWorklistAttemptsResetOnlyWhenWorkBecomesFreshAgain(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)

	assertDeletionAttemptResetsOnlyOnAFreshRequest(t, a, pool)
	assertNewPendingEnrichmentDropsThePreviousAttempt(t, a, pool)
}

func assertDeletionAttemptResetsOnlyOnAFreshRequest(t *testing.T, a *api, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	account := a.login(t, uniqueWorklistLabel("attempt-reset-account"))
	user := identity.User{ID: mustUUID(t, account.userID)}
	type deletionState struct{ requestedAt, attemptedAt pgtype.Timestamptz }
	change := func(what string, act func(context.Context, identity.User) (identity.User, error)) deletionState {
		t.Helper()
		if _, err := act(ctx, user); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		var state deletionState
		if err := pool.QueryRow(ctx, `SELECT deletion_requested_at, purge_attempted_at FROM users WHERE id = $1`,
			user.ID).Scan(&state.requestedAt, &state.attemptedAt); err != nil {
			t.Fatal(err)
		}
		return state
	}
	requested := change("request", a.auth.Service.RequestAccountDeletion)
	if _, err := pool.Exec(ctx, `UPDATE users SET purge_attempted_at = now() WHERE id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	requestedAgain := change("repeated request", a.auth.Service.RequestAccountDeletion)
	if !requestedAgain.attemptedAt.Valid || !requestedAgain.requestedAt.Time.Equal(requested.requestedAt.Time) {
		t.Fatalf("an idempotent deletion request reset its existing worklist history: %+v", requestedAgain)
	}
	if cancelled := change("cancel", a.auth.Service.CancelAccountDeletion); cancelled != (deletionState{}) {
		t.Fatalf("cancel left deletion worklist state behind: %+v", cancelled)
	}
	if fresh := change("fresh request", a.auth.Service.RequestAccountDeletion); !fresh.requestedAt.Valid || fresh.attemptedAt.Valid {
		t.Fatalf("fresh deletion request inherited an old attempt or was not stamped: %+v", fresh)
	}
}

func assertNewPendingEnrichmentDropsThePreviousAttempt(t *testing.T, a *api, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(pool)
	tag := uniqueWorklistLabel("attempt-reset-enrichment")
	f := newFixture(t, a, pool, tag)
	if _, err := pool.Exec(ctx, `UPDATE search_documents
		SET enrichment_status = 'pending', enrichment_attempted_at = NULL,
		    updated_at = '1700-01-01' WHERE skill_id = $1`, mustUUID(t, f.skillID)); err != nil {
		t.Fatal(err)
	}
	claimed, err := q.ListPendingEnrichment(ctx, gen.ListPendingEnrichmentParams{ClaimLease: pgconv.Interval(queue.SweepClaimLease), BatchSize: 1})
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
		Limitations: "[]", Scan: []byte(`{}`), EnrichmentStatus: "pending", RestartEnrichmentAttempts: true,
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

	w := &leaseWorld{pool: pool, f: f, tag: tag, base: base, snapshotID: snapshotID, outputHost: outputHost}

	w.artifactWorklistsLease(t)
	w.datasetWorklistsLease(t)
	w.uploadCleanupLeases(t)
	w.accountPurgeLeases(t, a)
	w.enrichmentLeases(t)
	w.activeRunSupervisionLeasesAndReopens(t)
	w.runCleanupLeasesReopensAndWaitsForTheRescueWindow(t)
}

type leaseWorld struct {
	pool       *pgxpool.Pool
	f          fixture
	tag        string
	base       time.Time
	snapshotID pgtype.UUID
	outputHost pgtype.UUID
}

type worklistClaim func(context.Context, *gen.Queries) (string, error)

func assertConcurrentClaims(t *testing.T, pool *pgxpool.Pool, name string, want map[string]bool, claim worklistClaim) {
	t.Helper()
	ctx := context.Background()
	// Each claim runs its own statement against the pool rather than a shared
	// transaction, so the row lock releases as soon as it returns.
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

func onlyClaimed[T any](rows []T, err error, id func(T) pgtype.UUID) (string, error) {
	if err != nil || len(rows) != 1 {
		return "", fmt.Errorf("rows=%v: %w", rows, err)
	}
	return uuidText(id(rows[0])), nil
}

func claimArtifactPastRetention(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListArtifactsPastRetention(ctx, gen.ListArtifactsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListArtifactsPastRetentionRow) pgtype.UUID { return r.ID })
}

func claimRunOutputPastRetention(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListRunOutputsPastRetention(ctx, gen.ListRunOutputsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListRunOutputsPastRetentionRow) pgtype.UUID { return r.ID })
}

func claimArtifactObject(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListArtifactsClaimingObject(ctx, gen.ListArtifactsClaimingObjectParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListArtifactsClaimingObjectRow) pgtype.UUID { return r.ID })
}

func claimDatasetPastRetention(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListDatasetsPastRetention(ctx, gen.ListDatasetsPastRetentionParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListDatasetsPastRetentionRow) pgtype.UUID { return r.ID })
}

func claimDatasetObject(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListDatasetsClaimingObject(ctx, gen.ListDatasetsClaimingObjectParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListDatasetsClaimingObjectRow) pgtype.UUID { return r.ID })
}

func claimUploadCleanupIntent(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListDatasetCleanupIntents(ctx, gen.ListDatasetCleanupIntentsParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListDatasetCleanupIntentsRow) pgtype.UUID { return r.ID })
}

func claimAccountPastGrace(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListAccountsPastGrace(ctx, gen.ListAccountsPastGraceParams{ClaimLease: sweepClaim(), BatchSize: 1, Cutoff: pgconv.Timestamptz(time.Now())})
	return onlyClaimed(rows, err, func(id pgtype.UUID) pgtype.UUID { return id })
}

func claimPendingEnrichment(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListPendingEnrichment(ctx, gen.ListPendingEnrichmentParams{ClaimLease: sweepClaim(), BatchSize: 1})
	return onlyClaimed(rows, err, func(r gen.ListPendingEnrichmentRow) pgtype.UUID { return r.SkillID })
}

func claimActiveRun(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListActiveRuns(ctx, run.ActiveRunClaim(1))
	return onlyClaimed(rows, err, func(r gen.Run) pgtype.UUID { return r.ID })
}

func claimRunNeedingCleanup(ctx context.Context, q *gen.Queries) (string, error) {
	rows, err := q.ListRunsNeedingCleanup(ctx, run.CleanupClaim(1))
	return onlyClaimed(rows, err, func(r gen.Run) pgtype.UUID { return r.ID })
}

func (w *leaseWorld) insertArtifact(t *testing.T, kind, key string, expires time.Time, runID *pgtype.UUID) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `INSERT INTO artifacts
			(workspace_id, run_id, kind, file_name, content_type, size_bytes, content_hash,
			 object_key, scan_status, expires_at, created_at)
			VALUES ($1, $2, $3, $4, 'application/octet-stream', 1, $4, $4,
			        'available', $5, $6) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), runID, kind, key, expires, w.base).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *leaseWorld) minutesAfterBase(i int) time.Time {
	return w.base.Add(time.Duration(i) * time.Minute)
}

func (w *leaseWorld) artifactWorklistsLease(t *testing.T) {
	artifactRetention := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := w.insertArtifact(t, "download_package", fmt.Sprintf("%s-artifact-expired-%d", w.tag, i), w.minutesAfterBase(i), nil)
		artifactRetention[id] = true
	}
	assertConcurrentClaims(t, w.pool, "artifact retention", artifactRetention, claimArtifactPastRetention)

	runOutputRetention := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := w.insertArtifact(t, "run_output", fmt.Sprintf("%s-run-output-%d", w.tag, i), w.minutesAfterBase(i), &w.outputHost)
		runOutputRetention[id] = true
	}
	assertConcurrentClaims(t, w.pool, "run-output retention", runOutputRetention, claimRunOutputPastRetention)

	artifactReconcile := map[string]bool{}
	for i := 0; i < 2; i++ {
		id := w.insertArtifact(t, "download_package", fmt.Sprintf("%s-artifact-live-%d", w.tag, i), time.Now().Add(time.Hour), nil)
		artifactReconcile[id] = true
	}
	assertConcurrentClaims(t, w.pool, "artifact reconciliation", artifactReconcile, claimArtifactObject)
}

func (w *leaseWorld) insertDataset(t *testing.T, key string, expires time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `INSERT INTO datasets
			(workspace_id, test_case_id, file_name, content_type, size_bytes,
			 content_hash, object_key, expires_at, created_at)
			VALUES ($1, $2, $3, 'text/plain', 1, $3, $3, $4, $5) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.testCaseID), key, expires, w.base).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *leaseWorld) datasetWorklistsLease(t *testing.T) {
	datasetRetention := map[string]bool{}
	datasetReconcile := map[string]bool{}
	for i := 0; i < 2; i++ {
		datasetRetention[w.insertDataset(t, fmt.Sprintf("%s-dataset-expired-%d", w.tag, i), w.minutesAfterBase(i))] = true
		datasetReconcile[w.insertDataset(t, fmt.Sprintf("%s-dataset-live-%d", w.tag, i), time.Now().Add(time.Hour))] = true
	}
	assertConcurrentClaims(t, w.pool, "dataset retention", datasetRetention, claimDatasetPastRetention)
	assertConcurrentClaims(t, w.pool, "dataset reconciliation", datasetReconcile, claimDatasetObject)
}

func (w *leaseWorld) uploadCleanupLeases(t *testing.T) {
	cleanupIntents := map[string]bool{}
	for i := 0; i < 2; i++ {
		var id string
		if err := w.pool.QueryRow(context.Background(), `INSERT INTO dataset_object_cleanup_intents
			(workspace_id, object_key, not_before) VALUES ($1, $2, $3) RETURNING id::text`,
			mustUUID(t, w.f.workspaceID), fmt.Sprintf("%s-intent-%d", w.tag, i), w.minutesAfterBase(i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		cleanupIntents[id] = true
	}
	assertConcurrentClaims(t, w.pool, "upload cleanup", cleanupIntents, claimUploadCleanupIntent)
}

func (w *leaseWorld) accountPurgeLeases(t *testing.T, a *api) {
	accounts := map[string]bool{}
	for i := 0; i < 2; i++ {
		account := a.login(t, fmt.Sprintf("%s-account-%d", w.tag, i))
		accounts[account.userID] = true
		if _, err := w.pool.Exec(context.Background(), `UPDATE users SET deletion_requested_at = $2,
			purge_attempted_at = NULL, purge_started_at = NULL WHERE id = $1`,
			mustUUID(t, account.userID), w.minutesAfterBase(i)); err != nil {
			t.Fatal(err)
		}
	}
	assertConcurrentClaims(t, w.pool, "account purge", accounts, claimAccountPastGrace)
}

func (w *leaseWorld) enrichmentLeases(t *testing.T) {
	secondSkill := seedSkill(t, w.pool, w.f.workspaceID, w.tag+"-second-skill")
	seedVersion(t, w.pool, w.f.workspaceID, secondSkill, w.tag+"-second-version")
	enrichment := map[string]bool{w.f.skillID: true, secondSkill: true}
	if _, err := w.pool.Exec(context.Background(), `UPDATE search_documents SET enrichment_status = 'pending',
		enrichment_attempted_at = NULL, updated_at = CASE skill_id WHEN $1 THEN $3::timestamptz ELSE $4::timestamptz END
		WHERE skill_id IN ($1, $2)`, mustUUID(t, w.f.skillID), mustUUID(t, secondSkill), w.base, w.base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	assertConcurrentClaims(t, w.pool, "enrichment", enrichment, claimPendingEnrichment)
}

func (w *leaseWorld) insertRun(t *testing.T, status string, finished *time.Time) string {
	t.Helper()
	var id string
	if err := w.pool.QueryRow(context.Background(), `INSERT INTO runs
			(workspace_id, skill_version_id, test_case_snapshot_id, status, provider,
			 finished_at, cleanup_status, created_at)
			VALUES ($1, $2, $3, $4, 'test', $5, 'pending', $6) RETURNING id::text`,
		mustUUID(t, w.f.workspaceID), mustUUID(t, w.f.versionID), w.snapshotID, status, finished, w.base).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (w *leaseWorld) activeRunSupervisionLeasesAndReopens(t *testing.T) {
	ctx := context.Background()
	activeFirst := w.insertRun(t, "queued", nil)
	activeSecond := w.insertRun(t, "queued", nil)
	active := map[string]bool{activeFirst: true, activeSecond: true}
	if _, err := w.pool.Exec(ctx, `UPDATE runs SET supervision_checked_at = '-infinity', created_at = '-infinity'
		WHERE id IN ($1, $2)`, mustUUID(t, activeFirst), mustUUID(t, activeSecond)); err != nil {
		t.Fatal(err)
	}
	assertConcurrentClaims(t, w.pool, "active-run supervision", active, claimActiveRun)
	if _, err := w.pool.Exec(ctx, `UPDATE runs SET supervision_checked_at = now() - interval '31 seconds'
		WHERE id = $1`, mustUUID(t, activeFirst)); err != nil {
		t.Fatal(err)
	}
	if rows, err := gen.New(w.pool).ListActiveRuns(ctx, run.ActiveRunClaim(1)); err != nil || len(rows) != 1 || uuidText(rows[0].ID) != activeFirst {
		t.Fatalf("active-run lease did not reopen after one supervisor interval: rows=%v err=%v", rows, err)
	}
}

func (w *leaseWorld) runCleanupLeasesReopensAndWaitsForTheRescueWindow(t *testing.T) {
	ctx := context.Background()
	finished := w.base
	cleanup := map[string]bool{w.insertRun(t, "failed", &finished): true, w.insertRun(t, "failed", &finished): true}
	assertConcurrentClaims(t, w.pool, "run cleanup", cleanup, claimRunNeedingCleanup)
	var cleanupID string
	for id := range cleanup {
		cleanupID = id
		break
	}
	if _, err := w.pool.Exec(ctx, `UPDATE runs SET cleanup_attempted_at = now() - interval '31 seconds'
		WHERE id = $1`, mustUUID(t, cleanupID)); err != nil {
		t.Fatal(err)
	}
	if rows, err := gen.New(w.pool).ListRunsNeedingCleanup(ctx, run.CleanupClaim(1)); err != nil || len(rows) != 1 || uuidText(rows[0].ID) != cleanupID {
		t.Fatalf("cleanup lease did not reopen after one supervisor interval: rows=%v err=%v", rows, err)
	}
	justFinished := time.Now()
	w.insertRun(t, "failed", &justFinished)
	if rows, err := gen.New(w.pool).ListRunsNeedingCleanup(ctx, run.CleanupClaim(1)); err != nil || len(rows) != 0 {
		t.Fatalf("a run that finished a moment ago was rescued before its own cleanup could run: rows=%v err=%v", rows, err)
	}
	pastRescue := time.Now().Add(-65 * time.Second)
	settled := w.insertRun(t, "failed", &pastRescue)
	if rows, err := gen.New(w.pool).ListRunsNeedingCleanup(ctx, run.CleanupClaim(1)); err != nil || len(rows) != 1 || uuidText(rows[0].ID) != settled {
		t.Fatalf("a run finished past the rescue window was not rescued: rows=%v err=%v", rows, err)
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
	svc := &packaging.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
	n, err := objreconcile.PurgeExpired(ctx, pool, store, objreconcile.RetentionOwner{
		List: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return candidates, nil
		}, Mark: svc.MarkArtifactPurged,
	}, 10)
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

	// One connection: a nested transaction opened while the guard transaction is
	// still held would starve waiting for a connection that never frees up.
	cfg := pool.Config().Copy()
	cfg.MaxConns = 1
	single, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	store := &countingObjectStore{}
	svc := &packaging.Service{Pool: single, ClearSightings: objreconcile.ClearArtifactSightings}
	n, err := objreconcile.PurgeExpired(ctx, single, store, objreconcile.RetentionOwner{
		List: func(context.Context, int32) ([]objreconcile.Candidate, error) {
			return []objreconcile.Candidate{expired}, nil
		}, Mark: svc.MarkArtifactPurged, Guard: svc.GuardArtifactRemoval,
	}, 1)
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
	svc := &packaging.Service{Pool: pool, ClearSightings: objreconcile.ClearArtifactSightings}
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

func TestEnrichmentClaimsTheExactVersionAndLeavesVersionlessSkillsUnclaimed(t *testing.T) {
	pool := requireDB(t)
	shelveExistingWorklists(t, pool)
	a := newAPI(t, pool)
	ctx := context.Background()
	owner := a.login(t, uniqueWorklistLabel("versionless-owner"))
	versionless := seedSkill(t, pool, owner.workspaceID, uniqueWorklistLabel("versionless"))
	versioned := newFixture(t, a, pool, uniqueWorklistLabel("versioned"))
	for _, skill := range []string{versionless, versioned.skillID} {
		if _, err := pool.Exec(ctx, `UPDATE search_documents
			SET enrichment_status = 'pending', enrichment_attempted_at = NULL,
			    updated_at = '1700-01-01' WHERE skill_id = $1`, mustUUID(t, skill)); err != nil {
			t.Fatal(err)
		}
	}
	claimed, err := wiring.NewCatalogService(pool).PendingEnrichments(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, row := range claimed {
		ids = append(ids, uuidText(row.SkillID))
		if uuidText(row.SkillID) == versioned.skillID && uuidText(row.VersionID) != versioned.versionID {
			t.Errorf("claimed version %s, want %s", uuidText(row.VersionID), versioned.versionID)
		}
	}
	if !contains(ids, versioned.skillID) {
		t.Fatalf("the pending skill with a version was not claimed: %v", ids)
	}
	if contains(ids, versionless) {
		t.Errorf("claimed a skill with no version, which has no package to enrich: %v", ids)
	}
}
