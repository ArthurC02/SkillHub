// ADR-026 decision 2 and improvement/doc.go's invariant 9 both say `available` is
// answered at READ time, because a reference outlives the evidence it points at:
// a trace partition is dropped on retention, and WS-002 clause 3 lets a user
// delete a run output whenever they like. Two of the three read paths were not
// answering it — the suggestion list did not ask at all, and artifact references
// were stamped `true` when they were written and never revisited.
//
// Plus the two write-atomicity gaps in the same context: the judge's bill, and the
// provenance of a version built from suggestions.
package apiserver_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

// The suggestion list is the page a user reads BEFORE adopting a change, so a
// citation that no longer resolves is invisible at exactly the moment it matters.
// toSuggestionView used to unmarshal `evaluation_suggestions.evidence` and send it
// straight back out, which meant every ref kept the `available` it was written
// with — forever.
func TestSuggestionEvidenceIsReAnsweredAtReadTime(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "suggestion-availability")
	ev := evaluateWithSuggestions(t, a, pool, c, "suggestion-availability-skill",
		[]llmclient.ImprovementProposal{{
			Category: "skill", Problem: "the skill never says what to do about duplicates",
			Evidence:       "「" + suggestionQuote + "」",
			TargetPath:     "SKILL.md",
			ExpectedImpact: "the agent would deduplicate instead of stopping",
			ProposedContent: packagedSkillMD("suggestion-availability-skill") +
				"\nWhen rows repeat, keep the first and drop the rest.\n",
		}})

	status, suggestions, _ := c.listSuggestions(t, ev.runID)
	if status != 200 {
		t.Fatalf("GET suggestions: %d", status)
	}
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(suggestions))
	}
	cited := suggestions[0].Evidence
	if len(cited) == 0 {
		t.Fatal("the stored suggestion carries no evidence; there is nothing for this test to re-answer")
	}
	for _, e := range cited {
		if !e.Available {
			t.Fatalf("precondition: evidence %q is already unavailable", e.Excerpt)
		}
	}

	// The evidence goes away.
	//
	// Production removes these by dropping the month's partition (MaintainPartitions),
	// which no row trigger sees. A per-row DELETE is refused by 0005's immutability
	// trigger, and dropping the partition here would take every other test's events
	// with it — so this borrows one connection, turns off user triggers on it alone
	// (session_replication_role is per-session, not per-table), and removes the row.
	// The end state is the one a dropped partition leaves: a citation whose event no
	// longer resolves.
	dropTraceEvents(t, pool, ev.runID)

	_, suggestions, _ = c.listSuggestions(t, ev.runID)
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions after the trace went away, want 1 — the suggestion outlives its evidence", len(suggestions))
	}
	for _, e := range suggestions[0].Evidence {
		if e.Available {
			t.Errorf("evidence %q still claims to be available after the trace event was removed", e.Excerpt)
		}
		if e.Excerpt == "" {
			t.Error("a stale citation lost its excerpt; it must be labelled, not blanked (ADR-009)")
		}
	}
}

