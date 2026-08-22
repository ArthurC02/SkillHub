// WS-004 and CORE-007's read and delete surfaces through the real route table:
// the Run history, one run's outputs, deleting one of them, the per-download
// records, and whether a pending account deletion can be asked about after the
// request was made.
//
// All five were the same gap in different places (04 丙-22, 丙-24): the backend
// could do the thing and there was no way to ask it. 02:WS-002 第 1 條 and
// 02:SEC-006 第 1 條 both have a user as their subject, and a capability with no
// route is not something a user can reach at any layer.
package apiserver_test

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

"github.com/ArthurC02/skillhub/apps/platform/internal/product/learning"
"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

// O11Y-004's second half is the word "查詢", and the funnel query is a psql
// script rather than an endpoint (ADR-029 決策 6). A script nothing runs rots
// against the schema silently, so this executes the real file against the real
// tables — the same reason the packaging tests read contracts/packaging/profiles
// instead of a fixture copy.
//
// psql meta-commands and its variables are stripped here; what is under test is
// the query body, which is what a column rename breaks.
func TestTheFunnelQueryStillRunsAgainstTheSchema(t *testing.T) {
	pool := requireDB(t)
	sql := funnelQuery(t, "'-infinity'", "'infinity'")

	rows, err := pool.Query(context.Background(), sql)
	if err != nil {
		t.Fatalf("the funnel query no longer runs against this schema: %v", err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var segment int
		var description, note string
		var numerator, denominator int64
		if err := rows.Scan(&segment, &description, &numerator, &denominator, &note); err != nil {
			t.Fatal(err)
		}
		seen++
		if numerator > denominator {
			t.Errorf("segment %d reports %d of %d", segment, numerator, denominator)
		}
		// 02:O11Y-004's last clause: the precision limit travels with the number.
		if note == "" {
			t.Errorf("segment %d reports a percentage with no precision limit", segment)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 7 {
		t.Errorf("the funnel reports %d segments, and 01 §11.2 has 7", seen)
	}
}

func funnelQuery(t *testing.T, from, to string) string {
	t.Helper()
	raw, err := os.ReadFile("../../../../../../tools/analytics/funnel.sql")
	if err != nil {
		t.Fatal(err)
	}
	var body []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), `\`) {
			continue
		}
		body = append(body, line)
	}
	return strings.NewReplacer(":from", from, ":to", to).Replace(strings.Join(body, "\n"))
}

func TestReturningWorkspaceUsesSameUTCVisitDay(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	workspaceA := a.login(t, "funnel-workspace-a").workspaceID
	workspaceB := a.login(t, "funnel-workspace-b").workspaceID
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL TIME ZONE 'America/Los_Angeles'"); err != nil {
		t.Fatal(err)
	}
	insert := func(name, at string, workspace any) {
		t.Helper()
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_events
            (event_name, session_id, workspace_id, occurred_at) VALUES ($1, 'shared-browser', $2, $3)`,
			name, workspace, at); err != nil {
			t.Fatal(err)
		}
	}
	// The anonymous day must not be retroactively assigned. A has two visits on
	// opposite sides of UTC midnight (but the same Los Angeles date); B has one.
	insert("session_started", "2040-08-18T12:00:00Z", nil)
	for _, visit := range []struct{ at, workspace string }{
		{"2040-08-19T23:30:00Z", workspaceA},
		{"2040-08-20T00:30:00Z", workspaceA},
		{"2040-08-21T12:00:00Z", workspaceB},
	} {
		insert("session_started", visit.at, nil)
		insert("skill_detail_viewed", visit.at, mustUUID(t, visit.workspace))
	}
	rows, err := tx.Query(ctx, funnelQuery(t, "'2040-08-18T00:00:00Z'", "'2040-08-22T00:00:00Z'"))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var segment int
		var description, note string
		var numerator, denominator int64
		if err := rows.Scan(&segment, &description, &numerator, &denominator, &note); err != nil {
			t.Fatal(err)
		}
		if segment == 7 {
			found = true
			if numerator != 1 || denominator != 2 {
				t.Errorf("returning workspaces = %d of %d, want A only: 1 of 2", numerator, denominator)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("funnel returned no segment 7")
	}
}

func TestAnalyticsPurgeHonorsTheExactCutoff(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	sessionID := "purge-cutoff-fixture"
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM analytics_events WHERE session_id = $1", sessionID)
	})
	// Keep the cutoff far behind every normal test row. Purge is intentionally a
	// deployment-wide operation, so a future cutoff would erase sibling fixtures.
	now := time.Date(2000, 1, 11, 0, 0, 0, 0, time.UTC)
	cutoff := now.Add(-10 * 24 * time.Hour)
	for _, at := range []time.Time{cutoff.Add(-time.Second), cutoff, cutoff.Add(time.Second)} {
		if _, err := pool.Exec(ctx, `INSERT INTO analytics_events
            (event_name, session_id, occurred_at) VALUES ('session_started', $1, $2)`, sessionID, at); err != nil {
			t.Fatal(err)
		}
	}
	svc := &analytics.Service{Pool: pool, Retention: 10 * 24 * time.Hour, Now: func() time.Time { return now }}
	removed, err := svc.PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("purge removed %d rows, want only the row before the cutoff", removed)
	}
	if n := countRows(t, pool, "SELECT count(*) FROM analytics_events WHERE session_id = $1", sessionID); n != 2 {
		t.Errorf("%d cutoff fixture rows remain, want the cutoff and newer rows", n)
	}
}

