package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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
	tag := strings.ReplaceAll(t.Name(), "/", "-")
	return seedNamedRun(t, pool, tag)
}

func seedNamedRun(t *testing.T, pool *pgxpool.Pool, tag string) material {
	t.Helper()
	ctx := context.Background()

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
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, status, provider, finished_at)
		SELECT snap.workspace_id, v.id, snap.id, 'succeeded', 'test', now() FROM snap, v
		RETURNING id, workspace_id`, tag).Scan(&run.ID, &run.WorkspaceID)
	if err != nil {
		t.Fatalf("seed run: %v", err)
	}
	run.Status = "succeeded"
	return material{run: run, attempt: 1}
}

func aVerdict(summary string, overall Overall) verdict {
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

func seedImprovedVersion(t *testing.T, pool *pgxpool.Pool, runID pgtype.UUID, number int, contentHash string) pgtype.UUID {
	t.Helper()
	var versionID pgtype.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO skill_versions (workspace_id, skill_id, version_number, content_hash, package_object_key)
		SELECT v.workspace_id, v.skill_id, $2, $3, 'packages/' || $3
		FROM runs r
		JOIN skill_versions v ON v.id = r.skill_version_id
		WHERE r.id = $1
		RETURNING id`, runID, number, contentHash).Scan(&versionID)
	if err != nil {
		t.Fatalf("seed improved version: %v", err)
	}
	return versionID
}

func seedSuggestion(t *testing.T, s *Service, workspaceID, evaluationID pgtype.UUID, targetPath string) gen.EvaluationSuggestion {
	t.Helper()
	suggestion, err := s.queries().CreateEvaluationSuggestion(context.Background(), gen.CreateEvaluationSuggestionParams{
		WorkspaceID: workspaceID, EvaluationID: evaluationID, Category: string(SuggestionSkill),
		Problem: "the steps are vague", Evidence: []byte(`[]`), TargetPath: targetPath,
		ProposedContent: "clearer steps", ExpectedImpact: "fewer retries",
	})
	if err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	return suggestion
}

func appliedTargetPaths(t *testing.T, s *Service, workspaceID, versionID pgtype.UUID) []string {
	t.Helper()
	applied, err := s.AppliedSuggestions(context.Background(), workspaceID, versionID)
	if err != nil {
		t.Fatalf("applied suggestions: %v", err)
	}
	paths := make([]string, len(applied))
	for i, suggestion := range applied {
		paths[i] = suggestion.TargetPath
	}
	sort.Strings(paths)
	return paths
}

