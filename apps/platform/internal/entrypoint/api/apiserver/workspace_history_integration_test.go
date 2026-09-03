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
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/product/entitlements"
)

// O11Y-004's second half is the word "查詢", and the funnel query is a psql
// script rather than an endpoint (ADR-029 決策 6). A script nothing runs rots
// against the schema silently, so this executes the real file against the real
// tables — the same reason the packaging tests read contracts/packaging/profiles
// instead of a fixture copy.
//
// The psql meta-commands are interpreted here rather than stripped; psqlRender
// says why that distinction cost us a bug.
func TestTheFunnelQueryStillRunsAgainstTheSchema(t *testing.T) {
	pool := requireDB(t)
	// No -v at all: the all-time report, which is also the default branch of the
	// file's own defaulting.
	sql := psqlRender(t, readFunnel(t), nil)

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

// The range the operator passed has to survive the file. `\set` is assignment and
// not defaulting, so the two unconditional `\set` lines this file opened with
// until 2026-08-25 threw away every -v: BETA-002's "the two beta weeks" was
// really "everything this database has ever held", M1–M4 integration residue
// included, and no two reports were comparable because both were "so far".
func TestTheFunnelKeepsTheRangeTheOperatorPassed(t *testing.T) {
	raw := readFunnel(t)

	bounded := psqlRender(t, raw, map[string]string{
		"from": "'2026-09-01T00:00:00Z'", "to": "'2026-09-15T00:00:00Z'",
	})
	for _, want := range []string{"'2026-09-01T00:00:00Z'::timestamptz", "'2026-09-15T00:00:00Z'::timestamptz"} {
		if !strings.Contains(bounded, want) {
			t.Errorf("the funnel discarded the range it was given: %s is not in the query", want)
		}
	}
	if strings.Contains(bounded, "infinity'::timestamptz") {
		t.Error("the funnel overwrote the range it was given with an unbounded one")
	}

	// And with no -v it still runs, all-time, rather than failing on an unset
	// variable — which is what makes the \if idiom the right one.
	if unbounded := psqlRender(t, raw, nil); !strings.Contains(unbounded, "'-infinity'::timestamptz") {
		t.Error("with no -v the funnel should fall back to an all-time range")
	}
}

func readFunnel(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../../../../../../tools/analytics/funnel.sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// psqlRender interprets the meta-commands funnel.sql actually uses — `\set` and
// the `\if :{?var}` / `\else` / `\endif` defaulting idiom — with `vars` pre-set
// the way `psql -v name=value` pre-sets them.
//
// Interpreting rather than deleting the `\` lines is the whole point. Stripping
// them (what this helper did until 2026-08-25) meant the "we run the real file"
// test ran a version of the file with no variable handling at all: the bug where
// both bounds were unconditionally reassigned to ±infinity was 100% reproducible
// and structurally invisible here.
//
// Not psql itself, which would be more faithful still: the tests below insert
// their fixtures inside a transaction that is rolled back, and a psql in another
// process cannot see uncommitted rows. Anything beyond these four commands fails
// loudly rather than being skipped.
func psqlRender(t *testing.T, raw string, vars map[string]string) string {
	t.Helper()
	set := map[string]string{}
	for name, value := range vars {
		set[name] = value
	}
	var body []string
	inIf, skipping := false, false
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 || !strings.HasPrefix(fields[0], `\`) {
			if !skipping {
				body = append(body, line)
			}
			continue
		}
		switch fields[0] {
		case `\if`:
			if inIf {
				t.Fatalf("nested \\if is not interpreted here: %q", line)
			}
			name, ok := strings.CutPrefix(fields[1], `:{?`)
			if !ok || !strings.HasSuffix(name, "}") {
				t.Fatalf("only \\if :{?var} is interpreted here: %q", line)
			}
			_, defined := set[strings.TrimSuffix(name, "}")]
			inIf, skipping = true, !defined
		case `\else`:
			skipping = !skipping
		case `\endif`:
			inIf, skipping = false, false
		case `\set`:
			if !skipping && len(fields) == 3 {
				set[fields[1]] = psqlValue(fields[2])
			}
		default:
			t.Fatalf("funnel.sql uses a meta-command this test cannot interpret: %q", line)
		}
	}
	var replacements []string
	for name, value := range set {
		replacements = append(replacements, ":"+name, value)
	}
	return strings.NewReplacer(replacements...).Replace(strings.Join(body, "\n"))
}

// psqlValue unwraps one level of psql quoting: the outer pair of single quotes
// goes, and a doubled single quote inside becomes one. So the file's fallback for
// :from is the eleven-character string -infinity WITH its quotes, which is what
// makes the substituted :from a valid SQL literal.
func psqlValue(s string) string {
	if len(s) >= 2 && strings.HasPrefix(s, "'") && strings.HasSuffix(s, "'") {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	return s
}

func funnelQuery(t *testing.T, from, to string) string {
	t.Helper()
	return psqlRender(t, readFunnel(t), map[string]string{"from": from, "to": to})
}

// 01 §11.2's seventh item is "再次回來試跑或重新驗證", so a return is a Run or an
// evaluation on a second day — not a browser that reopened the catalogue for a
// second, which is what the query counted until 2026-08-25. In a twelve-person
// beta where seven people glance back and nobody runs anything again, the old
// shape printed 58% and it would have been read as retention.
//
// The day is bucketed in UTC, and this proves the bucket is not the server's:
// workspace A's two Runs are the same Los Angeles date on opposite sides of UTC
// midnight, and the session below is deliberately in another zone.
func TestTheFunnelCountsAReturnAsARunNotAVisit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	workspaceA := newFixture(t, a, pool, "funnel-return-a")
	workspaceB := newFixture(t, a, pool, "funnel-return-b")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL TIME ZONE 'America/Los_Angeles'"); err != nil {
		t.Fatal(err)
	}

	seedRunAt(t, tx, workspaceA, "succeeded", "2040-08-19T23:30:00Z")
	seedRunAt(t, tx, workspaceA, "succeeded", "2040-08-20T00:30:00Z")
	seedRunAt(t, tx, workspaceB, "succeeded", "2040-08-21T12:00:00Z")
	// B did come back with the browser, twice, and still has not run anything.
	// That is precisely the visit the definition does not ask about.
	for _, at := range []string{"2040-08-21T12:00:00Z", "2040-08-22T12:00:00Z"} {
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_events
            (event_name, session_id, workspace_id, occurred_at)
            VALUES ('session_started', 'funnel-return-browser', $1, $2)`,
			mustUUID(t, workspaceB.workspaceID), at); err != nil {
			t.Fatal(err)
		}
	}

	segment := funnelSegment(t, tx, funnelQuery(t, "'2040-08-18T00:00:00Z'", "'2040-08-23T00:00:00Z'"), 7)
	if segment.numerator != 1 || segment.denominator != 2 {
		t.Errorf("returning workspaces = %d of %d, want A only: 1 of 2",
			segment.numerator, segment.denominator)
	}
}

// 01 §11.2's sixth item is "完成試跑後打包下載". The denominator was every workspace
// that *created* a Run in the window and the numerator was every workspace that
// downloaded in it, from a different time column — so the numerator was not even
// a subset, and two workspaces of C's shape with one of B's printed 200%.
func TestSegmentSixIsCompletedRunsAndTheirOwnDownloads(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	completed := newFixture(t, a, pool, "funnel-six-completed")
	started := newFixture(t, a, pool, "funnel-six-started")
	earlier := newFixture(t, a, pool, "funnel-six-earlier")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// In the window and finished: the only workspace the definition asks about.
	seedRunAt(t, tx, completed, "succeeded", "2040-09-03T10:00:00Z")
	// In the window, never finished. Counted by `created_at` alone, which is what
	// the denominator used to be.
	seedRunAt(t, tx, started, "running", "2040-09-04T10:00:00Z")
	// Ran and succeeded *before* the window, downloaded inside it. This is the
	// row that used to reach the numerator without being in the denominator.
	seedRunAt(t, tx, earlier, "succeeded", "2040-08-30T10:00:00Z")
	seedDownloadAt(t, tx, earlier, "2040-09-02T10:00:00Z")

	segment := funnelSegment(t, tx, funnelQuery(t, "'2040-09-01T00:00:00Z'", "'2040-09-15T00:00:00Z'"), 6)
	if segment.numerator != 0 || segment.denominator != 1 {
		t.Errorf("packaged after a completed run = %d of %d, want 0 of 1",
			segment.numerator, segment.denominator)
	}
}

// 01 §11.2's first item is a detail view *after* an intent. A bare intersection
// counts the session that opened a shared /skills/{id} link, searched fruitlessly
// afterwards and left — and that number is the one the M5 exposure boundary is
// waiting on, so it may not be flattering by accident.
func TestSegmentOneNeedsTheSearchToComeFirst(t *testing.T) {
	pool := requireDB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	insert := func(name, session, at string) {
		t.Helper()
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_events
            (event_name, session_id, occurred_at) VALUES ($1, $2, $3)`, name, session, at); err != nil {
			t.Fatal(err)
		}
	}
	// The deep link, then a fruitless search: not a conversion.
	insert("skill_detail_viewed", "funnel-one-deeplink", "2040-10-01T11:00:00Z")
	insert("search_performed", "funnel-one-deeplink", "2040-10-01T11:05:00Z")
	// The journey the segment is about.
	insert("search_performed", "funnel-one-searcher", "2040-10-01T11:00:00Z")
	insert("skill_detail_viewed", "funnel-one-searcher", "2040-10-01T11:05:00Z")

	segment := funnelSegment(t, tx, funnelQuery(t, "'2040-10-01T00:00:00Z'", "'2040-10-02T00:00:00Z'"), 1)
	if segment.numerator != 1 || segment.denominator != 2 {
		t.Errorf("viewed a detail after searching = %d of %d, want 1 of 2",
			segment.numerator, segment.denominator)
	}
}

// 01 §11.2's second item is a fork or a trial that followed the detail view, and
// the query was a set intersection: a workspace that forked at 09:00 and opened a
// detail page at 17:00 counted as a conversion. Both halves of the OR are checked
// here, because a set intersection on either half is the same bug.
func TestSegmentTwoNeedsTheDetailViewToComeFirst(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	ran := newFixture(t, a, pool, "funnel-two-ran")
	ranFirst := newFixture(t, a, pool, "funnel-two-ran-first")
	forked := newFixture(t, a, pool, "funnel-two-forked")
	forkedFirst := newFixture(t, a, pool, "funnel-two-forked-first")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	view := func(f fixture, at string) {
		t.Helper()
		if _, err := tx.Exec(ctx, `INSERT INTO analytics_events
            (event_name, session_id, workspace_id, occurred_at)
            VALUES ('skill_detail_viewed', 'funnel-two', $1, $2)`,
			mustUUID(t, f.workspaceID), at); err != nil {
			t.Fatal(err)
		}
	}
	fork := func(f fixture, name, at string) {
		t.Helper()
		if _, err := tx.Exec(ctx, `INSERT INTO skills
            (workspace_id, name, forked_from_skill_id, created_at) VALUES ($1, $2, $3, $4)`,
			mustUUID(t, f.workspaceID), name, mustUUID(t, f.skillID), at); err != nil {
			t.Fatal(err)
		}
	}

	// The two journeys the segment is about.
	view(ran, "2040-11-01T10:00:00Z")
	seedRunAt(t, tx, ran, "succeeded", "2040-11-01T11:00:00Z")
	view(forked, "2040-11-01T10:00:00Z")
	fork(forked, "funnel-two-forked-copy", "2040-11-01T11:00:00Z")
	// The same two facts in the other order: nothing followed the view.
	seedRunAt(t, tx, ranFirst, "succeeded", "2040-11-01T10:00:00Z")
	view(ranFirst, "2040-11-01T11:00:00Z")
	fork(forkedFirst, "funnel-two-forked-first-copy", "2040-11-01T10:00:00Z")
	view(forkedFirst, "2040-11-01T11:00:00Z")

	segment := funnelSegment(t, tx, funnelQuery(t, "'2040-11-01T00:00:00Z'", "'2040-11-02T00:00:00Z'"), 2)
	if segment.numerator != 2 || segment.denominator != 4 {
		t.Errorf("acted after a detail view = %d of %d, want the two ordered journeys: 2 of 4",
			segment.numerator, segment.denominator)
	}
}

// 01 §11.2's sixth item is "完成試跑後打包下載", and the numerator only asked whether
// the workspace downloaded somewhere in the window — so downloading in the
// morning and succeeding in the afternoon read as a conversion.
func TestSegmentSixNeedsTheDownloadToFollowTheRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "funnel-six-order")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Both inside the window, wrong way round. seedRunAt leaves finished_at NULL,
	// which is why the query compares against COALESCE(finished_at, created_at).
	seedDownloadAt(t, tx, f, "2040-12-01T09:00:00Z")
	seedRunAt(t, tx, f, "succeeded", "2040-12-01T15:00:00Z")

	segment := funnelSegment(t, tx, funnelQuery(t, "'2040-12-01T00:00:00Z'", "'2040-12-02T00:00:00Z'"), 6)
	if segment.numerator != 0 || segment.denominator != 1 {
		t.Errorf("packaged after a completed run = %d of %d, want 0 of 1",
			segment.numerator, segment.denominator)
	}
}

// 01 §11.2's fourth item is "完成 Run 後認為結果有幫助". The denominator was the
// evaluations that got an answer, which measures how the people who bothered to
// reply felt — a different and much more flattering quantity than the one asked
// for, and BETA-002 prints it as the funnel.
func TestSegmentFourCountsCompletedRunsNotAnsweredQuestionnaires(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "funnel-four")
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	seedRunAt(t, tx, f, "succeeded", "2041-01-01T09:00:00Z")
	seedRunAt(t, tx, f, "succeeded", "2041-01-01T10:00:00Z")
	// Only the first run's owner ever answered. The second is silence, which the
	// old denominator dropped instead of counting.
	var runID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM runs WHERE workspace_id = $1 AND created_at = $2`,
		mustUUID(t, f.workspaceID), "2041-01-01T09:00:00Z").Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO evaluations
        (workspace_id, run_id, status, overall, evidence_complete, feedback_helpful, created_at)
        VALUES ($1, $2, 'completed', 'met', true, true, $3)`,
		mustUUID(t, f.workspaceID), mustUUID(t, runID), "2041-01-01T11:00:00Z"); err != nil {
		t.Fatal(err)
	}

	segment := funnelSegment(t, tx, funnelQuery(t, "'2041-01-01T00:00:00Z'", "'2041-01-02T00:00:00Z'"), 4)
	if segment.numerator != 1 || segment.denominator != 2 {
		t.Errorf("found the result helpful = %d of %d, want 1 of 2 completed runs",
			segment.numerator, segment.denominator)
	}
}

// The sessions that reach 01 §12's risk — an intent the platform could not parse
// — used to write no row at all, because the handler answers "no results" before
// the service that records the event ever runs. They are not a random sample of
// the denominator: they are the whole reason the M5 generation entry exists, so
// dropping them made segment 1 read systematically high.
func TestASearchTheProductCannotParseIsStillASearch(t *testing.T) {
	pool := requireDB(t)
	a := betaAPI(t, pool, policy.QuotaLimits{}, nil, 180*24*time.Hour)
	f := newFixture(t, a, pool, "alice-unparsed-intent")
	session := f.analyticsSession(t)
	if session == "" {
		t.Fatal("no analytics session cookie was issued")
	}

	// A single Han character (isComprehensible wants two runes) and a blank
	// query: both are intents somebody submitted.
	for _, q := range []string{"圖", ""} {
		if code := f.status(t, http.MethodGet, "/api/skills/search?q="+url.QueryEscape(q)); code != http.StatusOK {
			t.Fatalf("public search for %q: got %d", q, code)
		}
	}

	if n := betaCount(t, pool, `SELECT count(*) FROM analytics_events
        WHERE event_name = 'search_performed' AND session_id = $1
          AND result_count = 0 AND has_results = false`, session); n != 2 {
		t.Errorf("search_performed rows for two unparseable intents: %d, want 2", n)
	}
}

type funnelRow struct{ numerator, denominator int64 }

// funnelSegment runs the whole report inside the caller's transaction and returns
// one row. The whole report on purpose: a segment that stops parsing because
// another segment's SQL broke is a failure this should show.
func funnelSegment(t *testing.T, tx pgx.Tx, query string, want int) funnelRow {
	t.Helper()
	rows, err := tx.Query(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found *funnelRow
	for rows.Next() {
		var segment int
		var description, note string
		var row funnelRow
		if err := rows.Scan(&segment, &description, &row.numerator, &row.denominator, &note); err != nil {
			t.Fatal(err)
		}
		if segment == want {
			found = &row
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if found == nil {
		t.Fatalf("the funnel returned no segment %d", want)
	}
	return *found
}

// seedRunAt writes one run at a chosen instant, with the snapshot it needs.
// Straight into the tables rather than through the API, for seedBetaRun's reason:
// what is under test is what the report counts, and driving a real run to a
// terminal state would need a sandbox provider.
func seedRunAt(t *testing.T, tx pgx.Tx, f fixture, status, at string) {
	t.Helper()
	ctx := context.Background()
	var snapshotID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
		VALUES ($1, $2, 'do the thing', '[]'::jsonb, md5(random()::text))
		RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.testCaseID)).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, created_at)
		VALUES ($1, $2, $3, 'seed', '{}'::jsonb, '{}'::jsonb, $4, $5)`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, snapshotID), status, at,
	); err != nil {
		t.Fatal(err)
	}
}