// seedRunArtifact records one output against a run, the way the settle path does
// when a provider reports its manifest, and puts bytes behind it so a delete has
// something to remove.
func seedRunArtifact(
	t *testing.T, a *api, pool *pgxpool.Pool, workspaceID, runID, name string,
) string {
	t.Helper()
	ctx := context.Background()
	key := "run-artifacts/" + runID + "/" + name
	a.packages[key] = []byte("artifact bytes")
	n, err := gen.New(pool).InsertRunArtifact(ctx, gen.InsertRunArtifactParams{
		WorkspaceID: mustUUID(t, workspaceID), RunID: mustUUID(t, runID),
		FileName: name, ContentType: "application/octet-stream",
		SizeBytes: 14, ContentHash: "deadbeef", ObjectKey: key,
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("seeding %s inserted %d rows", name, n)
	}
	var id string
	if err := pool.QueryRow(ctx,
		`SELECT id::text FROM artifacts WHERE run_id = $1 AND file_name = $2`,
		mustUUID(t, runID), name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

type runListView struct {
	RunID          string `json:"run_id"`
	Status         string `json:"status"`
	SkillID        string `json:"skill_id"`
	SkillName      string `json:"skill_name"`
	SkillVersionID string `json:"skill_version_id"`
	TestCaseID     string `json:"test_case_id"`
	CleanupStatus  string `json:"cleanup_status"`
	CreatedAt      string `json:"created_at"`
}

func (c *client) listRuns(t *testing.T) []runListView {
	t.Helper()
	var out struct {
		Runs []runListView `json:"runs"`
	}
	if code := getJSON(t, c.Client, c.base+"/runs", &out); code != http.StatusOK {
		t.Fatalf("GET /runs: got %d", code)
	}
	return out.Runs
}

// 02:WS-002 第 1 條's "Run 歷史". There was no endpoint at all before this, so
// the clause had no surface at any layer — not a missing screen, a missing route.
func TestTheRunHistoryListsTheWorkspacesOwnRunsAndNobodyElses(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	mine := newFixture(t, a, pool, "history-owner")
	created := mine.start(t)

	other := newFixture(t, a, pool, "history-stranger")
	theirs := other.start(t)

	rows := mine.listRuns(t)
	var found *runListView
	for i, r := range rows {
		if r.RunID == created.RunID {
			found = &rows[i]
		}
		if r.RunID == theirs.RunID {
			t.Errorf("another workspace's run %s is in this history", theirs.RunID)
		}
	}
	if found == nil {
		t.Fatalf("the run just created is not in the history: %+v", rows)
	}
	// A history row has to be readable without opening the run: which skill, and
	// enough to start the same test again. Resolving the name client-side would be
	// one lookup per row.
	if found.SkillID != mine.skillID || found.SkillName == "" {
		t.Errorf("history row names no skill: %+v", found)
	}
	if found.TestCaseID != mine.testCaseID {
		t.Errorf("test_case_id = %q, want the editable case %q", found.TestCaseID, mine.testCaseID)
	}
	if found.CreatedAt == "" || found.CleanupStatus == "" {
		t.Errorf("history row is missing when/what state: %+v", found)
	}
}

// 02:WS-002 第 3 條 and 02:SEC-006 第 1 條 both list Artifact among the things a
// user may delete. Until this route existed the only way to remove a run's output
// was to delete the entire account, which is not the same offer.
func TestARunArtifactCanBeListedAndDeletedOnItsOwn(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "artifact-owner")
	created := f.start(t)
	artifactID := seedRunArtifact(t, a, pool, f.workspaceID, created.RunID, "report.csv")
	objectKey := "run-artifacts/" + created.RunID + "/report.csv"

	list := func() []map[string]any {
		t.Helper()
		var out struct {
			Artifacts []map[string]any `json:"artifacts"`
		}
		if code := getJSON(t, f.Client,
			f.base+"/runs/"+created.RunID+"/artifacts", &out); code != http.StatusOK {
			t.Fatalf("GET artifacts: got %d", code)
		}
		return out.Artifacts
	}
	before := list()
	if len(before) != 1 || before[0]["file_name"] != "report.csv" {
		t.Fatalf("the run's output is not listed: %v", before)
	}
	// The manifest row and nothing more: the archive is a sandbox's output and the
	// control plane never opens it (iron rule 1).
	if _, leaked := before[0]["object_key"]; leaked {
		t.Error("the artifact list hands out the storage key")
	}

	path := "/runs/" + created.RunID + "/artifacts/" + artifactID
	if code := f.status(t, http.MethodDelete, path); code != http.StatusNoContent {
		t.Fatalf("DELETE artifact: got %d", code)
	}
	if after := list(); len(after) != 0 {
		t.Errorf("the deleted artifact is still listed: %v", after)
	}
	// 02:SEC-006's "完成後不再出現在一般存取介面" is the row; NFR-002's promise that
	// deletion deletes is the bytes.
	if _, still := a.packages[objectKey]; still {
		t.Error("the artifact row is gone and its bytes are not")
	}
	// CORE-008: a delete is one of NFR-001 第 4 條's five audited actions.
	if n := countRows(t, pool,
		`SELECT count(*) FROM audit_events WHERE action = 'artifact.delete' AND resource_id = $1`,
		mustUUID(t, artifactID)); n != 1 {
		t.Errorf("%d audit events for one artifact delete, want 1", n)
	}

	// Idempotent, like the download package's delete: the caller asked for the file
	// not to exist, and that is true on the second call too.
	if code := f.status(t, http.MethodDelete, path); code != http.StatusNoContent {
		t.Errorf("repeating the delete: got %d, want 204", code)
	}
	// And another workspace cannot delete it — nor learn that it was ever there.
	stranger := a.login(t, "artifact-stranger")
	if code := stranger.status(t, http.MethodDelete, path); code != http.StatusNoContent {
		t.Errorf("a stranger's delete: got %d; the answer must not distinguish", code)
	}
	if code := stranger.status(t, http.MethodGet,
		"/runs/"+created.RunID+"/artifacts"); code != http.StatusNotFound {
		t.Errorf("a stranger listing another workspace's run artifacts: got %d, want 404", code)
	}
}

// WS-004 asks for "誰、何時" per download, which an aggregate count cannot answer.
// The rows existed in download_records from 0027 and nothing served them.
func TestTheDownloadRecordsAreListedOneRowPerDownload(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "record-reader")
	artifact := buildDownload(t, a, pool, c, "recorded-skill")

	records := func(cl *client) (int, []map[string]any) {
		t.Helper()
		var out struct {
			Records []map[string]any `json:"records"`
		}
		code := getJSON(t, cl.Client, cl.base+"/downloads/"+artifact.ArtifactID+"/records", &out)
		return code, out.Records
	}
	code, empty := records(c)
	if code != http.StatusOK {
		t.Fatalf("GET records: got %d", code)
	}
	if len(empty) != 0 {
		t.Errorf("a package nobody downloaded has %d records", len(empty))
	}

	for i := 0; i < 2; i++ {
		if resp, _ := c.fetchContent(t, artifact.ArtifactID); resp.StatusCode != http.StatusOK {
			t.Fatalf("download %d: got %d", i, resp.StatusCode)
		}
	}
	code, got := records(c)
	if code != http.StatusOK {
		t.Fatalf("GET records: got %d", code)
	}
	if len(got) != 2 {
		t.Fatalf("%d records for two downloads: %v", len(got), got)
	}
	for _, r := range got {
		if r["downloaded_at"] == "" || r["actor"] == "" {
			t.Errorf("a record answers neither who nor when: %v", r)
		}
	}
	// Existence is private (WS-006): a stranger gets the same answer an unknown id
	// gets, not an empty list that would confirm the package exists.
	stranger := a.login(t, "record-stranger")
	if code, _ := records(stranger); code != http.StatusNotFound {
		t.Errorf("a stranger reading another workspace's download records: got %d, want 404", code)
	}
}

// 02:SEC-006 asks the deletion job to have a state the user can follow. Until now
// it appeared once, in the response to DELETE /me, so a user who closed the tab
// had no way to ask whether the request had been recorded or when it runs out.
func TestAPendingAccountDeletionCanBeAskedAboutAfterTheRequest(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "deleter")

	me := func() map[string]any {
		t.Helper()
		var out map[string]any
		if code := getJSON(t, c.Client, c.base+"/me", &out); code != http.StatusOK {
			t.Fatalf("GET /me: got %d", code)
		}
		return out
	}
	if v := me()["deletion_requested_at"]; v != nil {
		t.Errorf("a fresh account reports a pending deletion: %v", v)
	}

	if code := c.status(t, http.MethodDelete, "/me"); code != http.StatusOK {
		t.Fatalf("DELETE /me: got %d", code)
	}
	after := me()
	if after["deletion_requested_at"] == nil {
		t.Fatal("the deletion request is not visible on /me, so nothing can report its state")
	}
	// The date the grace period runs out is the one thing a user needs from this
	// screen, and deriving it client-side would put the retention constant in two
	// places.
	if after["purge_after"] == nil {
		t.Error("no purge_after; the user cannot tell how long they have to change their mind")
	}

	if code, _ := postJSON(t, c, "/me/deletion/cancel", `{}`); code != http.StatusOK {
		t.Fatalf("cancel: got %d", code)
	}
	if v := me()["deletion_requested_at"]; v != nil {
		t.Errorf("a cancelled deletion still reports as pending: %v", v)
	}
}

