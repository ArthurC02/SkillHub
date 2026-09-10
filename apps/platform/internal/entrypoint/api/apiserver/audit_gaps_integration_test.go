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

	var versionID, actor string
	var resourceType string
	if err := pool.QueryRow(context.Background(), `
		SELECT resource_id::text, actor_user_id::text, resource_type FROM audit_events
		WHERE action = 'run.refused' AND workspace_id = $1
		ORDER BY id DESC LIMIT 1`, mustUUID(t, f.workspaceID)).
		Scan(&versionID, &actor, &resourceType); err != nil {
		t.Fatal(err)
	}

	if versionID != f.versionID {
		t.Errorf("refusal points at %s, want the refused version %s", versionID, f.versionID)
	}
	if actor != f.userID {
		t.Errorf("refusal actor = %s, want the caller %s", actor, f.userID)
	}
	if resourceType != "skill_version" {
		t.Errorf("resource_type = %q, want skill_version", resourceType)
	}

	if n := countRow(t, pool, `
		SELECT count(*) FROM audit_events
		WHERE action = 'run.refused' AND metadata::text ILIKE '%in progress%'`); n != 0 {
		t.Error("the refusal's user-facing message was stored in the audit metadata")
	}

	if n := countRow(t, pool, "SELECT count(*) FROM runs WHERE workspace_id = $1",
		mustUUID(t, f.workspaceID)); n != run.MaxConcurrentRunsPerWorkspace {
		t.Errorf("runs = %d, want %d: the refused request left a row behind",
			n, run.MaxConcurrentRunsPerWorkspace)
	}
}

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

	if actor != "" {
		t.Errorf("cleanup audit names actor %s; teardown is platform-initiated", actor)
	}

	if _, view := f.getRun(t, finished.RunID); view.CleanupStatus.Value != string(gen.RunCleanupStatusCleaned) {
		t.Errorf("cleanup_status = %+v, want cleaned", view.CleanupStatus)
	}
}

const cleanupAuditSQL = `SELECT count(*) FROM audit_events WHERE action = 'run.cleanup' AND resource_id = $1`

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

	if got := runCleanupStatus(t, pool, finished.RunID); got != string(gen.RunCleanupStatusFailed) {
		t.Errorf("cleanup_status = %q after the repeats, want failed", got)
	}

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

func TestSourceAvailabilityIsAuditedOnlyWhenItChanges(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-audit")

	reachable := true

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

	sweep(t, 0)
	sweep(t, 0)
	if n := edges(t, "import_source.unavailable") + edges(t, "import_source.restored"); n != 0 {
		t.Fatalf("%d events for a source that never changed state", n)
	}

	reachable = false
	sweep(t, 1)
	sweep(t, 1)
	if n := edges(t, "import_source.unavailable"); n != 1 {
		t.Errorf("import_source.unavailable events = %d, want exactly 1 for two failing probes", n)
	}

	reachable = true
	sweep(t, 0)
	sweep(t, 0)
	if n := edges(t, "import_source.restored"); n != 1 {
		t.Errorf("import_source.restored events = %d, want exactly 1 for two succeeding probes", n)
	}
	if n := edges(t, "import_source.unavailable"); n != 1 {
		t.Errorf("import_source.unavailable events = %d after the recovery, want the original 1", n)
	}

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

const unchangedUpstreamBody = "the bytes this workspace imported\n"

const unchangedUpstreamHash = "adb6702943faee64f69f8375dabd058539d636b8aa4bef3e1674cc20d86ea966"

func TestSourceContentChangeIsAuditedOnceAndOnlyOnAChange(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-content")

	body := unchangedUpstreamBody
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

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

	sweep(t)
	sweep(t)
	if n := events(t); n != 0 {
		t.Fatalf("%d change events for an upstream that served the same bytes", n)
	}
	if at := changedAt(t); at != nil {
		t.Fatalf("content_changed_at = %v for an unchanged source", *at)
	}

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

	sweep(t)
	body = "and again, differently\n"
	sweep(t)
	if n := events(t); n != 1 {
		t.Errorf("import_source.changed events = %d after three more sweeps, want the original 1", n)
	}
	if at := changedAt(t); at == nil || !at.Equal(*first) {
		t.Errorf("content_changed_at moved from %v to %v; it records when it FIRST stopped matching", first, at)
	}

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

func TestASourceThatCannotBeRefetchedIsNotRecordedAsChanged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "alice-source-refetch")

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
