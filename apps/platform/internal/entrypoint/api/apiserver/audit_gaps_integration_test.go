// The three operations NFR-001 covers that were reaching the audit trail through
// nothing at all (04 丙-23): a run the gate refused, a run's sandboxes being torn
// down, and an imported package's upstream source appearing or disappearing.
//
// Each was observable somewhere before this — a Prometheus counter, a slog line,
// a column that the next write overwrites — and none of those is the trail. The
// tests below therefore assert on audit_events rows rather than on the behaviour
// that produces them, which is already covered by preflight_integration_test.go,
// dispatch_halt_integration_test.go and the ingest package's own tests.
//
// Shared harness lives in authz_integration_test.go (TestMain, migrate, requireDB,
// newAPI, login), run_integration_test.go (fixture, newFixture, start),
// preflight_integration_test.go (confirmPermissions, startWithHash),
// governance_integration_test.go (countRow, mustUUID) and
// dispatch_halt_integration_test.go (haltHarness, mustRun).
package apiserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// refusalReasons is every reason recorded against one workspace, in order. The
// reason lives in metadata rather than in the action name on purpose (audit.go),
// so this is what a review of "why did this workspace keep being told no" reads.
func refusalReasons(t *testing.T, pool *pgxpool.Pool, workspaceID string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT metadata->>'reason' FROM audit_events
		WHERE action = 'run.refused' AND workspace_id = $1
		ORDER BY id`, mustUUID(t, workspaceID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var reasons []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			t.Fatal(err)
		}
		reasons = append(reasons, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return reasons
}

// NFR-001 on the side of gate B that was writing nothing down. A run that starts
// leaves a run row, a status history and a run.create event; a run that is stopped
// used to leave a counter with no workspace, no version and no time on it — and a
// security review is usually opened to ask about the second kind.
func TestARefusedRunIsAudited(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-refusal-audit")

	hash := f.confirmPermissions(t)
	for i := 1; i <= run.MaxConcurrentRunsPerWorkspace; i++ {
		if code, view := f.startWithHash(t, hash); code != http.StatusCreated {
			t.Fatalf("filling slot %d: got %d (%s)", i, code, view.Error)
		}
	}
	// Every run that was allowed through is on the trail already. Nothing may be
	// recorded as refused while the gate was still saying yes.
	if got := refusalReasons(t, pool, f.workspaceID); len(got) != 0 {
		t.Fatalf("runs that started were recorded as refusals: %v", got)
	}

	if code, refused := f.startWithHash(t, hash); code != http.StatusUnprocessableEntity {
		t.Fatalf("run past the concurrency limit: got %d (%s), want 422", code, refused.Error)
	}

	got := refusalReasons(t, pool, f.workspaceID)
	if len(got) != 1 || got[0] != "workspace_concurrency" {
		t.Fatalf("refusal reasons = %v, want exactly [workspace_concurrency]", got)
	}

	// The reason vocabulary is metrics.RunRefused's labels, not a second one
	// invented for the trail: a spike on the dashboard and the rows that explain
	// it have to be searchable with the same word.
	var versionID, actor string
	var resourceType string
	if err := pool.QueryRow(context.Background(), `
		SELECT resource_id::text, actor_user_id::text, resource_type FROM audit_events
		WHERE action = 'run.refused' AND workspace_id = $1
		ORDER BY id DESC LIMIT 1`, mustUUID(t, f.workspaceID)).
		Scan(&versionID, &actor, &resourceType); err != nil {
		t.Fatal(err)
	}
	// No run exists to point at, so the refused version is the resource, and the
	// user who asked is the actor — the two identifiers that make the row useful.
	if versionID != f.versionID {
		t.Errorf("refusal points at %s, want the refused version %s", versionID, f.versionID)
	}
	if actor != f.userID {
		t.Errorf("refusal actor = %s, want the caller %s", actor, f.userID)
	}
	if resourceType != "skill_version" {
		t.Errorf("resource_type = %q, want skill_version", resourceType)
	}

	// Iron rule 11: the refusal message can quote scan findings and package
	// content, so only the reason code is stored.
	if n := countRow(t, pool, `
		SELECT count(*) FROM audit_events
		WHERE action = 'run.refused' AND metadata::text ILIKE '%in progress%'`); n != 0 {
		t.Error("the refusal's user-facing message was stored in the audit metadata")
	}

	// A refusal is not a run. Nothing about it may reach the run tables.
	if n := countRow(t, pool, "SELECT count(*) FROM runs WHERE workspace_id = $1",
		mustUUID(t, f.workspaceID)); n != run.MaxConcurrentRunsPerWorkspace {
		t.Errorf("runs = %d, want %d: the refused request left a row behind",
			n, run.MaxConcurrentRunsPerWorkspace)
	}
}

// A refusal that never reaches gate B's own checks still lands on the trail. This
// is the path through the sentinel errors rather than through refused(), because
// the permission check lives with its feature (preflight.go) and not in gateb.go.
func TestAnUnconfirmedRunIsAuditedAsRefused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-unconfirmed-audit")

	code, view := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"0000000000000000000000000000000000000000000000000000000000000000"}`)
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("run on an unconfirmed summary: got %d (%s), want 422", code, view.Error)
	}
	got := refusalReasons(t, pool, f.workspaceID)
	if len(got) != 1 || got[0] != "permissions_unconfirmed" {
		t.Fatalf("refusal reasons = %v, want exactly [permissions_unconfirmed]", got)
	}
}

