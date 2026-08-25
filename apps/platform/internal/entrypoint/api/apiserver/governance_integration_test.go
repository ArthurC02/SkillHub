// Database-backed tests for the M1 governance trio: account deletion and the
// Artifact purge (CORE-007), the audit trail (CORE-008), and manual takedown
// (INGEST-010). Same harness rules as authz_integration_test.go: they need
// SKILLHUB_TEST_DATABASE_URL and skip without it.
package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

// recordingStore stands in for object storage: the purge has to reach it, and
// which keys it reached is the assertion.
type recordingStore struct{ removed []string }

func (s *recordingStore) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	return nil
}

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

func countRow(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seedVersion gives a seeded skill the immutable version row the purge has to
// reason about.
func seedVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, hash string) gen.SkillVersion {
	t.Helper()
	v, err := gen.New(pool).CreateSkillVersion(context.Background(), gen.CreateSkillVersionParams{
		WorkspaceID:      mustUUID(t, workspaceID),
		SkillID:          mustUUID(t, skillID),
		ContentHash:      hash,
		PackageObjectKey: "packages/" + hash + ".zip",
		Manifest:         []byte(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func makeCatalog(t *testing.T, pool *pgxpool.Pool, workspaceID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"UPDATE workspaces SET is_catalog = true WHERE id = $1", mustUUID(t, workspaceID))
	if err != nil {
		t.Fatal(err)
	}
}

func postJSON(t *testing.T, c *client, path, body string) (int, map[string]any) {
	t.Helper()
	resp, err := c.Post(c.base+path, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func deleteJSON(t *testing.T, c *client, path string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, c.base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// CORE-007: the grace period is real time in which nothing has happened yet and
// the user can still change their mind.
func TestAccountDeletionGraceIsCancellable(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-grace")

	status, body := deleteJSON(t, alice, "/me")
	if status != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", status)
	}
	// WS-002/PDM-006 6.1: the scope has to be stated before the deletion, and
	// it has to admit that forked versions survive without the user's identity.
	if scope, _ := body["scope"].(string); !strings.Contains(scope, "forked") {
		t.Fatalf("DELETE /me did not state the deletion scope: %v", body["scope"])
	}
	if body["purge_after"] == "" || body["purge_after"] == nil {
		t.Fatal("DELETE /me did not say when the purge happens")
	}

	// The account stays usable during the grace period — otherwise there is no
	// way back in to cancel.
	if alice.me(t)["user_id"] != alice.userID {
		t.Fatal("account became unusable during the grace period")
	}

	// The app's own identity service, not a fresh one: the purge only runs when
	// all six owning contexts' steps have been injected, and NewApp is where
	// that happens (ADR-034).
	svc := a.auth.Service
	// Nothing is due yet under the real 30 day policy.
	if n, err := svc.PurgeExpiredAccounts(context.Background(), &recordingStore{}, identity.AccountDeletionGrace, 100); err != nil || n != 0 {
		t.Fatalf("purge inside the grace period: purged %d, err %v", n, err)
	}

	if status, _ := postJSON(t, alice, "/me/deletion/cancel", "{}"); status != http.StatusOK {
		t.Fatalf("cancel: got %d", status)
	}
	// Zero grace makes every outstanding request due; a cancelled one is not a
	// request any more, so it must still be untouched.
	if n, err := svc.PurgeExpiredAccounts(context.Background(), &recordingStore{}, 0, 100); err != nil {
		t.Fatal(err)
	} else if n != 0 {
		t.Fatalf("cancelled request was purged anyway (%d accounts)", n)
	}
	if alice.me(t)["user_id"] != alice.userID {
		t.Fatal("account gone after cancelling the deletion")
	}
}

// CORE-007 / PDM-006 6.1: private content really goes, referenced immutable
// versions really stay, and nothing left behind names the user.
func TestAccountPurgeHardDeletesPrivateContentAndDeIdentifiesTheRest(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	alice := a.login(t, "alice-purge")
	bob := a.login(t, "bob-purge")
	// Alice's workspace is the catalog one here purely so Bob has something he
	// is allowed to fork; the fork is what makes a version "referenced".
	makeCatalog(t, pool, alice.workspaceID)

	private := seedSkill(t, pool, alice.workspaceID, "alice-private")
	// The private version gets an import source so this test also covers the one
	// ordering constraint between the purge steps: ingest only removes sources
	// that no skill_versions row still points at, so it has to run after registry
	// deletes those rows. Reverse the two and this source survives, silently
	// (see identity.purgeSteps).
	var sourceID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO skill_sources (workspace_id, source_type, source_url, content_hash, fetched_at)
		VALUES ($1, 'git', 'https://example.invalid/alice.git', 'hash-private', now()) RETURNING id`,
		mustUUID(t, alice.workspaceID)).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	if _, err := gen.New(pool).CreateSkillVersion(ctx, gen.CreateSkillVersionParams{
		WorkspaceID:      mustUUID(t, alice.workspaceID),
		SkillID:          mustUUID(t, private),
		SourceID:         sourceID,
		ContentHash:      "hash-private",
		PackageObjectKey: "packages/hash-private.zip",
		Manifest:         []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}
	shared := seedSkill(t, pool, alice.workspaceID, "alice-shared")
	sharedVer := seedVersion(t, pool, alice.workspaceID, shared, "hash-shared")

	if status, _ := postJSON(t, bob, "/skills/"+shared+"/fork", "{}"); status != http.StatusCreated {
		t.Fatalf("bob fork of alice's catalog skill: got %d", status)
	}

	// Uploaded private content with objects behind it (TEST-002, PACK-001).
	var testCaseID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
		VALUES ($1, $2, 'tc', 'do the thing') RETURNING id`,
		mustUUID(t, alice.workspaceID), mustUUID(t, shared)).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO datasets (workspace_id, test_case_id, file_name, content_type,
		                      size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'input.csv', 'text/csv', 10, 'h1', 'datasets/alice.csv', now() + interval '90 days')`,
		mustUUID(t, alice.workspaceID), testCaseID); err != nil {
		t.Fatal(err)
	}
	var snapshotID, runID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots
			(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'purge me', '[]'::jsonb, 'purge-snapshot') RETURNING id`,
		mustUUID(t, alice.workspaceID), testCaseID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider)
		VALUES ($1, $2, $3, 'purge-test') RETURNING id`,
		mustUUID(t, alice.workspaceID), sharedVer.ID, snapshotID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (workspace_id, run_id, kind, file_name, content_type,
		                       size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, NULL, 'download_package', 'pkg.zip', 'application/zip', 10, 'h2',
		        'artifacts/alice.zip', now() + interval '30 days'),
		       ($1, $2, 'run_output', 'run.zip', 'application/zip', 10, 'h3',
		        'artifacts/alice.zip', now() + interval '30 days')`,
		mustUUID(t, alice.workspaceID), runID); err != nil {
		t.Fatal(err)
	}

	if status, _ := deleteJSON(t, alice, "/me"); status != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", status)
	}
	store := &recordingStore{}
	svc := a.auth.Service
	n, err := svc.PurgeExpiredAccounts(ctx, store, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("purged %d accounts, want 1", n)
	}

	// Unreferenced private content is gone, files included.
	if c := countRow(t, pool, "SELECT count(*) FROM skills WHERE id = $1", mustUUID(t, private)); c != 0 {
		t.Fatal("unreferenced private skill survived the purge")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE content_hash = 'hash-private'"); c != 0 {
		t.Fatal("unreferenced private version survived the purge")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM search_documents WHERE skill_id = $1", mustUUID(t, private)); c != 0 {
		t.Fatal("search document of a purged skill survived")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_sources WHERE id = $1", sourceID); c != 0 {
		t.Fatal("import source of a purged version survived; the purge steps ran out of order")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM datasets WHERE workspace_id = $1", mustUUID(t, alice.workspaceID)); c != 0 {
		t.Fatal("dataset rows survived the purge")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM artifacts WHERE workspace_id = $1", mustUUID(t, alice.workspaceID)); c != 0 {
		t.Fatal("artifact rows survived the purge")
	}
	// By name and not by row count. Three owners list their own keys, and identity
	// de-duplicates them before removal: the run output and download package above
	// deliberately share one content-addressed key.
	removed := map[string]bool{}
	for _, k := range store.removed {
		removed[k] = true
	}
	for _, want := range []string{"datasets/alice.csv", "artifacts/alice.zip"} {
		if !removed[want] {
			t.Errorf("object %q survived the purge; keys removed were %v", want, store.removed)
		}
	}
	if len(store.removed) != 2 {
		t.Fatalf("object storage keys removed: %v, want exactly the dataset and the artifact", store.removed)
	}

	// Referenced content stays intact for the third party who depends on it.
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE id = $1", sharedVer.ID); c != 1 {
		t.Fatal("a version another user forked was deleted; that breaks their provenance chain")
	}
	if ids := bob.skillIDs(t, "/skills"); len(ids) == 0 {
		t.Fatal("bob's fork disappeared with alice's account")
	}

	// ...but it no longer leads back to Alice.
	var email, name string
	if err := pool.QueryRow(ctx, "SELECT email, display_name FROM users WHERE id = $1",
		mustUUID(t, alice.userID)).Scan(&email, &name); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(email, "alice-purge") || strings.Contains(name, "alice-purge") {
		t.Fatalf("purged account is still identifiable: %s / %s", email, name)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM user_identities WHERE user_id = $1", mustUUID(t, alice.userID)); c != 0 {
		t.Fatal("external identity survived; the account could be logged into again")
	}
	if got := alice.status(t, http.MethodGet, "/me"); got != http.StatusUnauthorized {
		t.Fatalf("session survived the purge: GET /me got %d", got)
	}
	if c := countRow(t, pool,
		"SELECT count(*) FROM audit_events WHERE actor_user_id = $1 AND action = 'account.purged'",
		mustUUID(t, alice.userID)); c != 1 {
		t.Fatal("the purge left no audit event")
	}

	// Iron rule 9: re-running finds nothing left to do and changes nothing.
	if again, err := svc.PurgeExpiredAccounts(ctx, store, 0, 100); err != nil || again != 0 {
		t.Fatalf("second purge run: purged %d, err %v", again, err)
	}
}