// seedDownloadAt writes the domain fact segment 6 counts: a package artifact, its
// download_artifacts row and one download at a chosen instant.
func seedDownloadAt(t *testing.T, tx pgx.Tx, f fixture, at string) {
	t.Helper()
	ctx := context.Background()
	var artifactID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO artifacts (workspace_id, kind, file_name, content_type, size_bytes,
		                       content_hash, object_key, expires_at)
		VALUES ($1, 'download_package', 'package.zip', 'application/zip', 10,
		        'deadbeef', 'downloads/' || md5(random()::text), $2)
		RETURNING id::text`, mustUUID(t, f.workspaceID), at).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO download_artifacts (artifact_id, workspace_id, skill_version_id, target,
		                                profile_version, packager_version, manifest_hash, includes_test_cases)
		VALUES ($1, $2, $3, 'claude-code', '1', '1', 'deadbeef', false)`,
		mustUUID(t, artifactID), mustUUID(t, f.workspaceID), mustUUID(t, f.versionID)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO download_records (workspace_id, artifact_id, actor_user_id, downloaded_at)
		VALUES ($1, $2, $3, $4)`,
		mustUUID(t, f.workspaceID), mustUUID(t, artifactID), mustUUID(t, f.userID), at); err != nil {
		t.Fatal(err)
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
	RunID          string       `json:"run_id"`
	Status         string       `json:"status"`
	SkillID        string       `json:"skill_id"`
	SkillName      string       `json:"skill_name"`
	SkillVersionID string       `json:"skill_version_id"`
	TestCaseID     string       `json:"test_case_id"`
	CleanupStatus  labelledJSON `json:"cleanup_status"`
	CreatedAt      string       `json:"created_at"`
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
	if found.CreatedAt == "" || found.CleanupStatus.Label == "" {
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
	tag := uniqueWorklistLabel("history-per-case")
	f := newFixture(t, a, pool, tag)
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
	stranger := newFixture(t, a, pool, tag+"-stranger")
	if rows := stranger.listRunsForTestCase(t, f.testCaseID); len(rows) != 0 {
		t.Errorf("another workspace's runs leaked through test_case_id: %+v", rows)
	}
	if rows := f.listRunsForTestCase(t, "not-a-uuid"); len(rows) != 0 {
		t.Errorf("an unparseable test_case_id fell back to the whole history: %+v", rows)
	}

	var otherSnapshotID string
	if err := pool.QueryRow(context.Background(),
		"SELECT test_case_snapshot_id::text FROM runs WHERE id = $1", mustUUID(t, theirs.RunID),
	).Scan(&otherSnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, created_at)
		SELECT $1, $2, $3, 'test', clock_timestamp() + make_interval(secs => n)
		FROM generate_series(1, 500) AS n`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, otherSnapshotID),
	); err != nil {
		t.Fatal(err)
	}
	rows = f.listRunsForTestCase(t, f.testCaseID)
	if len(rows) != 1 || rows[0].RunID != mine.RunID {
		t.Fatalf("filtered history lost a matching run older than 500 unrelated rows: %+v", rows)
	}

	var mineSnapshotID string
	if err := pool.QueryRow(context.Background(),
		"SELECT test_case_snapshot_id::text FROM runs WHERE id = $1", mustUUID(t, mine.RunID),
	).Scan(&mineSnapshotID); err != nil {
		t.Fatal(err)
	}
	var newerMatchingID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, created_at)
		VALUES ($1, $2, $3, 'test', clock_timestamp() + interval '1 hour')
		RETURNING id::text`, mustUUID(t, f.workspaceID), mustUUID(t, f.versionID), mustUUID(t, mineSnapshotID),
	).Scan(&newerMatchingID); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Runs []runListView `json:"runs"`
	}
	url := f.base + "/runs?test_case_id=" + f.testCaseID + "&limit=1&offset=1"
	if code := getJSON(t, f.Client, url, &page); code != http.StatusOK {
		t.Fatalf("GET filtered second page: got %d", code)
	}
	if len(page.Runs) != 1 || page.Runs[0].RunID != mine.RunID || page.Runs[0].RunID == newerMatchingID {
		t.Fatalf("filter pagination was applied before filtering: %+v", page.Runs)
	}

	var indexDefinition string
	if err := pool.QueryRow(context.Background(), `
		SELECT indexdef FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'runs_test_case_snapshot_id_idx'`,
	).Scan(&indexDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(indexDefinition, "(test_case_snapshot_id, created_at DESC)") {
		t.Fatalf("run history index has the wrong columns: %s", indexDefinition)
	}
}