// A run that is simply not this caller's is NOT audited as a refusal. WS-006 makes
// "not yours" and "does not exist" the same answer on purpose, and a row per probe
// would turn the trail into an enumeration log of what exists.
func TestALookupMissIsNotAuditedAsARefusal(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-miss-audit")

	code, _ := f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"00000000-0000-0000-0000-0000000000ff","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"deadbeef"}`)
	if code == http.StatusCreated {
		t.Fatal("a run was created on a version that does not exist")
	}
	if got := refusalReasons(t, pool, f.workspaceID); len(got) != 0 {
		t.Errorf("a lookup miss was recorded as a gate refusal: %v", got)
	}
}

// RUN-007's teardown, on the trail rather than only in runs.cleanup_status — which
// the next attempt overwrites, so a run that took two passes to release would look
// like it succeeded first time.
func TestCleanupOutcomeIsAudited(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-cleanup-audit")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	finished := f.start(t)
	if err := svc.Drive(ctx, ws, mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	if fake.Live() != 1 {
		t.Fatalf("precondition: %d sandboxes held, want 1", fake.Live())
	}
	if n := countRow(t, pool, cleanupAuditSQL, mustUUID(t, finished.RunID)); n != 0 {
		t.Fatalf("a cleanup was audited before one happened (%d rows)", n)
	}

	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	var status, actor string
	if err := pool.QueryRow(ctx, `
		SELECT metadata->>'cleanup_status', coalesce(actor_user_id::text, '')
		FROM audit_events WHERE action = 'run.cleanup' AND resource_id = $1`,
		mustUUID(t, finished.RunID)).Scan(&status, &actor); err != nil {
		t.Fatal(err)
	}
	if status != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("audited cleanup_status = %q, want cleaned", status)
	}
	// Cleanup is enqueued by a terminal transition or found by the supervisor's
	// backlog scan; nobody asks for it, so there is no actor to name.
	if actor != "" {
		t.Errorf("cleanup audit names actor %s; teardown is platform-initiated", actor)
	}

	// Iron rule 9: the row and the status write are one transaction, so the trail
	// cannot claim a teardown that the run row does not agree happened.
	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus.Value != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("cleanup_status = %+v, want cleaned", view.CleanupStatus)
	}
}

const cleanupAuditSQL = `SELECT count(*) FROM audit_events WHERE action = 'run.cleanup' AND resource_id = $1`

// INGEST-010's two edges, and the repeats in between that must NOT be recorded.
//
// The sweep re-probes every source on a schedule, so a row per probe would bury
// the two moments that matter under thousands that say nothing new — and would
// make the 400 day retention of PDM-006 §6 expensive for no information.
func TestSourceAvailabilityIsAuditedOnlyWhenItChanges(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-audit")

	reachable := true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !reachable {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)
	host := mustHost(t, upstream.URL)

	var sourceID pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO skill_sources (workspace_id, source_type, source_url, content_hash, fetched_at)
		VALUES ($1, 'git', $2, 'hash-source-audit', now()) RETURNING id`,
		mustUUID(t, c.workspaceID), upstream.URL).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{
		Allowed: map[string]bool{host: true}, AllowInsecure: true,
	}}
	sweep := func(t *testing.T, wantUnavailable int) {
		t.Helper()
		// A bound well above one row: other tests in this package import packages of
		// their own, and this sweep is deliberately not filtered to one workspace.
		if _, unavailable, err := svc.CheckSources(context.Background(), 200); err != nil {
			t.Fatal(err)
		} else if unavailable < wantUnavailable {
			t.Fatalf("sweep reported %d unavailable sources, want at least %d", unavailable, wantUnavailable)
		}
	}
	edges := func(t *testing.T, action string) int {
		t.Helper()
		return countRow(t, pool,
			"SELECT count(*) FROM audit_events WHERE action = $1 AND resource_id = $2", action, sourceID)
	}

	// Two passes while the source answers: nothing changed, so nothing is recorded.
	sweep(t, 0)
	sweep(t, 0)
	if n := edges(t, "import_source.unavailable") + edges(t, "import_source.restored"); n != 0 {
		t.Fatalf("%d events for a source that never changed state", n)
	}

	// It goes away, and stays away. One event, on the edge.
	reachable = false
	sweep(t, 1)
	sweep(t, 1)
	if n := edges(t, "import_source.unavailable"); n != 1 {
		t.Errorf("import_source.unavailable events = %d, want exactly 1 for two failing probes", n)
	}

	// It comes back, and stays back. One event, again on the edge.
	reachable = true
	sweep(t, 0)
	sweep(t, 0)
	if n := edges(t, "import_source.restored"); n != 1 {
		t.Errorf("import_source.restored events = %d, want exactly 1 for two succeeding probes", n)
	}
	if n := edges(t, "import_source.unavailable"); n != 1 {
		t.Errorf("import_source.unavailable events = %d after the recovery, want the original 1", n)
	}

	// The event is scoped to the workspace whose content changed availability, and
	// carries no actor: the sweep found this, nobody asked for it.
	var ws, actor string
	if err := pool.QueryRow(context.Background(), `
		SELECT workspace_id::text, coalesce(actor_user_id::text, '') FROM audit_events
		WHERE action = 'import_source.restored' AND resource_id = $1`, sourceID).Scan(&ws, &actor); err != nil {
		t.Fatal(err)
	}
	if ws != c.workspaceID {
		t.Errorf("event workspace = %s, want %s", ws, c.workspaceID)
	}
	if actor != "" {
		t.Errorf("event names actor %s; the source probe is platform-initiated", actor)
	}

	// Iron rule 9: the mark and the event commit together, so the column cannot
	// disagree with the trail.
	if n := countRow(t, pool,
		"SELECT count(*) FROM skill_sources WHERE id = $1 AND unavailable_since IS NULL", sourceID); n != 1 {
		t.Error("the source answered again but unavailable_since was not cleared")
	}
}

func mustHost(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}