// The claim ADR-034 rests on: an account deletion is one transaction, all of it
// or none of it. That guarantee is the whole reason the ADR refused to
// event-source this purge, and it is what CORE-007 promises a user who asks to
// be forgotten. Since DDD-026 the owning contexts' steps are injected
// function values, so nothing about the type system enforces it any more: an
// owner whose PurgeWorkspace opens its own transaction, or logs its error and
// returns nil, would make a partial deletion happen quietly. Taking the caller's
// pgx.Tx makes that awkward to write; a closure holding a pool makes it
// possible anyway.
//
// So this test is the guarantee. Deleting it means giving up the argument that
// kept account deletion atomic — say so in the ADR before you do.
func TestAccountPurgeRollsBackEveryContextWhenOneStepFails(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)
	alice := a.login(t, "alice-rollback")

	skill := seedSkill(t, pool, alice.workspaceID, "alice-rollback")
	seedVersion(t, pool, alice.workspaceID, skill, "hash-rollback")
	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (workspace_id, kind, file_name, content_type,
		                       size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, 'download_package', 'pkg.zip', 'application/zip', 10, 'h-rollback',
		        'artifacts/rollback.zip', now() + interval '30 days')`,
		mustUUID(t, alice.workspaceID)); err != nil {
		t.Fatal(err)
	}
	if status, _ := deleteJSON(t, alice, "/me"); status != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", status)
	}

	svc := a.auth.Service
	// ingest is last in identity's ordered step list, so by the time this fires
	// the other four have all written inside the transaction. Replacing an
	// injected field is the whole point of the injection: no production code has
	// to grow a test hook for this.
	svc.PurgeImportSources = func(context.Context, pgx.Tx, pgtype.UUID) error {
		return errors.New("simulated failure inside an owner's purge step")
	}
	// The failure has to reach the caller. A failing account is still logged and
	// skipped so it cannot strand the rest of the batch, but the batch reports
	// it: returning nil made a run in which every account failed look exactly
	// like a run with nothing to purge, and `maintenance purge-accounts` exited
	// 0 either way.
	// The count is not asserted because the batch is workspace-wide and other
	// tests share this database; the row-level assertions below are stricter
	// anyway — they name this account.
	n, err := svc.PurgeExpiredAccounts(ctx, &recordingStore{}, 0, 100)
	if err == nil {
		t.Error("a failing purge step was swallowed; maintenance purge-accounts would exit 0")
	}
	t.Logf("PurgeExpiredAccounts after a failing step: purged=%d err=%v", n, err)

	// Everything the steps before the failure wrote has to be gone with it.
	if c := countRow(t, pool, "SELECT count(*) FROM skills WHERE id = $1", mustUUID(t, skill)); c != 1 {
		t.Error("the registry step's delete was committed although a later step failed")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE skill_id = $1", mustUUID(t, skill)); c != 1 {
		t.Error("skill_versions were hard deleted although a later step failed")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM artifacts WHERE workspace_id = $1",
		mustUUID(t, alice.workspaceID)); c != 1 {
		t.Error("the run step's delete was committed although a later step failed")
	}
	// ...and so does identity's own de-identification. Without this half the
	// assertions above would also pass if the failing step had merely been
	// skipped, which is the failure mode being ruled out: skipped means the rest
	// committed, rolled back means nothing did.
	var email string
	if err := pool.QueryRow(ctx, "SELECT email FROM users WHERE id = $1",
		mustUUID(t, alice.userID)).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(email, "alice-rollback") {
		t.Fatalf("the user row was de-identified even though the purge failed: %s", email)
	}

	// The rollback also has to leave the account purgeable, or "retry on the next
	// run" is not the recovery the fail-closed design claims it is.
	svc.PurgeImportSources = (&ingest.Service{Pool: pool}).PurgeWorkspace
	if _, err := svc.PurgeExpiredAccounts(ctx, &recordingStore{}, 0, 100); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT email FROM users WHERE id = $1",
		mustUUID(t, alice.userID)).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(email, "alice-rollback") {
		t.Fatalf("the retry after the rollback did not purge the account: %s", email)
	}
}

// INGEST-010: a taken-down skill leaves the public surface and stays gone
// across a full reindex, while every row it owns is retained.
func TestTakedownRemovesSkillFromPublicSurface(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	operator := a.login(t, "operator-takedown")
	makeCatalog(t, pool, operator.workspaceID)
	other := a.login(t, "reader-takedown")

	skillID := seedSkill(t, pool, operator.workspaceID, "quarantined-parser")
	seedVersion(t, pool, operator.workspaceID, skillID, "hash-takedown")

	if ids := other.skillIDs(t, "/api/skills/search?q=quarantined-parser"); !contains(ids, skillID) {
		t.Fatal("precondition failed: the skill is not in public search before the takedown")
	}

	if status, _ := postJSON(t, operator, "/skills/"+skillID+"/takedown", `{"reason":""}`); status != http.StatusBadRequest {
		t.Fatalf("takedown without a reason: got %d, want 400", status)
	}
	status, body := postJSON(t, operator, "/skills/"+skillID+"/takedown",
		`{"reason":"upstream repository was deleted"}`)
	if status != http.StatusOK {
		t.Fatalf("takedown: got %d (%v)", status, body)
	}

	if ids := other.skillIDs(t, "/api/skills/search?q=quarantined-parser"); contains(ids, skillID) {
		t.Fatal("taken-down skill is still in public search")
	}
	// The URL was public and is still in circulation: 410, not 404 — withdrawn
	// is a different fact from never existed.
	if got := other.status(t, http.MethodGet, "/api/skills/"+skillID); got != http.StatusGone {
		t.Fatalf("detail of a taken-down skill: got %d, want 410", got)
	}
	// A takedown is not a delete: the content is retained (PDM-006 §6).
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE content_hash = 'hash-takedown'"); c != 1 {
		t.Fatal("takedown deleted the version snapshot")
	}
	// And it stops being a fork source.
	if s, _ := postJSON(t, other, "/skills/"+skillID+"/fork", "{}"); s != http.StatusNotFound {
		t.Fatalf("fork of a taken-down skill: got %d, want 404", s)
	}
	// Taking the same skill down twice is a conflict, not a silent success.
	if s, _ := postJSON(t, operator, "/skills/"+skillID+"/takedown", `{"reason":"again"}`); s != http.StatusConflict {
		t.Fatalf("repeat takedown: got %d, want 409", s)
	}

	// The projection rebuild must not resurrect it — that is what makes the
	// takedown durable rather than a one-off delete of one row.
	q := gen.New(pool)
	if _, err := q.PruneDeletedSearchDocuments(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ReindexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if ids := other.skillIDs(t, "/api/skills/search?q=quarantined-parser"); contains(ids, skillID) {
		t.Fatal("reindex put the taken-down skill back into public search")
	}

	if c := countRow(t, pool,
		"SELECT count(*) FROM audit_events WHERE action = 'skill.takedown' AND resource_id = $1",
		mustUUID(t, skillID)); c != 1 {
		t.Fatal("takedown left no audit event")
	}
}

// CORE-008: the operations NFR-001 names leave a trail, in the same transaction
// as the change itself.
func TestKeyOperationsLeaveAuditEvents(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	owner := a.login(t, "owner-audit") // auth.login
	makeCatalog(t, pool, owner.workspaceID)
	forker := a.login(t, "forker-audit")

	skillID := seedSkill(t, pool, owner.workspaceID, "audited-skill")
	seedVersion(t, pool, owner.workspaceID, skillID, "hash-audit")

	if s, _ := postJSON(t, forker, "/skills/"+skillID+"/fork", "{}"); s != http.StatusCreated {
		t.Fatalf("fork: got %d", s)
	}
	if s, _ := postJSON(t, owner, "/skills/"+skillID+"/takedown", `{"reason":"license unclear"}`); s != http.StatusOK {
		t.Fatalf("takedown: got %d", s)
	}
	deletable := seedSkill(t, pool, owner.workspaceID, "deletable-skill")
	if s, _ := deleteJSON(t, owner, "/skills/"+deletable); s != http.StatusOK {
		t.Fatalf("skill delete: got %d", s)
	}
	if s, _ := deleteJSON(t, owner, "/me"); s != http.StatusOK {
		t.Fatalf("deletion request: got %d", s)
	}
	if s, _ := postJSON(t, owner, "/me/deletion/cancel", "{}"); s != http.StatusOK {
		t.Fatalf("deletion cancel: got %d", s)
	}
	if s, _ := postJSON(t, owner, "/auth/logout", "{}"); s != http.StatusNoContent {
		t.Fatalf("logout: got %d", s)
	}

	for _, action := range []string{
		"auth.login", "auth.logout", "skill.fork", "skill.takedown", "skill.delete",
		"account.deletion_requested", "account.deletion_cancelled",
	} {
		actor := owner.userID
		if action == "skill.fork" {
			actor = forker.userID
		}
		if c := countRow(t, pool,
			"SELECT count(*) FROM audit_events WHERE action = $1 AND actor_user_id = $2",
			action, mustUUID(t, actor)); c == 0 {
			t.Errorf("no audit event for %s", action)
		}
	}

	// Append-only: the trail cannot be edited by anything the application can do
	// (0005/0013 trigger).
	if _, err := pool.Exec(ctx,
		"UPDATE audit_events SET action = 'tampered' WHERE actor_user_id = $1",
		mustUUID(t, owner.userID)); err == nil {
		t.Fatal("audit events are updatable; the trail is not evidence")
	}
	if _, err := pool.Exec(ctx,
		"DELETE FROM audit_events WHERE actor_user_id = $1", mustUUID(t, owner.userID)); err == nil {
		t.Fatal("audit events are deletable outside the retention purge")
	}
}

// WS-005 / PDM-006 §6.1 / 04 丙-63: the grace purge for a skill the user deleted
// on its own. The assertions that matter here are the four about what the sweep
// must NOT take: hard deleting a version another workspace forked, or that a run
// points at, destroys a third party's provenance chain (DISC-003) and punishes
// somebody who never asked for anything -- and unlike the row it deletes
// correctly, that damage has no way back.
//
// Each retained skill is retained for exactly one reason: the test case is on
// `tested` and the run is on `used`'s version, so a single exclusion breaking
// shows up as a single failure rather than being covered by its neighbour.
func TestDeletedSkillPurgeTakesOnlyWhatIsPastGraceAndUnreferenced(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	alice := a.login(t, "alice-skillgrace")
	bob := a.login(t, "bob-skillgrace")
	// Alice's workspace is the catalog one only so Bob has something he is
	// allowed to fork; the fork is what makes a version referenced.
	makeCatalog(t, pool, alice.workspaceID)

	past := seedSkill(t, pool, alice.workspaceID, "grace-past")
	seedVersion(t, pool, alice.workspaceID, past, "grace-past-hash")
	recent := seedSkill(t, pool, alice.workspaceID, "grace-recent")
	seedVersion(t, pool, alice.workspaceID, recent, "grace-recent-hash")
	forked := seedSkill(t, pool, alice.workspaceID, "grace-forked")
	seedVersion(t, pool, alice.workspaceID, forked, "grace-forked-hash")
	used := seedSkill(t, pool, alice.workspaceID, "grace-used")
	usedVer := seedVersion(t, pool, alice.workspaceID, used, "grace-used-hash")
	tested := seedSkill(t, pool, alice.workspaceID, "grace-tested")
	seedVersion(t, pool, alice.workspaceID, tested, "grace-tested-hash")

	if status, _ := postJSON(t, bob, "/skills/"+forked+"/fork", "{}"); status != http.StatusCreated {
		t.Fatalf("bob fork of alice's catalog skill: got %d", status)
	}
	var testCaseID, snapshotID, runID pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
		VALUES ($1, $2, 'grace-tc', 'do the thing') RETURNING id`,
		mustUUID(t, alice.workspaceID), mustUUID(t, tested)).Scan(&testCaseID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots
			(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'do the thing', '[]'::jsonb, 'grace-snapshot') RETURNING id`,
		mustUUID(t, alice.workspaceID), testCaseID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider)
		VALUES ($1, $2, $3, 'grace-test') RETURNING id`,
		mustUUID(t, alice.workspaceID), usedVer.ID, snapshotID).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	// Deleted through the real endpoint, because the row state this sweep reads
	// is whatever that endpoint leaves behind.
	for _, id := range []string{past, recent, forked, used, tested} {
		if status, _ := deleteJSON(t, alice, "/skills/"+id); status != http.StatusOK {
			t.Fatalf("DELETE /skills/%s: got %d", id, status)
		}
	}
	// Four of them are moved past any plausible grace period; `recent` is not,
	// and that difference is the whole predicate.
	for _, id := range []string{past, forked, used, tested} {
		if _, err := pool.Exec(ctx,
			"UPDATE skills SET deleted_at = now() - interval '40 days' WHERE id = $1",
			mustUUID(t, id)); err != nil {
			t.Fatal(err)
		}
	}

	// The same object cmd/maintenance builds for this job: one field, no
	// projection writes, because the purge needs neither.
	svc := &registry.Service{Pool: pool}

	// Fail closed before anything else: an unset window must not mean "now".
	if _, err := svc.PurgeDeletedSkills(ctx, 0, 100); err == nil {
		t.Fatal("a zero grace period was accepted; every deletion would be purged instantly")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skills WHERE id = $1", mustUUID(t, recent)); c != 1 {
		t.Fatal("the refused zero-grace sweep deleted rows anyway")
	}

	sweep, err := svc.PurgeDeletedSkills(ctx, 30*24*time.Hour, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Errorf and not Fatalf: when this number is wrong, the rows below say which
	// way it was wrong, and that is the difference between "the sweep took
	// nothing" and "the sweep took somebody's forked version".
	if sweep.Purged != 1 {
		t.Errorf("purged %d skills, want exactly the one past grace with nothing depending on it", sweep.Purged)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skills WHERE id = $1", mustUUID(t, past)); c != 0 {
		t.Error("a skill deleted long past the grace period survived the sweep")
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE skill_id = $1", mustUUID(t, past)); c != 0 {
		t.Error("the versions of a purged skill survived; the 0013 purge flag did not reach skill_versions")
	}

	// What must not go, one case per reason.
	for _, keep := range []struct {
		id, why string
	}{
		{recent, "deleted inside the grace period"},
		{forked, "another workspace forked it"},
		{used, "a run used one of its versions"},
		{tested, "it still holds test cases"},
	} {
		if c := countRow(t, pool, "SELECT count(*) FROM skills WHERE id = $1", mustUUID(t, keep.id)); c != 1 {
			t.Errorf("skill was purged although %s", keep.why)
		}
		if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE skill_id = $1", mustUUID(t, keep.id)); c != 1 {
			t.Errorf("frozen versions were purged although %s", keep.why)
		}
	}
	if c := countRow(t, pool, "SELECT count(*) FROM runs WHERE id = $1", runID); c != 1 {
		t.Error("the run whose version the sweep had to spare is gone")
	}

	// The two counts the operator reads instead of guessing. Compared with >=
	// rather than = because this database is shared with every other test in the
	// package and several of them soft-delete skills too; what these assert is
	// that the sweep classified this test's four survivors correctly -- three as
	// kept forever by the provenance rule, one as still waiting out its grace.
	if sweep.Kept < 3 {
		t.Errorf("kept = %d, want at least the fork, the run and the test cases", sweep.Kept)
	}
	if sweep.Waiting < 1 {
		t.Errorf("waiting = %d, want at least the skill deleted a moment ago", sweep.Waiting)
	}

	// Iron rule 9: running it again finds nothing left to do and takes nothing else.
	again, err := svc.PurgeDeletedSkills(ctx, 30*24*time.Hour, 100)
	if err != nil {
		t.Fatal(err)
	}
	if again.Purged != 0 {
		t.Errorf("second sweep purged %d skills; the first one did not finish or the second took survivors", again.Purged)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE id = $1", usedVer.ID); c != 1 {
		t.Error("the version a run points at was taken by the second sweep")
	}
}

