package eval

// Database coverage for the Evaluation aggregate's invariants (ADR-026 decision
// 1, ADR-032 §4). These are the rules doc.go enumerates, and none of them can be
// proved without PostgreSQL: the partial unique index, the 0024 immutability
// trigger and the `status = 'pending'` predicates are the enforcement, and a Go
// assertion about a mock would only be re-stating the code under test.
//
// Point SKILLHUB_TEST_DATABASE_URL at a throwaway database and they run; leave it
// unset and they skip, so a CI job without a database reports "skipped" rather
// than a false pass.
//
// WARNING: TestMain drops and recreates schema "public" in that database.
// Never point SKILLHUB_TEST_DATABASE_URL at a database you care about.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

const aggregateDBURLEnv = "SKILLHUB_TEST_DATABASE_URL"

var aggregatePool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv(aggregateDBURLEnv)
	if dsn == "" {
		// 02:PORT-004. Without this, an unset or misspelled URL is indistinguishable
		// from a passing run: every database test removes itself and go test still
		// prints ok. CI sets SKILLHUB_REQUIRE_DB=1 so the service failing to come up
		// is a red build rather than a quiet one.
		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", aggregateDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run()) // every database test skips; see requireEvalDB
	}
	if err := validateDestructiveEvalDatabaseURL(dsn); err != nil {
		panic(err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(err)
	}
	unlock := lockTestSchema(ctx, pool)
	if err := migrateEvalSchema(ctx, pool); err != nil {
		panic(err)
	}
	aggregatePool = pool
	code := m.Run()
	unlock()
	pool.Close()
	os.Exit(code)
}

// validateDestructiveEvalDatabaseURL refuses to point the schema drop below at
// anything that is not an obviously disposable local database.
func validateDestructiveEvalDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", aggregateDBURLEnv, err)
	}
	host := strings.ToLower(u.Hostname())
	database := strings.Trim(u.Path, "/")
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return fmt.Errorf("%s must target localhost before destructive migrations", aggregateDBURLEnv)
	}
	if !strings.HasSuffix(strings.ToLower(database), "_test") {
		return fmt.Errorf("%s database name must end in _test before destructive migrations", aggregateDBURLEnv)
	}
	return nil
}

func TestDestructiveEvalDatabaseURLGuard(t *testing.T) {
	for _, raw := range []string{
		"postgres://user:pass@db.internal/skillhub_test",
		"postgres://user:pass@localhost/skillhub",
	} {
		if err := validateDestructiveEvalDatabaseURL(raw); err == nil {
			t.Fatalf("unsafe DSN accepted: %s", raw)
		}
	}
	if err := validateDestructiveEvalDatabaseURL("postgres://u:p@localhost/skillhub_test"); err != nil {
		t.Fatalf("safe test DSN rejected: %v", err)
	}
}

func migrateEvalSchema(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		return err
	}
	dir := filepath.Join("..", "..", "..", "..", "..", "db", "migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		// No arguments means the simple protocol, so a file with several
		// statements applies as one batch.
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	// River's tables are not in db/migrations and nothing here queues work, so
	// the schema is complete without them.
	return nil
}

func requireEvalDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if aggregatePool == nil {
		t.Skipf("%s not set; skipping Evaluation aggregate invariant test", aggregateDBURLEnv)
	}
	return aggregatePool
}

// --- fixtures ----------------------------------------------------------------

