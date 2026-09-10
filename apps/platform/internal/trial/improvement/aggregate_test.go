package eval

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

		if os.Getenv("SKILLHUB_REQUIRE_DB") == "1" {
			fmt.Fprintf(os.Stderr, "SKILLHUB_REQUIRE_DB=1 but %s is unset; this run would have skipped every database test and still reported success\n", aggregateDBURLEnv)
			os.Exit(1)
		}
		os.Exit(m.Run())
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

		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	return nil
}

func requireEvalDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if aggregatePool == nil {
		t.Skipf("%s not set; skipping Evaluation aggregate invariant test", aggregateDBURLEnv)
	}
	return aggregatePool
}

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

	if _, err := s.Pool.Exec(ctx,
		"UPDATE evaluations SET superseded_at = NULL WHERE id = $1", first.ID); err == nil {
		t.Fatal("a superseded revision was restored to current; two verdicts would now stand")
	}
}

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

func TestFeedbackIsWritableAndOnlyOnTheCurrentRevision(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("first", OverallMet))
	if _, err := s.SetFeedback(ctx, m.run.WorkspaceID, m.run.ID, true, "useful"); err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	before := frozen(reload(t, s, m, first.ID))

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

	var probe map[string]any
	if err := json.Unmarshal([]byte(after), &probe); err != nil || probe["status"] != string(gen.RunStatusSucceeded) {
		t.Fatalf("run row did not read back as a succeeded run: %v (%v)", after, err)
	}
}

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

func TestAFailedRevisionRecordsWhatItWasAttemptedWith(t *testing.T) {
	pool := requireEvalDB(t)
	ctx := context.Background()

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
		s := &Service{Pool: pool}
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
