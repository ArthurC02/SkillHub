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
	"time"

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

// The same iron rule 9 gap on the cleanup side. TestCleanupOutcomeIsAudited above
// asserts that the row and the event are both there on the success path, which
// stays true whether the audit write rides the transaction or the pool — so it
// could not fail when the handle was swapped, and neither could anything else in
// the suite.
//
// recordCleanup writes its audit row last, so the failure has to be the commit
// itself; failOutboxCommitFor (run_integration_test.go) is that failure.
func TestACleanupThatFailsAfterItsAuditWriteLeavesNoAuditRow(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	_, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-cleanup-atomicity")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	finished := f.start(t)
	if err := svc.Drive(ctx, ws, mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	// Installed after the run is driven, so only the cleanup's own transaction is
	// the one that cannot commit.
	runID := mustUUID(t, finished.RunID)
	failOutboxCommitFor(t, pool, runID)

	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err == nil {
		t.Fatal("cleanup reported success although its transaction could not commit")
	}
	if n := countRow(t, pool, cleanupAuditSQL, runID); n != 0 {
		t.Errorf("%d run.cleanup audit rows outlived the transaction that wrote them", n)
	}
	if got := runCleanupStatus(t, pool, finished.RunID); got == string(gen.RunCleanupStatusCleaned) {
		t.Error("cleanup_status says cleaned although the transaction that wrote it rolled back")
	}
}

// A run that cannot be cleaned is put back on the supervisor's worklist every 30
// seconds (supervisor.go, ListRunsNeedingCleanup keeps everything that is not
// `cleaned`) and each cleanup job has five River attempts on top. A row per pass
// is thousands of audit_events a day for one stuck run — in the table PDM-006 §6
// keeps for 400 days, and for exactly the reason
// TestSourceAvailabilityIsAuditedOnlyWhenItChanges gives above: they say nothing
// new. The two moments that matter are the edges.
func TestCleanupOutcomeIsAuditedOnlyWhenItChanges(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, svc := haltHarness(t, a, pool)
	f := newFixture(t, a, pool, "alice-cleanup-repeat-audit")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)

	finished := f.start(t)
	if err := svc.Drive(ctx, ws, mustUUID(t, finished.RunID)); err != nil {
		t.Fatalf("driving the run: %v", err)
	}
	runID := mustUUID(t, finished.RunID)

	// The provider will not let go of the sandbox, so every pass reaches the same
	// outcome. The first one is news; the rest are the same sentence again.
	fake.DestroyStatus = http.StatusInternalServerError
	for pass := 1; pass <= 3; pass++ {
		if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err == nil {
			t.Fatalf("pass %d: a refused teardown reported success", pass)
		}
	}
	if n := countRow(t, pool, cleanupAuditSQL, runID); n != 1 {
		t.Errorf("run.cleanup audit rows = %d after three identical failures, want 1", n)
	}
	if n := countRow(t, pool, cleanupFailedOutboxSQL, runID); n != 1 {
		t.Errorf("run.cleanup_failed outbox events = %d after three identical failures, want 1", n)
	}
	// The column still has to be current even on a pass that records nothing, or
	// the run would sit in `cleaning_up` forever.
	if got := runCleanupStatus(t, pool, finished.RunID); got != string(gen.RunCleanupStatusFailed) {
		t.Errorf("cleanup_status = %q after the repeats, want failed", got)
	}

	// The teardown finally works. That IS new, so it is recorded — the edge this
	// whole rule exists to keep.
	fake.DestroyStatus = 0
	if err := svc.Cleanup(ctx, mustRun(t, pool, f.workspaceID, finished.RunID)); err != nil {
		t.Fatalf("the recovered teardown: %v", err)
	}
	if n := countRow(t, pool, cleanupAuditSQL, runID); n != 2 {
		t.Errorf("run.cleanup audit rows = %d after the teardown succeeded, want 2", n)
	}
}