// seedRun writes the minimum chain an evaluation hangs off: user, workspace,
// skill, version, test case, snapshot, terminal run. Raw SQL rather than the
// other contexts' services, because what is under test is this table's rules and
// borrowing internal/ingest or internal/run here would be a cross-context import
// bought for a fixture (ADR-032 §1).
func seedRun(t *testing.T, pool *pgxpool.Pool) material {
	t.Helper()
	ctx := context.Background()
	tag := strings.ReplaceAll(t.Name(), "/", "-")

	var run RunFacts
	err := pool.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (email, display_name) VALUES ($1 || '@example.test', $1)
			RETURNING id
		), w AS (
			INSERT INTO workspaces (owner_user_id, name) SELECT id, $1 FROM u RETURNING id
		), s AS (
			INSERT INTO skills (workspace_id, name) SELECT id, $1 FROM w
			RETURNING id, workspace_id
		), v AS (
			INSERT INTO skill_versions
				(workspace_id, skill_id, version_number, content_hash, package_object_key)
			SELECT workspace_id, id, 1, $1, 'packages/' || $1 FROM s
			RETURNING id, workspace_id
		), tc AS (
			INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt)
			SELECT workspace_id, id, $1, 'do the thing' FROM s
			RETURNING id, workspace_id
		), snap AS (
			INSERT INTO test_case_snapshots
				(workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash)
			SELECT workspace_id, id, 'do the thing', '[]'::jsonb, $1 FROM tc
			RETURNING id, workspace_id
		)
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, status, provider)
		SELECT snap.workspace_id, v.id, snap.id, 'succeeded', 'test' FROM snap, v
		RETURNING id, workspace_id`, tag).Scan(&run.ID, &run.WorkspaceID)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	run.Status = "succeeded"
	return material{run: run, attempt: 1}
}

// aVerdict is a completed judgement with real evidence attached, so that an
// assertion about "the judgement did not change" is about something with content.
func aVerdict(summary, overall string) verdict {
	return verdict{
		overall: overall,
		summary: summary,
		results: []CriterionResult{{
			CriterionID: "c1", Text: "the file is written", Result: ResultPassed,
			Source: SourceModel, Reason: summary,
			Evidence: []EvidenceRef{{
				Kind: KindAgentOutput, Excerpt: "wrote report.md", Available: true,
			}},
		}},
		findings:         []Finding{},
		evidenceComplete: true,
		model:            "gpt-5.6-terra",
		promptVersion:    "judge-v1",
		rubricVersion:    "rubric-v1",
	}
}

// judgement is every column the 0024 trigger freezes on a completed row: what a
// report quotes, and therefore what must never differ between two reads.
type judgement struct {
	Status, Overall              string
	Summary                      string
	Results, Findings            string
	Model, PromptVersion, Rubric string
	EvidenceComplete             bool
}

func frozen(ev gen.Evaluation) judgement {
	return judgement{
		Status: ev.Status, Overall: ev.Overall, Summary: derefString(ev.Summary),
		Results: string(ev.CriterionResults), Findings: string(ev.DeterministicFindings),
		Model: derefString(ev.JudgeModel), PromptVersion: derefString(ev.JudgePromptVersion),
		Rubric: derefString(ev.RubricVersion), EvidenceComplete: ev.EvidenceComplete,
	}
}

func reload(t *testing.T, s *Service, m material, id pgtype.UUID) gen.Evaluation {
	t.Helper()
	ev, err := s.queries().GetEvaluationRevision(context.Background(), gen.GetEvaluationRevisionParams{
		ID: id, RunID: m.run.ID, WorkspaceID: m.run.WorkspaceID,
	})
	if err != nil {
		t.Fatalf("reload evaluation: %v", err)
	}
	return ev
}

func beginAndComplete(t *testing.T, s *Service, m material, v verdict) gen.Evaluation {
	t.Helper()
	ev, err := s.begin(context.Background(), m)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := s.complete(context.Background(), m, ev, v); err != nil {
		t.Fatalf("complete: %v", err)
	}
	return ev
}

// --- invariant 1, 4, 5: re-evaluation appends ---------------------------------

// The headline rule of ADR-026 decision 1. A second evaluation is a second row;
// the first keeps every word it was cited with and gains only a superseded_at.
func TestReEvaluationAppendsARevisionInsteadOfOverwriting(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("under the first rubric", OverallMet))
	before := frozen(reload(t, s, m, first.ID))

	second := beginAndComplete(t, s, m, aVerdict("under the second rubric", OverallNotMet))
	if second.ID == first.ID {
		t.Fatal("the second evaluation reused the first row; re-evaluation must append")
	}

	after := reload(t, s, m, first.ID)
	if frozen(after) != before {
		t.Fatalf("the superseded judgement changed:\n before %+v\n after  %+v", before, frozen(after))
	}
	if !after.SupersededAt.Valid {
		t.Fatal("the previous revision was not marked superseded")
	}

	current, err := s.Current(ctx, m.run.WorkspaceID, m.run.ID)
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if current.ID != second.ID {
		t.Fatal("the standing verdict is not the newest revision")
	}
	if current.SupersededAt.Valid {
		t.Fatal("the newest revision was marked superseded")
	}

	revisions, err := s.Revisions(ctx, m.run.WorkspaceID, m.run.ID)
	if err != nil {
		t.Fatalf("revisions: %v", err)
	}
	if len(revisions) != 2 {
		t.Fatalf("want 2 revisions on record, got %d", len(revisions))
	}
}

// --- invariant 2: a completed judgement is a fact -----------------------------

// Two halves of the same rule. The service refuses because the row is no longer
// pending; the database refuses because 0024 froze the columns. The second half
// is the one that still holds when a future writer forgets the first.
func TestASettledVerdictCannotBeRewritten(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	ev := beginAndComplete(t, s, m, aVerdict("the original verdict", OverallMet))
	before := frozen(reload(t, s, m, ev.ID))

	err := s.complete(ctx, m, ev, aVerdict("a second opinion", OverallNotMet))
	if !errors.Is(err, errEvaluationSettled) {
		t.Fatalf("re-completing a settled evaluation: want errEvaluationSettled, got %v", err)
	}
	if got := frozen(reload(t, s, m, ev.ID)); got != before {
		t.Fatalf("the stored judgement changed:\n before %+v\n after  %+v", before, got)
	}

	for column, value := range map[string]any{
		"overall":           OverallNotMet,
		"summary":           "edited afterwards",
		"criterion_results": []byte(`[]`),
		"judge_model":       "something-cheaper",
		"evidence_complete": false,
	} {
		//nolint:gosec // column is a literal from the map above, not input.
		_, err := s.Pool.Exec(ctx,
			fmt.Sprintf("UPDATE evaluations SET %s = $1 WHERE id = $2", column), value, ev.ID)
		if err == nil {
			t.Fatalf("the database allowed %s to be rewritten on a completed evaluation", column)
		}
	}
	if got := frozen(reload(t, s, m, ev.ID)); got != before {
		t.Fatalf("a refused UPDATE still changed the row:\n before %+v\n after  %+v", before, got)
	}
}

// --- invariant 3: one terminal per revision -----------------------------------

// complete and fail race whenever the recovery sweep meets the worker that is
// still running. Exactly one wins; the loser must not turn a verdict into a
// failure, or a failure into a verdict.
func TestARevisionSettlesOnce(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	ctx := context.Background()

	t.Run("fail after complete leaves the verdict standing", func(t *testing.T) {
		m := seedRun(t, s.Pool)
		ev := beginAndComplete(t, s, m, aVerdict("a real verdict", OverallMet))
		before := frozen(reload(t, s, m, ev.ID))

		if err := s.fail(ctx, m, ev, nil, false, errors.New("recovery sweep was late")); err != nil {
			t.Fatalf("fail on a completed revision: %v", err)
		}
		if got := frozen(reload(t, s, m, ev.ID)); got != before {
			t.Fatalf("a late failure rewrote a verdict:\n before %+v\n after  %+v", before, got)
		}
	})

	t.Run("complete after fail does not resurrect the revision", func(t *testing.T) {
		m := seedRun(t, s.Pool)
		ev, err := s.begin(ctx, m)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := s.fail(ctx, m, ev, nil, false, errors.New("the judge was unreachable")); err != nil {
			t.Fatalf("fail: %v", err)
		}
		if err := s.complete(ctx, m, ev, aVerdict("late arrival", OverallMet)); !errors.Is(err, errEvaluationSettled) {
			t.Fatalf("completing a failed revision: want errEvaluationSettled, got %v", err)
		}
		if got := reload(t, s, m, ev.ID); got.Status != StatusFailed {
			t.Fatalf("a failed revision became %q", got.Status)
		}
	})
}

// --- invariant 4: superseded_at is stamped once -------------------------------

// The stamp records when *this* judgement stopped being the standing one. A third
// evaluation must not move the first one's stamp forward, and nothing may clear
// it: an un-superseded history row would be a second current verdict.
func TestSupersededAtIsStampedOnceAndNeverCleared(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("first", OverallMet))
	beginAndComplete(t, s, m, aVerdict("second", OverallNotMet))
	stamped := reload(t, s, m, first.ID).SupersededAt
	if !stamped.Valid {
		t.Fatal("the first revision was not superseded")
	}

	beginAndComplete(t, s, m, aVerdict("third", OverallPartiallyMet))
	if again := reload(t, s, m, first.ID).SupersededAt; again.Time != stamped.Time {
		t.Fatalf("a third evaluation re-stamped the first revision: %v -> %v", stamped.Time, again.Time)
	}

	// The 0024 trigger has to leave superseded_at writable — stamping it is how a
	// revision is retired — so the guard against clearing it is the partial unique
	// index, not the trigger.
	if _, err := s.Pool.Exec(ctx,
		"UPDATE evaluations SET superseded_at = NULL WHERE id = $1", first.ID); err == nil {
		t.Fatal("a superseded revision was restored to current; two verdicts would now stand")
	}
}

// --- invariant 6: a pending revision is not superseded from under itself ------

// A redelivered job must join the work in flight rather than start a second
// judgement. The judge is a metered call (ADR-026 decision 4), and the abandoned
// pending row would never reach a terminal.
func TestASecondEvaluationWhileOneIsPendingIsRefused(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first, err := s.begin(ctx, m)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := s.begin(ctx, m); !errors.Is(err, errEvaluationInProgress) {
		t.Fatalf("second begin while pending: want errEvaluationInProgress, got %v", err)
	}
	if got := reload(t, s, m, first.ID); got.SupersededAt.Valid {
		t.Fatal("the pending revision was superseded and left with no terminal")
	}
}

// --- what append-only does not constrain: feedback ----------------------------

// The user may change their mind (EVAL-001 clause 4), and doing so must not move
// a single word of the judgement. It also must not reach a superseded revision:
// feedback is about the verdict on screen, which is always the current one.
func TestFeedbackIsWritableAndOnlyOnTheCurrentRevision(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("first", OverallMet))
	if _, err := s.SetFeedback(ctx, m.run.WorkspaceID, m.run.ID, true, "useful"); err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	before := frozen(reload(t, s, m, first.ID))

	// Changing the answer replaces it rather than being refused.
	if _, err := s.SetFeedback(ctx, m.run.WorkspaceID, m.run.ID, false, ""); err != nil {
		t.Fatalf("change feedback: %v", err)
	}
	changed := reload(t, s, m, first.ID)
	if changed.FeedbackHelpful == nil || *changed.FeedbackHelpful {
		t.Fatal("the second answer was not recorded")
	}
	if changed.FeedbackComment != nil {
		t.Fatal("an empty comment must clear the previous one, not store \"\"")
	}
	if got := frozen(changed); got != before {
		t.Fatalf("feedback rewrote the judgement:\n before %+v\n after  %+v", before, got)
	}

	second := beginAndComplete(t, s, m, aVerdict("second", OverallNotMet))
	if _, err := s.SetFeedback(ctx, m.run.WorkspaceID, m.run.ID, true, "the new one is better"); err != nil {
		t.Fatalf("set feedback after re-evaluation: %v", err)
	}
	stale := reload(t, s, m, first.ID)
	if stale.FeedbackHelpful == nil || *stale.FeedbackHelpful {
		t.Fatal("feedback landed on the superseded revision")
	}
	if got := reload(t, s, m, second.ID); got.FeedbackComment == nil {
		t.Fatal("feedback did not land on the current revision")
	}
}

// --- invariant 7: nothing here writes runs ------------------------------------

// ADR-025 in one assertion: a `not_met` verdict on a run that executed cleanly is
// an ordinary state, and the run's terminal status is not the evaluation's to
// move. Cheap to state, and the regression it guards is silent.
func TestAVerdictDoesNotTouchTheRunsRow(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	var before string
	if err := s.Pool.QueryRow(ctx,
		"SELECT to_jsonb(runs)::text FROM runs WHERE id = $1", m.run.ID).Scan(&before); err != nil {
		t.Fatalf("read run: %v", err)
	}

	beginAndComplete(t, s, m, aVerdict("the task was not achieved", OverallNotMet))

	var after string
	if err := s.Pool.QueryRow(ctx,
		"SELECT to_jsonb(runs)::text FROM runs WHERE id = $1", m.run.ID).Scan(&after); err != nil {
		t.Fatalf("read run: %v", err)
	}
	if after != before {
		t.Fatalf("evaluating changed the run row:\n before %s\n after  %s", before, after)
	}
	// Guard against the comparison passing on two unparseable strings.
	var probe map[string]any
	if err := json.Unmarshal([]byte(after), &probe); err != nil || probe["status"] != string(gen.RunStatusSucceeded) {
		t.Fatalf("run row did not read back as a succeeded run: %v (%v)", after, err)
	}
}

// lockTestSchema serialises the packages that reset this database.
//
// apiserver, eval and registry each drop and recreate schema "public" in
// SKILLHUB_TEST_DATABASE_URL, and `go test ./...` runs packages concurrently:
// one package's reset lands while another is mid-run, and the second one sees
// "relation does not exist". Held on one connection for the whole package run
// rather than only across the migration, because the hazard is a reset
// colliding with somebody else's *tests*, not with their migration.
//
// Session-scoped, so a crashed run releases it along with its connection and a
// stale lock cannot wedge CI. Every package that resets this database must take
// it; one that forgets fails loudly with the panic above rather than silently.
func lockTestSchema(ctx context.Context, pool *pgxpool.Pool) func() {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		panic(err)
	}
	if _, err := conn.Exec(ctx,
		"SELECT pg_advisory_lock(hashtextextended('skillhub:test-schema', 0))"); err != nil {
		panic(err)
	}
	return func() {
		_, _ = conn.Exec(ctx,
			"SELECT pg_advisory_unlock(hashtextextended('skillhub:test-schema', 0))")
		conn.Release()
	}
}

// --- ADR-026 decision 1: every judgement records the conditions it was made under

// The half of that clause a verdict never needed. A `failed` revision is the one
// where "which judge could not answer" is the first question, and until begin
// wrote these three columns the answer existed only in the `evaluation_started`
// trace event — which retention drops with its partition, so the failure outlived
// the only record of what it had been attempted with.
//
// Each subtest also fixes the meaning of NULL there: absent is a statement about
// the attempt, never a forgotten write.
func TestAFailedRevisionRecordsWhatItWasAttemptedWith(t *testing.T) {
	pool := requireEvalDB(t)
	ctx := context.Background()

	// begin then fail, which is the exact pair a judge outage produces.
	failed := func(t *testing.T, s *Service, m material) gen.Evaluation {
		t.Helper()
		ev, err := s.begin(ctx, m)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if err := s.fail(ctx, m, ev, nil, false, errors.New("the judge was unreachable")); err != nil {
			t.Fatalf("fail: %v", err)
		}
		got := reload(t, s, m, ev.ID)
		if got.Status != StatusFailed {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		return got
	}

	t.Run("the declared conditions survive the failure", func(t *testing.T) {
		s := &Service{
			Pool: pool, Judge: &llmclient.Client{},
			JudgeModel: "gpt-5.6-terra", JudgePromptVersion: "judge-run@v1",
		}
		m := seedRun(t, s.Pool)
		m.rubric = &testlab.Rubric{Version: "content-007/writing/v1"}

		got := failed(t, s, m)
		if derefString(got.JudgeModel) != "gpt-5.6-terra" ||
			derefString(got.JudgePromptVersion) != "judge-run@v1" {
			t.Errorf("a failed revision must still say which judge could not answer, got %q / %q",
				derefString(got.JudgeModel), derefString(got.JudgePromptVersion))
		}
		if derefString(got.RubricVersion) != "content-007/writing/v1" {
			t.Errorf("the rubric it would have been judged under is frozen in the snapshot and knowable, got %q",
				derefString(got.RubricVersion))
		}
	})

	t.Run("a deployment with no judge records NULL, not a model name", func(t *testing.T) {
		s := &Service{Pool: pool} // Judge nil: nothing was ever going to be called.
		m := seedRun(t, s.Pool)

		got := failed(t, s, m)
		if got.JudgeModel != nil || got.JudgePromptVersion != nil {
			t.Errorf("naming a judge here describes a call this deployment cannot make, got %q / %q",
				derefString(got.JudgeModel), derefString(got.JudgePromptVersion))
		}
		if got.RubricVersion != nil {
			t.Errorf("this snapshot froze no rubric; '' would claim one, got %q", derefString(got.RubricVersion))
		}
	})

	t.Run("an undeclared prompt version stays NULL while the model is recorded", func(t *testing.T) {
		s := &Service{Pool: pool, Judge: &llmclient.Client{}}
		m := seedRun(t, s.Pool)

		got := failed(t, s, m)
		if derefString(got.JudgeModel) != "gpt-5.6-terra" {
			t.Errorf("the ADR-026 decision 4 tier is a real declaration even unconfigured, got %q",
				derefString(got.JudgeModel))
		}
		if got.JudgePromptVersion != nil {
			t.Errorf("the prompt version is learned from a response this attempt never got, "+
				"so %q is a placeholder standing where a fact belongs", derefString(got.JudgePromptVersion))
		}
	})

	t.Run("a completed row reports what ran, not what was declared", func(t *testing.T) {
		s := &Service{
			Pool: pool, Judge: &llmclient.Client{},
			JudgeModel: "declared-and-never-used", JudgePromptVersion: "declared-prompt",
		}
		m := seedRun(t, s.Pool)
		m.rubric = &testlab.Rubric{Version: "declared-rubric"}

		ev := beginAndComplete(t, s, m, aVerdict("what actually ran", OverallMet))
		got := reload(t, s, m, ev.ID)
		if derefString(got.JudgeModel) != "gpt-5.6-terra" ||
			derefString(got.JudgePromptVersion) != "judge-v1" ||
			derefString(got.RubricVersion) != "rubric-v1" {
			t.Errorf("the declaration outranked the response: got %q / %q / %q",
				derefString(got.JudgeModel), derefString(got.JudgePromptVersion),
				derefString(got.RubricVersion))
		}
	})
}