// listRunsForTestCase is listRuns narrowed to one test case.
func (c *client) listRunsForTestCase(t *testing.T, testCaseID string) []runListView {
	t.Helper()
	var out struct {
		Runs []runListView `json:"runs"`
	}
	url := c.base + "/runs?test_case_id=" + testCaseID
	if code := getJSON(t, c.Client, url, &out); code != http.StatusOK {
		t.Fatalf("GET %s: got %d", url, code)
	}
	return out.Runs
}

// The return leg of the journey: 建立 → 試跑 → 回來看. Without this parameter a Test
// Case detail page has no way to ask "what happened when I ran this", which is
// also the support the M3 主路徑 (採納建議 → 新版本 → 同一個 Test Case 重跑) needs.
func TestTheRunHistoryCanBeNarrowedToOneTestCase(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "history-per-case")
	mine := f.start(t)

	// A second draft on the same skill, run once. It is the row the filter has to
	// leave out — a filter that returns everything passes a one-run test.
	other := f
	other.testCaseID = seedTestCase(t, pool, f.workspaceID, f.skillID)
	theirs := other.start(t)

	rows := f.listRunsForTestCase(t, f.testCaseID)
	if len(rows) != 1 {
		t.Fatalf("filtered history = %d runs, want 1: %+v", len(rows), rows)
	}
	if rows[0].RunID != mine.RunID {
		t.Errorf("filtered history returned run %s, want %s", rows[0].RunID, mine.RunID)
	}
	if rows[0].RunID == theirs.RunID {
		t.Errorf("the other test case's run %s is in this filtered history", theirs.RunID)
	}
	if rows[0].TestCaseID != f.testCaseID {
		t.Errorf("test_case_id = %q, want %q", rows[0].TestCaseID, f.testCaseID)
	}
	// Both runs are still in the unfiltered history, so the filter narrowed rather
	// than hid.
	if len(f.listRuns(t)) != 2 {
		t.Errorf("unfiltered history lost a run: %+v", f.listRuns(t))
	}

	// WS-006 / iron rule 3: another workspace's test case is not a way in, and
	// neither is a filter the server cannot parse.
	stranger := newFixture(t, a, pool, "history-per-case-stranger")
	if rows := stranger.listRunsForTestCase(t, f.testCaseID); len(rows) != 0 {
		t.Errorf("another workspace's runs leaked through test_case_id: %+v", rows)
	}
	if rows := f.listRunsForTestCase(t, "not-a-uuid"); len(rows) != 0 {
		t.Errorf("an unparseable test_case_id fell back to the whole history: %+v", rows)
	}
}