// The other half: an artifact reference was written `Available: true` by
// deterministic.go and never re-answered anywhere, so an evaluation report went on
// citing a file the user had deleted as though it were still there.
func TestArtifactEvidenceIsReAnsweredAtReadTime(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "artifact-availability")
	skillID := importPackage(t, pool, a.packages, c, "artifact-availability-skill", false)
	versionID, _, _, _ := latestVersionOf(t, pool, skillID)
	runID := seedRunForVersion(t, pool, c.workspaceID, skillID, versionID)
	seedFinalOutput(t, pool, c.workspaceID, runID, suggestionFinalOutput)
	artifactID := seedRunArtifact(t, a, pool, c.workspaceID, runID, "deduplicated.csv")

	a.evaluations.Judge = judgeServer(t, failedBoth, "judge-run@test")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	// evaluationBody's DeterministicFindings carries no evidence, and the point
	// here is the evidence, so this reads the report with its own shape.
	available := func() (found, live bool) {
		t.Helper()
		resp, err := c.Get(c.base + "/runs/" + runID + "/evaluation")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET evaluation: %d", resp.StatusCode)
		}
		var body struct {
			DeterministicFindings []struct {
				Evidence []struct {
					Kind      string `json:"kind"`
					Excerpt   string `json:"excerpt"`
					Available bool   `json:"available"`
				} `json:"evidence"`
			} `json:"deterministic_findings"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, f := range body.DeterministicFindings {
			for _, e := range f.Evidence {
				if e.Kind == "artifact" && strings.Contains(e.Excerpt, "deduplicated.csv") {
					return true, e.Available
				}
			}
		}
		return false, false
	}

	found, live := available()
	if !found {
		t.Fatal("the evaluation cites no artifact evidence; there is nothing for this test to re-answer")
	}
	if !live {
		t.Fatal("precondition: the artifact was reported unavailable while it still exists")
	}

	// WS-002 clause 3: the owner deletes one output. The evaluation is immutable
	// and keeps its reference; what must change is the answer to "can I follow it".
	if _, err := pool.Exec(context.Background(),
		`UPDATE artifacts SET deleted_at = now() WHERE id = $1`, mustUUID(t, artifactID)); err != nil {
		t.Fatal(err)
	}

	found, live = available()
	if !found {
		t.Fatal("the artifact citation disappeared from the report; a stale reference is labelled, not removed")
	}
	if live {
		t.Error("the artifact citation still claims to be available after the file was deleted")
	}
}

// dropTraceEvents removes a run's trace events the way retention effectively does.
// See its caller for why it goes around the immutability trigger rather than
// through it.
func dropTraceEvents(t *testing.T, pool *pgxpool.Pool, runID string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SET session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	// Restored on the same connection before it goes back to the pool, or every
	// later test borrowing it would run without triggers.
	defer func() {
		if _, err := conn.Exec(ctx, `SET session_replication_role = DEFAULT`); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := conn.Exec(ctx, `DELETE FROM trace_events WHERE run_id = $1`, mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
}

// `evaluation_model_usage` is an `immutable:` table in db/query-owners.yaml — the
// model usage as of the judgement. It was written on the pool from judge(), before
// complete() opened the transaction that records the verdict, so the two could
// come apart in both directions: a bill with no completed judgement to explain it,
// or a judgement recorded as failed with the money already spent and nothing
// recording it. ON CONFLICT DO NOTHING answers re-delivery, not this.
//
// The forced failure is a verdict write that raises, so complete()'s transaction
// rolls back in full. Anything written inside it goes with it; anything written
// before it does not, which is the difference being tested.
func TestAJudgementThatDoesNotCommitLeavesNoBillBehind(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "usage-atomicity")
	skillID := seedSkill(t, pool, c.workspaceID, "usage-atomicity-skill")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	ctx := context.Background()

	// The judge answers, and reports what it cost.
	a.evaluations.Judge = judgeServer(t, failedBoth, "judge-run@test")

	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION a4_break_verdict() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'a4: verdict write refused'; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER a4_break_verdict BEFORE UPDATE ON evaluations
		FOR EACH ROW WHEN (NEW.status = 'completed') EXECUTE FUNCTION a4_break_verdict();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DROP TRIGGER IF EXISTS a4_break_verdict ON evaluations;
			 DROP FUNCTION IF EXISTS a4_break_verdict();`)
	})

	if err := a.evaluations.Evaluate(ctx, mustUUID(t, c.workspaceID), mustUUID(t, runID)); err == nil {
		t.Fatal("the evaluation reported success while its verdict write raised")
	}

	var usage, completed int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM evaluation_model_usage u
		        JOIN evaluations e ON e.id = u.evaluation_id
		        WHERE e.run_id = $1 AND u.operation = 'judge'),
		       (SELECT count(*) FROM evaluations WHERE run_id = $1 AND status = 'completed')`,
		mustUUID(t, runID),
	).Scan(&usage, &completed); err != nil {
		t.Fatal(err)
	}
	if completed != 0 {
		t.Fatalf("precondition: %d completed judgement(s) after the verdict write raised", completed)
	}
	if usage != 0 {
		t.Errorf("%d judge usage row(s) survived a judgement that never committed: "+
			"a bill in the database that no verdict explains", usage)
	}
}

// EVAL-002 clause 4: an applied suggestion points at the version it produced, and
// `packaging` reads exactly that link to write a download package's provenance. The
// write is outside SaveVersion's transaction, so it can fail with the version
// already committed — and this is a synchronous HTTP path, so nothing retries it.
//
// The gap cannot be closed without an ingest transaction variant (A5/main's call).
// What it must not do meanwhile is fail silently: the caller is told both halves,
// and the audit trail gets a durable row, because user content losing a property
// without the user asking is the thing the trail is for.
func TestALostProvenanceWriteIsAnnouncedAndAudited(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "provenance-loss")
	ev := evaluateWithSuggestions(t, a, pool, c, "provenance-loss-skill",
		[]llmclient.ImprovementProposal{{
			Category: "skill", Problem: "the skill never says what to do about duplicates",
			Evidence:       "「" + suggestionQuote + "」",
			TargetPath:     "SKILL.md",
			ExpectedImpact: "the agent would deduplicate instead of stopping",
			ProposedContent: packagedSkillMD("provenance-loss-skill") +
				"\nWhen rows repeat, keep the first and drop the rest.\n",
		}})

	_, suggestions, evaluationID := c.listSuggestions(t, ev.runID)
	if len(suggestions) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(suggestions))
	}
	ctx := context.Background()

	// A suggestion is only built into a version once its owner accepted it
	// (EVAL-002 clause 3).
	if code, _ := c.decide(t, suggestions[0].SuggestionID, "accepted"); code != 200 {
		t.Fatalf("accepting the suggestion got %d", code)
	}

	// Make the provenance write fail while leaving the version write alone: a
	// trigger on the one UPDATE MarkSuggestionsApplied performs.
	if _, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION a4_break_provenance() RETURNS trigger AS $$
		BEGIN RAISE EXCEPTION 'a4: provenance write refused'; END $$ LANGUAGE plpgsql;
		CREATE TRIGGER a4_break_provenance BEFORE UPDATE OF applied_skill_version_id
		ON evaluation_suggestions FOR EACH ROW EXECUTE FUNCTION a4_break_provenance();`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DROP TRIGGER IF EXISTS a4_break_provenance ON evaluation_suggestions;
			 DROP FUNCTION IF EXISTS a4_break_provenance();`)
	})

	status, body := c.applySuggestions(t, ev.skillID, evaluationID, suggestions[0].SuggestionID)
	if status < 500 {
		t.Fatalf("apply returned %d; a provenance write that failed must not be reported as success", status)
	}
	if !strings.Contains(body.Error, "version") || !strings.Contains(body.Error, "provenance") {
		t.Errorf("error = %q; the caller has to be told the version exists AND its provenance is missing", body.Error)
	}

	var audited int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_events WHERE action = 'evaluation.provenance_not_recorded'`,
	).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("%d audit rows for the lost provenance, want 1: a silent loss is the whole defect", audited)
	}
}