// enqueueObjectKeys fills the worklist by hand.
//
// NOT a stand-in for the producer any more: the purge statements enqueue their
// own keys in an `enqueued` CTE (04 丙-73), and the orphan below arrives that
// way. What is left is the case the producer cannot reach -- an object orphaned
// before 0039 existed, which no row in the database knows about. Filling the
// queue by hand is the operator's only recovery path for those, so it is worth
// having a test that proves the sweep honours entries it did not create.
func enqueueObjectKeys(t *testing.T, pool *pgxpool.Pool, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, err := pool.Exec(context.Background(),
			"INSERT INTO object_collection_queue (object_key) VALUES ($1) ON CONFLICT DO NOTHING",
			key); err != nil {
			t.Fatal(err)
		}
	}
}

func queuedObjectKeys(t *testing.T, pool *pgxpool.Pool, key string) int {
	t.Helper()
	return countRow(t, pool,
		"SELECT count(*) FROM object_collection_queue WHERE object_key = $1", key)
}

// 04 丙-73: package objects are content-addressed and shared with every fork, so
// no delete path may take them at the moment it takes rows -- whether an object
// may go is only knowable once the rows are gone. This sweep is what answers the
// question afterwards, and all but one of the assertions below are about what it
// must NOT touch.
//
// The asymmetry is the point. Leaving an object behind costs storage and can be
// found again; taking one that a fork still reads destroys somebody else's bytes
// with nothing left to restore them from, and that person never asked for
// anything. So the three keys here are one that may go and two that may not, and
// the two are on the worklist for the same reason a real one would be: a key
// gets enqueued when *a* referencing row disappears, never when the last one
// does, because nothing knows which was the last.
func TestOrphanObjectCollectionTakesOnlyWhatNothingReferences(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	a := newAPI(t, pool)

	alice := a.login(t, "alice-collect")
	bob := a.login(t, "bob-collect")
	makeCatalog(t, pool, alice.workspaceID)

	// shared: forked through the real endpoint, so the two version rows point at
	// one object because that is what a fork does (WS-001), not because the test
	// arranged it.
	shared := seedSkill(t, pool, alice.workspaceID, "collect-shared")
	sharedVer := seedVersion(t, pool, alice.workspaceID, shared, "collect-shared-hash")
	if status, _ := postJSON(t, bob, "/skills/"+shared+"/fork", "{}"); status != http.StatusCreated {
		t.Fatalf("bob fork of alice's catalog skill: got %d", status)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE package_object_key = $1",
		sharedVer.PackageObjectKey); c != 2 {
		t.Fatalf("fork left %d version rows on the shared object, want 2; the rest of this test asserts nothing", c)
	}

	// orphan: deleted through the endpoint and taken by the real grace purge, so
	// the state the sweep reads is the state a purge actually leaves behind.
	orphan := seedSkill(t, pool, alice.workspaceID, "collect-orphan")
	orphanVer := seedVersion(t, pool, alice.workspaceID, orphan, "collect-orphan-hash")

	// returned: enqueued while nothing referenced it, then re-imported. Identical
	// bytes get the same content-addressed key, so the object is alive again and
	// the worklist entry is stale -- which is why the decision is taken at sweep
	// time and not at enqueue time.
	returned := seedSkill(t, pool, alice.workspaceID, "collect-returned")
	returnedKey := "packages/collect-returned-hash.zip"

	enqueueObjectKeys(t, pool, sharedVer.PackageObjectKey, returnedKey)

	if status, _ := deleteJSON(t, alice, "/skills/"+orphan); status != http.StatusOK {
		t.Fatalf("DELETE /skills/%s: got %d", orphan, status)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE skills SET deleted_at = now() - interval '40 days' WHERE id = $1",
		mustUUID(t, orphan)); err != nil {
		t.Fatal(err)
	}
	if _, err := (&registry.Service{Pool: pool}).PurgeDeletedSkills(ctx, 30*24*time.Hour, 100); err != nil {
		t.Fatal(err)
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE package_object_key = $1",
		orphanVer.PackageObjectKey); c != 0 {
		t.Fatalf("the grace purge left the orphan's version rows; nothing below is testing collection")
	}
	// The producer, asserted on its own. Nobody enqueued this key: the purge did,
	// in the same statement that deleted the rows holding it. Without this the
	// sweep is a consumer of a queue that fills itself by magic -- and the queue
	// staying empty forever is exactly the failure 丙-73 opened for, since an
	// object nobody remembered is an object nobody can ever collect.
	if queuedObjectKeys(t, pool, orphanVer.PackageObjectKey) != 1 {
		t.Fatalf("the grace purge deleted the last rows holding %q without remembering the key; "+
			"those bytes are paid for and now unreachable", orphanVer.PackageObjectKey)
	}
	seedVersion(t, pool, alice.workspaceID, returned, "collect-returned-hash")

	store := &recordingStore{}
	c, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, 100)
	if err != nil {
		t.Fatal(err)
	}

	// The one thing it is allowed to do.
	if !slices.Contains(store.removed, orphanVer.PackageObjectKey) {
		t.Errorf("the purged skill's object was not collected; its bytes are paid for and unreachable forever")
	}
	if queuedObjectKeys(t, pool, orphanVer.PackageObjectKey) != 0 {
		t.Error("a collected object kept its worklist entry; the next pass will try to remove it again forever")
	}

	// The two it is not, one per reason.
	for _, spared := range []struct{ key, why string }{
		{sharedVer.PackageObjectKey, "a fork's version still references it"},
		{returnedKey, "a new version brought the same content-addressed key back"},
	} {
		if slices.Contains(store.removed, spared.key) {
			t.Errorf("the sweep removed a package object although %s", spared.why)
		}
		if queuedObjectKeys(t, pool, spared.key) != 0 {
			t.Errorf("the entry stayed on the worklist although %s; a queue that never shrinks "+
				"is indistinguishable from a sweep that has stopped", spared.why)
		}
	}
	if c := countRow(t, pool, "SELECT count(*) FROM skill_versions WHERE package_object_key = $1",
		sharedVer.PackageObjectKey); c != 2 {
		t.Error("the shared object's version rows changed; this sweep must not touch skill_versions at all")
	}
	if c.Collected < 1 || c.Dropped < 2 {
		t.Errorf("collected=%d dropped=%d, want at least 1 and 2: the two counts are how an operator "+
			"tells 'the keys came back' from 'the store will not take the delete'", c.Collected, c.Dropped)
	}
	if c.Depth != 0 {
		t.Errorf("queue depth %d after a pass that reached every entry", c.Depth)
	}

	// Iron rule 9: the second pass finds nothing to do and takes nothing else.
	// The sweep runs on a cron and a crash between the object and the queue row
	// is the ordinary case, so "safe to repeat" is the normal path, not an edge.
	before := len(store.removed)
	again, err := (&registry.Service{Pool: pool}).CollectOrphanObjects(ctx, store, 100)
	if err != nil {
		t.Fatal(err)
	}
	if again.Collected != 0 || len(store.removed) != before {
		t.Errorf("the second pass removed %d more objects; it is taking things the first one spared",
			len(store.removed)-before)
	}
}