func TestSuggestionApplicationsRetainEveryVersionAndRejectReplays(t *testing.T) {
	for _, tc := range []struct {
		name          string
		firstVersionA bool
	}{
		{name: "A then B", firstVersionA: true},
		{name: "B then A", firstVersionA: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{Pool: requireEvalDB(t)}
			m := seedRun(t, s.Pool)
			evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
			x := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
			y := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "Y")
			versionA := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-a")
			versionB := seedImprovedVersion(t, s.Pool, m.run.ID, 3, t.Name()+"-b")

			apply := func(versionID pgtype.UUID, suggestionIDs []pgtype.UUID) {
				t.Helper()
				if err := s.RecordSuggestionsApplied(context.Background(), m.run.WorkspaceID, evaluation.ID, versionID, suggestionIDs); err != nil {
					t.Fatalf("record suggestions applied: %v", err)
				}
			}
			if tc.firstVersionA {
				apply(versionA, []pgtype.UUID{x.ID})
				apply(versionB, []pgtype.UUID{x.ID, y.ID})
			} else {
				apply(versionB, []pgtype.UUID{x.ID, y.ID})
				apply(versionA, []pgtype.UUID{x.ID})
			}
			apply(versionA, []pgtype.UUID{x.ID})
			apply(versionB, []pgtype.UUID{x.ID, y.ID})

			if got, want := appliedTargetPaths(t, s, m.run.WorkspaceID, versionA), []string{"X"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("suggestions applied to A = %v, want %v", got, want)
			}
			if got, want := appliedTargetPaths(t, s, m.run.WorkspaceID, versionB), []string{"X", "Y"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("suggestions applied to B = %v, want %v", got, want)
			}

			var pairs, appliedEvents int
			var xWitness, yWitness pgtype.UUID
			if err := s.Pool.QueryRow(context.Background(), `
				SELECT count(*) FROM evaluation_suggestion_applications
				WHERE workspace_id = $1 AND suggestion_id = ANY($2)`, m.run.WorkspaceID, []pgtype.UUID{x.ID, y.ID}).Scan(&pairs); err != nil {
				t.Fatalf("count application pairs: %v", err)
			}
			if err := s.Pool.QueryRow(context.Background(), `
				SELECT applied_skill_version_id FROM evaluation_suggestions WHERE id = $1`, x.ID).Scan(&xWitness); err != nil {
				t.Fatalf("read X scalar witness: %v", err)
			}
			if err := s.Pool.QueryRow(context.Background(), `
				SELECT applied_skill_version_id FROM evaluation_suggestions WHERE id = $1`, y.ID).Scan(&yWitness); err != nil {
				t.Fatalf("read Y scalar witness: %v", err)
			}
			if err := s.Pool.QueryRow(context.Background(), `
				SELECT count(*) FROM outbox_events
				WHERE aggregate_id = $1 AND event_type = 'evaluation.suggestions_applied'`, evaluation.ID).Scan(&appliedEvents); err != nil {
				t.Fatalf("count applied outbox events: %v", err)
			}
			wantX := versionB
			if tc.firstVersionA {
				wantX = versionA
			}
			if pairs != 3 || xWitness != wantX || yWitness != versionB || appliedEvents != 2 {
				t.Fatalf("pairs=%d, scalar X=%v, scalar Y=%v, outbox=%d; want 3, %v, %v, 2", pairs, xWitness, yWitness, appliedEvents, wantX, versionB)
			}
		})
	}
}

func TestApplyingSuggestionsAcceptsThePendingOneAndKeepsAnEarlierAcceptanceTime(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	accepted := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	pending := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "Y")
	ctx := context.Background()
	decided, err := s.Decide(ctx, m.run.WorkspaceID, accepted.ID, DecisionAccepted)
	if err != nil {
		t.Fatalf("accept X: %v", err)
	}
	version := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-improved")
	if err := s.RecordSuggestionsApplied(ctx, m.run.WorkspaceID, evaluation.ID, version, []pgtype.UUID{accepted.ID, pending.ID}); err != nil {
		t.Fatalf("record suggestions applied: %v", err)
	}
	decision := func(id pgtype.UUID) (string, pgtype.Timestamptz) {
		t.Helper()
		var d string
		var at pgtype.Timestamptz
		if err := s.Pool.QueryRow(ctx, "SELECT decision, decided_at FROM evaluation_suggestions WHERE id = $1", id).Scan(&d, &at); err != nil {
			t.Fatalf("read decision: %v", err)
		}
		return d, at
	}
	if d, at := decision(accepted.ID); d != string(DecisionAccepted) || !at.Time.Equal(decided.DecidedAt.Time) {
		t.Fatalf("X = %q decided at %v, want accepted at its first acceptance %v", d, at.Time, decided.DecidedAt.Time)
	}
	if d, at := decision(pending.ID); d != string(DecisionAccepted) || !at.Valid {
		t.Fatalf("Y = %q, decision time set %v; want accepted with a decision time", d, at.Valid)
	}
}

func TestSuggestionApplicationRejectsAVersionFromAnotherWorkspace(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	first := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, first, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, first.run.WorkspaceID, evaluation.ID, "X")
	second := seedNamedRun(t, s.Pool, t.Name()+"-foreign")
	var foreignVersion pgtype.UUID
	if err := s.Pool.QueryRow(context.Background(), "SELECT skill_version_id FROM runs WHERE id = $1", second.run.ID).Scan(&foreignVersion); err != nil {
		t.Fatalf("read foreign version: %v", err)
	}
	if _, err := s.Pool.Exec(context.Background(), `
		INSERT INTO evaluation_suggestion_applications (workspace_id, suggestion_id, skill_version_id)
		VALUES ($1, $2, $3)`, first.run.WorkspaceID, suggestion.ID, foreignVersion); err == nil {
		t.Fatal("an application accepted a skill version from another workspace")
	} else {
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "23503" {
			t.Fatalf("cross-workspace version error = %v, want foreign key SQLSTATE 23503", err)
		}
	}
}