const cleanupFailedOutboxSQL = `SELECT count(*) FROM outbox_events
	WHERE event_type = 'run.cleanup_failed' AND aggregate_id = $1`

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
	// The body is stable and its hash is what the row records, so the content leg
	// added in 0041 finds nothing and this test keeps measuring exactly one thing:
	// the availability edges. TestSourceContentChangeIsAuditedOnceAndOnlyOnAChange
	// below is the other leg.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !reachable {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(unchangedUpstreamBody))
	}))
	t.Cleanup(upstream.Close)
	host := mustHost(t, upstream.URL)

	var sourceID pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO skill_sources (workspace_id, source_type, source_url, content_hash, fetched_at)
		VALUES ($1, 'git', $2, $3, now()) RETURNING id`,
		mustUUID(t, c.workspaceID), upstream.URL, unchangedUpstreamHash).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{
		Allowed: map[string]bool{host: true}, AllowInsecure: true,
	}}
	sweep := func(t *testing.T, wantUnavailable int) {
		t.Helper()
		// A bound well above one row: other tests in this package import packages of
		// their own, and this sweep is deliberately not filtered to one workspace.
		if _, unavailable, _, err := svc.CheckSources(context.Background(), 200); err != nil {
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

// The bytes a stable upstream serves in the availability test above, and their
// sha256 — the value a row records at import time (prepare() hashes the raw
// archive). Kept as constants so the two tests cannot drift into disagreeing
// about what "unchanged" means.
const unchangedUpstreamBody = "the bytes this workspace imported\n"

const unchangedUpstreamHash = "adb6702943faee64f69f8375dabd058539d636b8aa4bef3e1674cc20d86ea966"

// 02:SEC-007 第 2 條, 02:CONTENT-009 and INGEST-010 all delegate detection to one
// sentence: 「以重抓並與保存的內容雜湊比對進行」. Until 2026-08-26 the sweep sent one
// HEAD, so an upstream that was rewritten — or relicensed — answered 200 and was
// recorded as healthy. That is the case this test is about, and the reason it
// cannot be folded into the availability test above: the URL never stops
// resolving.
func TestSourceContentChangeIsAuditedOnceAndOnlyOnAChange(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-content")

	body := unchangedUpstreamBody
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// HEAD stays 200 throughout: the whole point is that availability never
		// changes, so anything this test detects had to come from the bytes.
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	host := mustHost(t, upstream.URL)

	var sourceID pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO skill_sources (workspace_id, source_type, source_url, content_hash, fetched_at)
		VALUES ($1, 'git', $2, $3, now()) RETURNING id`,
		mustUUID(t, c.workspaceID), upstream.URL, unchangedUpstreamHash).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{
		Allowed: map[string]bool{host: true}, AllowInsecure: true,
	}}
	sweep := func(t *testing.T) {
		t.Helper()
		if _, _, _, err := svc.CheckSources(context.Background(), 200); err != nil {
			t.Fatal(err)
		}
	}
	events := func(t *testing.T) int {
		t.Helper()
		return countRow(t, pool,
			"SELECT count(*) FROM audit_events WHERE action = 'import_source.changed' AND resource_id = $1",
			sourceID)
	}
	changedAt := func(t *testing.T) *time.Time {
		t.Helper()
		var at *time.Time
		if err := pool.QueryRow(context.Background(),
			"SELECT content_changed_at FROM skill_sources WHERE id = $1", sourceID).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}

	// Two sweeps against bytes that match: nothing to say.
	sweep(t)
	sweep(t)
	if n := events(t); n != 0 {
		t.Fatalf("%d change events for an upstream that served the same bytes", n)
	}
	if at := changedAt(t); at != nil {
		t.Fatalf("content_changed_at = %v for an unchanged source", *at)
	}

	// It is rewritten. The URL still answers, so a HEAD-only sweep would see
	// nothing at all here — this assertion is the whole point of the change.
	body = "somebody rewrote this upstream, and relicensed it while they were there\n"
	sweep(t)
	if n := events(t); n != 1 {
		t.Fatalf("import_source.changed events = %d after a rewrite, want 1; "+
			"the URL still resolves, so availability probing cannot see this", n)
	}
	first := changedAt(t)
	if first == nil {
		t.Fatal("content_changed_at is still null after a detected rewrite")
	}

	// It stays rewritten, and it is rewritten AGAIN. Neither is news: the edge has
	// been taken, and content_hash is the hash of an immutable snapshot that does
	// not become current again (iron rule 4). A row per sweep would bury the one
	// that matters, which is the same argument unavailable_since already settled.
	sweep(t)
	body = "and again, differently\n"
	sweep(t)
	if n := events(t); n != 1 {
		t.Errorf("import_source.changed events = %d after three more sweeps, want the original 1", n)
	}
	if at := changedAt(t); at == nil || !at.Equal(*first) {
		t.Errorf("content_changed_at moved from %v to %v; it records when it FIRST stopped matching", first, at)
	}

	// The event is workspace-scoped and actor-less, like its availability
	// siblings: the sweep found this, nobody asked for it.
	var ws, actor string
	if err := pool.QueryRow(context.Background(), `
		SELECT workspace_id::text, coalesce(actor_user_id::text, '') FROM audit_events
		WHERE action = 'import_source.changed' AND resource_id = $1`, sourceID).Scan(&ws, &actor); err != nil {
		t.Fatal(err)
	}
	if ws != c.workspaceID {
		t.Errorf("event workspace = %s, want %s", ws, c.workspaceID)
	}
	if actor != "" {
		t.Errorf("event names actor %s; the source sweep is platform-initiated", actor)
	}
}

// A fetch that fails is not a finding. "We could not look" entering the record as
// "it changed" would be permanent — the mark is written once and never cleared —
// and it is the failure mode this whole leg is most likely to hit, because the
// re-fetch is a full download on a schedule.
func TestASourceThatCannotBeRefetchedIsNotRecordedAsChanged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-refetch")

	// HEAD succeeds, GET does not: exactly the shape a rate limit or a
	// partial outage produces, and the one that must stay silent.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(upstream.Close)
	host := mustHost(t, upstream.URL)

	var sourceID pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO skill_sources (workspace_id, source_type, source_url, content_hash, fetched_at)
		VALUES ($1, 'git', $2, $3, now()) RETURNING id`,
		mustUUID(t, c.workspaceID), upstream.URL, unchangedUpstreamHash).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}

	svc := &ingest.Service{Pool: pool, Fetcher: &ingest.URLFetcher{
		Allowed: map[string]bool{host: true}, AllowInsecure: true,
	}}
	if _, _, _, err := svc.CheckSources(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	if n := countRow(t, pool,
		"SELECT count(*) FROM audit_events WHERE action = 'import_source.changed' AND resource_id = $1",
		sourceID); n != 0 {
		t.Errorf("%d change events for a source that could not be downloaded at all", n)
	}
	var at *time.Time
	if err := pool.QueryRow(context.Background(),
		"SELECT content_changed_at FROM skill_sources WHERE id = $1", sourceID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if at != nil {
		t.Errorf("content_changed_at = %v after a failed re-fetch; this mark is never cleared, "+
			"so writing it on a failure is permanent", *at)
	}
}