func TestSuggestionApplicationsAreImmutableUntilTheirVersionIsPurged(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	first := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-first")
	second := seedImprovedVersion(t, s.Pool, m.run.ID, 3, t.Name()+"-second")
	ctx := context.Background()
	for _, version := range []pgtype.UUID{first, second} {
		if err := s.RecordSuggestionsApplied(ctx, m.run.WorkspaceID, evaluation.ID, version, []pgtype.UUID{suggestion.ID}); err != nil {
			t.Fatal(err)
		}
	}
	for _, statement := range []string{
		"UPDATE evaluation_suggestion_applications SET applied_at = applied_at + interval '1 second' WHERE suggestion_id = $1",
		"DELETE FROM evaluation_suggestion_applications WHERE suggestion_id = $1",
	} {
		_, err := s.Pool.Exec(ctx, statement, suggestion.ID)
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "23001" {
			t.Fatalf("changing provenance returned %v, want immutable SQLSTATE 23001", err)
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, "SET LOCAL skillhub.purge = 'on'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM skill_versions WHERE id = $1", second); err != nil {
		t.Fatalf("purge the later version: %v", err)
	}
	var remaining pgtype.UUID
	if err := tx.QueryRow(ctx, "SELECT skill_version_id FROM evaluation_suggestion_applications WHERE suggestion_id = $1", suggestion.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM evaluation_suggestion_applications WHERE suggestion_id = $1", suggestion.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if remaining != first || count != 1 {
		t.Fatalf("provenance after purge = %v, count %d; want the first version only", remaining, count)
	}
}

func TestSuggestionVersionProvenanceMigrationBackfillsLegacyScalarAndKeepsPurgeFence(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	evaluation := beginAndComplete(t, s, m, aVerdict("complete", OverallMet))
	suggestion := seedSuggestion(t, s, m.run.WorkspaceID, evaluation.ID, "X")
	legacyVersion := seedImprovedVersion(t, s.Pool, m.run.ID, 2, t.Name()+"-legacy")
	fencedVersion := seedImprovedVersion(t, s.Pool, m.run.ID, 3, t.Name()+"-fenced")
	ctx := context.Background()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin migration boundary transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, "UPDATE evaluation_suggestions SET decision = 'accepted', decided_at = now(), applied_skill_version_id = $1 WHERE id = $2", legacyVersion, suggestion.ID); err != nil {
		t.Fatalf("prepare legacy migration state: %v", err)
	}
	for _, statement := range []string{
		"DROP TABLE evaluation_suggestion_applications",
		"ALTER TABLE evaluation_suggestions DROP CONSTRAINT evaluation_suggestions_id_workspace_key",
		"ALTER TABLE skill_versions DROP CONSTRAINT skill_versions_id_workspace_key",
	} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			t.Fatalf("prepare legacy migration state: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET purge_started_at = now()
		WHERE id = (SELECT owner_user_id FROM workspaces WHERE id = $1)`, m.run.WorkspaceID); err != nil {
		t.Fatalf("prepare legacy migration state: %v", err)
	}
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "db", "migrations", "0071_suggestion_version_provenance.sql"))
	if err != nil {
		t.Fatalf("read provenance migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(body)); err != nil {
		t.Fatalf("execute provenance migration: %v", err)
	}

	var backfilled pgtype.UUID
	if err := tx.QueryRow(ctx, `
		SELECT skill_version_id FROM evaluation_suggestion_applications
		WHERE workspace_id = $1 AND suggestion_id = $2`, m.run.WorkspaceID, suggestion.ID).Scan(&backfilled); err != nil {
		t.Fatalf("read backfilled application: %v", err)
	}
	if backfilled != legacyVersion {
		t.Fatalf("backfilled version = %v, want %v", backfilled, legacyVersion)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO evaluation_suggestion_applications (workspace_id, suggestion_id, skill_version_id)
		VALUES ($1, $2, $3)`, m.run.WorkspaceID, suggestion.ID, fencedVersion); err == nil {
		t.Fatal("migration left a provenance write path open during account purge")
	} else {
		var databaseError *pgconn.PgError
		if !errors.As(err, &databaseError) || databaseError.Code != "55000" {
			t.Fatalf("purge fence error = %v, want SQLSTATE 55000", err)
		}
	}
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

func TestAFailedVerdictIsFrozenLikeASettledOne(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	ev, err := s.begin(ctx, m)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := s.fail(ctx, m, ev, nil, false, errors.New("the judge was unreachable")); err != nil {
		t.Fatalf("fail: %v", err)
	}
	before := frozen(reload(t, s, m, ev.ID))

	for column, value := range map[string]any{
		"status":            string(StatusCompleted),
		"overall":           OverallMet,
		"summary":           "edited afterwards",
		"evidence_complete": true,
	} {
		//nolint:gosec // column is a literal from the map above, not input.
		_, err := s.Pool.Exec(ctx,
			fmt.Sprintf("UPDATE evaluations SET %s = $1 WHERE id = $2", column), value, ev.ID)
		if err == nil {
			t.Fatalf("the database allowed %s to be rewritten on a failed evaluation", column)
		}
	}
	if _, err := s.Pool.Exec(ctx, "DELETE FROM evaluations WHERE id = $1", ev.ID); err == nil {
		t.Fatal("the database allowed a failed evaluation to be deleted")
	}
	if got := frozen(reload(t, s, m, ev.ID)); got != before {
		t.Fatalf("a refused write still changed the row:\n before %+v\n after  %+v", before, got)
	}

	if _, err := s.Pool.Exec(ctx,
		"UPDATE evaluations SET feedback_helpful = true, superseded_at = now(), updated_at = now() WHERE id = $1",
		ev.ID); err != nil {
		t.Fatalf("feedback and supersession must stay writable on a failed evaluation: %v", err)
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
		if got := reload(t, s, m, ev.ID); got.Status != string(StatusFailed) {
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
		if got.Status != string(StatusFailed) {
			t.Fatalf("status = %q, want failed", got.Status)
		}
		return got
	}

	t.Run("the declared conditions survive the failure", func(t *testing.T) {
		s := &Service{
			Pool: pool, Judge: JudgeOrNone(&llmclient.Client{}),
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
		s := &Service{Pool: pool, Judge: JudgeOrNone(&llmclient.Client{})}
		m := seedRun(t, s.Pool)

		got := failed(t, s, m)
		if derefString(got.JudgeModel) != "gpt-5.6-terra" {
			t.Errorf("the judge tier is a real declaration even when unconfigured, got %q",
				derefString(got.JudgeModel))
		}
		if got.JudgePromptVersion != nil {
			t.Errorf("the prompt version is learned from a response this attempt never got, "+
				"so %q is a placeholder standing where a fact belongs", derefString(got.JudgePromptVersion))
		}
	})

	t.Run("a completed row reports what ran, not what was declared", func(t *testing.T) {
		s := &Service{
			Pool: pool, Judge: JudgeOrNone(&llmclient.Client{}),
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

func TestEveryEvaluationCommandLeavesItsEventInTheOutbox(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("first", OverallMet))
	second := beginAndComplete(t, s, m, aVerdict("second", OverallNotMet))
	if _, err := s.SetFeedback(ctx, m.run.WorkspaceID, m.run.ID, true, "useful"); err != nil {
		t.Fatalf("set feedback: %v", err)
	}
	suggestion, err := s.queries().CreateEvaluationSuggestion(ctx, gen.CreateEvaluationSuggestionParams{
		WorkspaceID: m.run.WorkspaceID, EvaluationID: second.ID, Category: string(SuggestionSkill),
		Problem: "the steps are vague", Evidence: []byte(`[]`), TargetPath: "SKILL.md",
		ProposedContent: "clearer steps", ExpectedImpact: "fewer retries",
	})
	if err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	if _, err := s.Decide(ctx, m.run.WorkspaceID, suggestion.ID, DecisionAccepted); err != nil {
		t.Fatalf("decide: %v", err)
	}
	var version pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT skill_version_id FROM runs WHERE id = $1`, m.run.ID).Scan(&version); err != nil {
		t.Fatalf("read the run's version: %v", err)
	}
	for range 2 {
		if err := s.RecordSuggestionsApplied(ctx, m.run.WorkspaceID, second.ID, version,
			[]pgtype.UUID{suggestion.ID}); err != nil {
			t.Fatalf("record applied: %v", err)
		}
	}
	if err := s.complete(ctx, m, second, aVerdict("a refused second opinion", OverallMet)); !errors.Is(err, errEvaluationSettled) {
		t.Fatalf("re-completing: want errEvaluationSettled, got %v", err)
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT event_type, aggregate_id FROM outbox_events
		WHERE correlation_id = $1 AND aggregate_type = 'evaluation'`, m.run.ID)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	revision := map[pgtype.UUID]string{first.ID: "first", second.ID: "second"}
	got := map[string]int{}
	for rows.Next() {
		var eventType string
		var aggregateID pgtype.UUID
		if err := rows.Scan(&eventType, &aggregateID); err != nil {
			t.Fatalf("scan outbox: %v", err)
		}
		got[eventType+" of the "+revision[aggregateID]]++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	want := map[string]int{
		"evaluation.started of the first":              1,
		"evaluation.completed of the first":            1,
		"evaluation.superseded of the first":           1,
		"evaluation.started of the second":             1,
		"evaluation.completed of the second":           1,
		"evaluation.feedback_recorded of the second":   1,
		"evaluation.suggestion_decided of the second":  1,
		"evaluation.suggestions_applied of the second": 1,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("outbox events:\n got  %v\n want %v", got, want)
	}
}

func TestTheMailboxRecordsOnlySuggestionsItsOwnEvaluationHolds(t *testing.T) {
	s := &Service{Pool: requireEvalDB(t)}
	m := seedRun(t, s.Pool)
	ctx := context.Background()

	first := beginAndComplete(t, s, m, aVerdict("first", OverallMet))
	second := beginAndComplete(t, s, m, aVerdict("second", OverallMet))
	earlier, err := s.queries().CreateEvaluationSuggestion(ctx, gen.CreateEvaluationSuggestionParams{
		WorkspaceID: m.run.WorkspaceID, EvaluationID: first.ID, Category: string(SuggestionSkill),
		Problem: "the steps are vague", Evidence: []byte(`[]`), TargetPath: "SKILL.md",
		ProposedContent: "clearer steps", ExpectedImpact: "fewer retries",
	})
	if err != nil {
		t.Fatalf("seed suggestion: %v", err)
	}
	if _, err := s.Decide(ctx, m.run.WorkspaceID, earlier.ID, DecisionAccepted); err != nil {
		t.Fatalf("decide: %v", err)
	}
	var version pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT skill_version_id FROM runs WHERE id = $1`, m.run.ID).Scan(&version); err != nil {
		t.Fatalf("read the run's version: %v", err)
	}
	nowhere := pgtype.UUID{Bytes: [16]byte{0xde, 0xad}, Valid: true}

	if err := s.RecordSuggestionsApplied(ctx, m.run.WorkspaceID, second.ID, version,
		[]pgtype.UUID{earlier.ID, nowhere}); err != nil {
		t.Fatalf("a letter naming suggestions the evaluation does not hold: %v", err)
	}

	var applied pgtype.UUID
	var recorded int
	if err := s.Pool.QueryRow(ctx, `
		SELECT (SELECT applied_skill_version_id FROM evaluation_suggestions WHERE id = $1),
		       (SELECT count(*) FROM outbox_events WHERE event_type = 'evaluation.suggestions_applied'
		        AND correlation_id = $2)`, earlier.ID, m.run.ID).Scan(&applied, &recorded); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if applied.Valid || recorded != 0 {
		t.Errorf("the second revision recorded the first one's suggestion: applied %v, %d events", applied, recorded)
	}
}
