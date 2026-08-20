// Re-running and comparing (EVAL-011/012, 02:EVAL-003) through the real route
// table. Two things are under test here and they are deliberately in one file:
// that a re-run is the ordinary run path and nothing new, and that the comparison
// reads both runs without touching either.
//
// See authz_integration_test.go for the shared helpers (TestMain, migrate, login)
// and evaluation_integration_test.go for seedEvaluatableRun and judgeServer.
package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
)

// --- shapes ------------------------------------------------------------------

type comparisonBody struct {
	Runs []struct {
		RunID          string `json:"run_id"`
		SkillID        string `json:"skill_id"`
		SkillVersionID string `json:"skill_version_id"`
		TestCaseID     string `json:"test_case_id"`
		Status         string `json:"status"`
		Evaluation     *struct {
			EvaluationID string `json:"evaluation_id"`
			Status       string `json:"status"`
			Overall      string `json:"overall"`
			Cost         struct {
				EvaluationUSD *float64 `json:"evaluation_usd"`
				Source        string   `json:"source"`
			} `json:"cost"`
		} `json:"evaluation"`
		FinalOutput string `json:"final_output"`
		Errors      []struct {
			Category string `json:"category"`
			Code     string `json:"code"`
			Message  string `json:"message"`
		} `json:"errors"`
		DurationMS *int64 `json:"duration_ms"`
		Cost       struct {
			USD                 *float64 `json:"usd"`
			IsLowerBound        *bool    `json:"is_lower_bound"`
			AuthoritativeSource string   `json:"authoritative_source"`
		} `json:"cost"`
		InputsAvailable *bool `json:"inputs_available"`
	} `json:"runs"`
	CriterionMatrix []struct {
		CriterionID string `json:"criterion_id"`
		Text        string `json:"text"`
		Results     []struct {
			RunID  string  `json:"run_id"`
			Result *string `json:"result"`
			Source string  `json:"source"`
		} `json:"results"`
	} `json:"criterion_matrix"`
	VersionDiffURL string `json:"version_diff_url"`
	Error          string `json:"error"`
}

func (c *client) compare(t *testing.T, runID, against string) (int, comparisonBody) {
	t.Helper()
	resp, err := c.Get(c.base + "/runs/" + runID + "/comparison?against=" + against)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out comparisonBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// seedRunUsage puts a `usage` event on a run, which is where the run's own cost
// comes from — the figure the comparison is required to label a lower bound.
func seedRunUsage(t *testing.T, pool *pgxpool.Pool, workspaceID, runID string, costUSD float64) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"scope": "run_total", "model": "gpt-5.4-mini",
		"input_tokens": 1200, "output_tokens": 340,
		"cost_usd": costUSD, "cost_source": "gateway",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type,
		                          source, status, schema_version, masked, masked_fields, payload)
		VALUES (gen_random_uuid(), $1, $2, 1, 2, now(), 'usage', 'llm_service', 'ok',
		        '1.1', true, '[]'::jsonb, $3)`,
		mustUUID(t, workspaceID), mustUUID(t, runID), payload,
	); err != nil {
		t.Fatal(err)
	}
}

// cmpVersion is one more stored version of a skill. Distinct content hashes,
// because two runs of the *same* skill on *different* versions is the case the
// comparison exists for and a shared hash cannot express it.
func cmpVersion(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, tag string) string {
	t.Helper()
	return uuidText(seedVersion(t, pool, workspaceID, skillID, "sha256:cmp-"+tag).ID)
}

// judgeAll answers every criterion the same way, which is all these tests need
// from a judge: what is under test is the comparison, not the verdict.
func judgeAll(result, overall, quote string) llmclient.JudgeVerdict {
	return llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: result, Reason: "fixture", EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "agent_output", Quote: quote}}},
			{CriterionID: "c2", Result: result, Reason: "fixture", EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "agent_output", Quote: quote}}},
		},
		Overall: overall, Summary: "fixture verdict",
	}
}

// --- EVAL-012: two judged runs, side by side ---------------------------------

func TestComparisonShowsBothVerdictsCostsAndTheVersionDiffLink(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "cmp-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "cmp-skill")

	beforeVersion := cmpVersion(t, pool, c.workspaceID, skillID, "sides-1")
	afterVersion := cmpVersion(t, pool, c.workspaceID, skillID, "sides-2")
	before := seedRunForVersion(t, pool, c.workspaceID, skillID, beforeVersion)
	after := seedRunForVersion(t, pool, c.workspaceID, skillID, afterVersion)
	seedFinalOutput(t, pool, c.workspaceID, before, "I could not find the file.")
	seedFinalOutput(t, pool, c.workspaceID, after, "Removed 17 duplicate rows.")
	seedRunUsage(t, pool, c.workspaceID, before, 0.0134)
	seedRunUsage(t, pool, c.workspaceID, after, 0.0210)

	a.evaluations.Judge = judgeServer(t, judgeAll("failed", "not_met", "could not find"), "judge-run@v1")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, before)); err != nil {
		t.Fatal(err)
	}
	a.evaluations.Judge = judgeServer(t, judgeAll("passed", "met", "Removed 17"), "judge-run@v1")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, after)); err != nil {
		t.Fatal(err)
	}

	status, body := c.compare(t, before, after)
	if status != http.StatusOK {
		t.Fatalf("GET comparison: got %d (%s)", status, body.Error)
	}
	if len(body.Runs) != 2 {
		t.Fatalf("a comparison has two sides, got %d", len(body.Runs))
	}
	// The run named in the path is first; the `against` run is second.
	if body.Runs[0].RunID != before || body.Runs[1].RunID != after {
		t.Fatalf("sides are out of order: %s / %s", body.Runs[0].RunID, body.Runs[1].RunID)
	}
	if body.Runs[0].SkillVersionID != beforeVersion || body.Runs[1].SkillVersionID != afterVersion {
		t.Error("each side reports the version it actually ran")
	}
	// What a re-run would be started from, per side: the skill the version belongs
	// to and the editable test case the snapshot was frozen from. Each side seeded
	// its own test case, so equal ids here would mean one side is reporting the
	// other's inputs.
	for i, side := range body.Runs {
		if side.SkillID != skillID {
			t.Errorf("side %d skill_id = %q, want %q", i, side.SkillID, skillID)
		}
		if side.TestCaseID == "" {
			t.Errorf("side %d carries no test_case_id, so no re-run could be addressed", i)
		}
	}
	if body.Runs[0].TestCaseID == body.Runs[1].TestCaseID {
		t.Error("each side reports its own test case, not the other's")
	}

	// ADR-025: execution and judgement are separate fields on each side, and both
	// runs executed successfully while only one of them achieved anything.
	for i, want := range []string{"not_met", "met"} {
		side := body.Runs[i]
		if side.Status != "succeeded" {
			t.Errorf("side %d execution status = %q, want succeeded", i, side.Status)
		}
		if side.Evaluation == nil {
			t.Fatalf("side %d has no judgement", i)
		}
		if side.Evaluation.Overall != want || side.Evaluation.Status != "completed" {
			t.Errorf("side %d judgement = %+v, want overall %q", i, side.Evaluation, want)
		}
	}
	if body.Runs[0].FinalOutput == "" || body.Runs[0].FinalOutput == body.Runs[1].FinalOutput {
		t.Error("each side shows its own final output (02:EVAL-003 第 2 條)")
	}

	// 丙-3: the run's cost is present, labelled a lower bound, and names where the
	// settling figure lives. The evaluation's own cost is a separate field and is
	// never folded into it.
	for i, side := range body.Runs {
		if side.Cost.USD == nil {
			t.Errorf("side %d lost the run cost", i)
		}
		if side.Cost.IsLowerBound == nil || !*side.Cost.IsLowerBound {
			t.Errorf("side %d must state that the run cost is a lower bound, got %+v", i, side.Cost)
		}
		if side.Cost.AuthoritativeSource == "" {
			t.Errorf("side %d does not name the authoritative cost source (ADR-017)", i)
		}
		if side.Evaluation != nil && side.Evaluation.Cost.EvaluationUSD != nil &&
			side.Cost.USD != nil && *side.Evaluation.Cost.EvaluationUSD == *side.Cost.USD {
			t.Errorf("side %d merged the two costs into one number", i)
		}
	}
	if got := *body.Runs[0].Cost.USD; got != 0.0134 {
		t.Errorf("run cost = %v, want the trace total 0.0134", got)
	}

	// The matrix is per criterion, so a regression is visible without reading the
	// overall (02:EVAL-003 第 2 條).
	if len(body.CriterionMatrix) != 2 {
		t.Fatalf("one row per acceptance criterion, got %d", len(body.CriterionMatrix))
	}
	for _, row := range body.CriterionMatrix {
		if row.Text == "" {
			t.Errorf("criterion %s has no wording, so the row cannot be read", row.CriterionID)
		}
		if len(row.Results) != 2 {
			t.Fatalf("criterion %s: %d verdicts, want 2", row.CriterionID, len(row.Results))
		}
		if row.Results[0].Result == nil || *row.Results[0].Result != "failed" {
			t.Errorf("criterion %s left verdict = %v, want failed", row.CriterionID, row.Results[0].Result)
		}
		if row.Results[1].Result == nil || *row.Results[1].Result != "passed" {
			t.Errorf("criterion %s right verdict = %v, want passed", row.CriterionID, row.Results[1].Result)
		}
		// EVAL-001 clause 5 survives the trip into the comparison.
		if row.Results[0].Source != "model" {
			t.Errorf("criterion %s loses its judgement source in the matrix", row.CriterionID)
		}
	}

	// WS-003's diff, linked rather than reimplemented.
	if body.VersionDiffURL == "" {
		t.Error("two different versions of one skill have a diff to link (WS-003)")
	}

	// Both inputs are still there, so a re-run of these inputs is still possible.
	for i, side := range body.Runs {
		if side.InputsAvailable == nil || !*side.InputsAvailable {
			t.Errorf("side %d reports its inputs as gone while they are still stored", i)
		}
	}

	// 02:EVAL-003 第 3 條: comparing wrote nothing. Both runs, both evaluations and
	// both snapshots are exactly as they were.
	for _, id := range []string{before, after} {
		if _, run := c.getRun(t, id); run.Status != "succeeded" {
			t.Errorf("run %s changed to %q by being compared", id, run.Status)
		}
	}
}

// --- an unevaluated side says so ----------------------------------------------

func TestComparisonAgainstAnUnevaluatedRunNeverReadsAsAPass(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "cmp-unevaluated")
	skillID := seedSkill(t, pool, c.workspaceID, "cmp-unevaluated-skill")

	judged := seedRunForVersion(t, pool, c.workspaceID, skillID,
		cmpVersion(t, pool, c.workspaceID, skillID, "uneval-1"))
	never := seedRunForVersion(t, pool, c.workspaceID, skillID,
		cmpVersion(t, pool, c.workspaceID, skillID, "uneval-2"))
	seedFinalOutput(t, pool, c.workspaceID, judged, "done")

	a.evaluations.Judge = judgeServer(t, judgeAll("passed", "met", "done"), "judge-run@v1")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, judged)); err != nil {
		t.Fatal(err)
	}

	status, body := c.compare(t, judged, never)
	if status != http.StatusOK {
		t.Fatalf("GET comparison: got %d (%s)", status, body.Error)
	}
	if body.Runs[1].Evaluation != nil {
		t.Errorf("a run nobody evaluated has no judgement to show, got %+v", body.Runs[1].Evaluation)
	}
	if body.Runs[1].Status != "succeeded" {
		t.Errorf("the unevaluated side still reports how it executed, got %q", body.Runs[1].Status)
	}
	for _, row := range body.CriterionMatrix {
		if row.Results[1].Result != nil {
			t.Errorf("criterion %s: an unevaluated side must be null, not %q",
				row.CriterionID, *row.Results[1].Result)
		}
		if row.Results[0].Result == nil {
			t.Errorf("criterion %s lost the verdict of the side that was evaluated", row.CriterionID)
		}
	}
}

// --- scope and malformed requests ---------------------------------------------

func TestComparisonRefusesRunsThatAreNotTheCallersAndSaysNothingAboutThem(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "cmp-scope-owner")
	stranger := a.login(t, "cmp-scope-stranger")
	skillID := seedSkill(t, pool, owner.workspaceID, "cmp-scoped")
	mine := seedRunForVersion(t, pool, owner.workspaceID, skillID,
		cmpVersion(t, pool, owner.workspaceID, skillID, "scope-1"))
	other := seedRunForVersion(t, pool, owner.workspaceID, skillID,
		cmpVersion(t, pool, owner.workspaceID, skillID, "scope-2"))

	strangerSkill := seedSkill(t, pool, stranger.workspaceID, "cmp-theirs")
	theirs := seedRunForVersion(t, pool, stranger.workspaceID, strangerSkill,
		cmpVersion(t, pool, stranger.workspaceID, strangerSkill, "scope-3"))

	const absent = "11111111-1111-4111-8111-111111111111"
	cases := []struct {
		name, run, against string
		want               int
	}{
		{"missing against", mine, "", http.StatusBadRequest},
		{"malformed against", mine, "not-a-uuid", http.StatusBadRequest},
		{"itself", mine, mine, http.StatusBadRequest},
		{"against does not exist", mine, absent, http.StatusNotFound},
		{"path run does not exist", absent, mine, http.StatusNotFound},
		// Existence is private both ways round: neither position may be used to
		// probe for somebody else's run (WS-006, iron rule 3).
		{"against is another workspace's", mine, theirs, http.StatusNotFound},
		{"path run is another workspace's", theirs, mine, http.StatusNotFound},
	}
	for _, tc := range cases {
		if status, body := owner.compare(t, tc.run, tc.against); status != tc.want {
			t.Errorf("%s: got %d want %d (%s)", tc.name, status, tc.want, body.Error)
		}
	}
	// The control: the owner's own pair works, so the failures above are about
	// scope and not about the fixture.
	if status, body := owner.compare(t, mine, other); status != http.StatusOK {
		t.Fatalf("owner comparing their own runs: got %d (%s)", status, body.Error)
	}
	// And a stranger cannot read the owner's pair at all.
	if status, _ := stranger.compare(t, mine, other); status != http.StatusNotFound {
		t.Errorf("a stranger comparing somebody else's runs: got %d, want 404", status)
	}
}

// --- design §5.4: deleted inputs must not look re-runnable ---------------------

func TestComparisonReportsInputsThatCanNoLongerBeSupplied(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "cmp-inputs")
	skillID := seedSkill(t, pool, c.workspaceID, "cmp-inputs-skill")
	ctx := context.Background()

	withData, datasetID := seedRunWithDataset(t, pool, c.workspaceID, skillID,
		cmpVersion(t, pool, c.workspaceID, skillID, "inputs-1"))
	other := seedRunForVersion(t, pool, c.workspaceID, skillID,
		cmpVersion(t, pool, c.workspaceID, skillID, "inputs-2"))

	_, body := c.compare(t, withData, other)
	if body.Runs[0].InputsAvailable == nil || !*body.Runs[0].InputsAvailable {
		t.Fatalf("the dataset is still stored, so the inputs are available: %+v", body.Runs[0])
	}

	// The user deletes the file. The comparison stays readable — the snapshot and
	// the verdicts are history — but nothing may now suggest this can be re-run.
	if _, err := pool.Exec(ctx,
		`UPDATE datasets SET deleted_at = now() WHERE id = $1`, mustUUID(t, datasetID)); err != nil {
		t.Fatal(err)
	}
	status, body := c.compare(t, withData, other)
	if status != http.StatusOK {
		t.Fatalf("a deleted input must not break the comparison: got %d (%s)", status, body.Error)
	}
	if body.Runs[0].InputsAvailable == nil || *body.Runs[0].InputsAvailable {
		t.Error("a run whose dataset was deleted cannot be re-run on the same inputs (ADR-003)")
	}
	if body.Runs[1].InputsAvailable == nil || !*body.Runs[1].InputsAvailable {
		t.Error("the other side's inputs were untouched")
	}

	// An expired file is gone for the same purpose, and says so for the same
	// reason: the retention sweep removes the object, not just the row.
	if _, err := pool.Exec(ctx,
		`UPDATE datasets SET deleted_at = NULL, expires_at = now() - interval '1 day' WHERE id = $1`,
		mustUUID(t, datasetID)); err != nil {
		t.Fatal(err)
	}
	if _, body := c.compare(t, withData, other); body.Runs[0].InputsAvailable == nil ||
		*body.Runs[0].InputsAvailable {
		t.Error("an expired dataset is not a re-runnable input either")
	}

	// Deleting the test case itself removes the other half of what a re-run needs.
	if _, err := pool.Exec(ctx,
		`UPDATE datasets SET expires_at = now() + interval '30 days' WHERE id = $1`,
		mustUUID(t, datasetID)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE test_cases SET deleted_at = now()
		WHERE id = (SELECT test_case_id FROM test_case_snapshots
		            WHERE id = (SELECT test_case_snapshot_id FROM runs WHERE id = $1))`,
		mustUUID(t, withData)); err != nil {
		t.Fatal(err)
	}
	_, body = c.compare(t, withData, other)
	if body.Runs[0].InputsAvailable == nil || *body.Runs[0].InputsAvailable {
		t.Error("a deleted test case cannot be re-run either")
	}
	// The id is still served while the re-run is refused, and that is the point of
	// having both fields: `test_case_id` says what a re-run would address,
	// `inputs_available` says whether it may be offered. A screen that reads the id
	// as permission is the failure this pair exists to prevent.
	if body.Runs[0].TestCaseID == "" {
		t.Error("the snapshot still names the test case it was frozen from")
	}
}

// seedRunWithDataset is a finished run whose snapshot references one uploaded
// file, which is what makes "the inputs are gone" a state it can reach.
func seedRunWithDataset(
	t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, versionID string,
) (runID, datasetID string) {
	t.Helper()
	ctx := context.Background()
	testCaseID := seedTestCase(t, pool, workspaceID, skillID)

	if err := pool.QueryRow(ctx, `
		INSERT INTO datasets (workspace_id, test_case_id, file_name, content_type, size_bytes,
		                      content_hash, object_key, expires_at)
		VALUES ($1, $2, 'rows.csv', 'text/csv', 512, 'sha256:rows',
		        'datasets/rows.csv', now() + interval '30 days')
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID),
	).Scan(&datasetID); err != nil {
		t.Fatal(err)
	}

	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt,
		                                 acceptance_criteria, dataset_refs, content_hash)
		VALUES ($1, $2, 'summarise the attached csv',
		        '[{"id":"c1","text":"produces a summary","source":"user"}]'::jsonb,
		        jsonb_build_array(jsonb_build_object(
		            'dataset_id', $3::text, 'file_name', 'rows.csv',
		            'content_hash', 'sha256:rows', 'size_bytes', 512)),
		        'sha256:with-dataset')
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID), datasetID,
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}

	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, finished_at)
		VALUES ($1, $2, $3, 'fake_sandbox', '{}'::jsonb, '{}'::jsonb, 'succeeded', now())
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID),
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	return runID, datasetID
}

// --- EVAL-011: re-running is the ordinary run path -----------------------------

// There is no re-run endpoint, and this is what it would have had to be: the same
// POST /skills/{id}/runs with the new version_id and the same test_case_id. The
// permission screen stays on that path — which is the whole reason not to add a
// second one (TEST-009, design §5.4).
func TestRerunningTheSameTestCaseOnANewVersionGoesThroughPreflight(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "rerun-owner")

	first := f.start(t)
	staleHash := f.confirmPermissions(t)

	// The improvement was applied and a new version exists (EVAL-002 clause 4).
	v2 := seedVersion(t, pool, f.workspaceID, f.skillID, "hash-rerun-v2")
	a.packages[v2.PackageObjectKey] = cleanPackage(t)
	next := f
	next.versionID = uuidText(v2.ID)

	// The confirmation the user gave for the old package does not carry over: the
	// package contents changed, so what they agreed to is not this.
	if code, body := next.startWithHash(t, staleHash); code != http.StatusUnprocessableEntity {
		t.Fatalf("a stale confirmation must not start a run on a new version: got %d (%s)",
			code, body.Error)
	}

	second := next.start(t)
	if second.RunID == first.RunID {
		t.Fatal("a re-run is a new run")
	}

	// Same test case, unedited, so both snapshots hash the same — which is what
	// makes the two runs comparable at all (design §5.4).
	var firstHash, secondHash, firstSnapshot, secondSnapshot string
	for _, pair := range []struct {
		runID string
		hash  *string
		snap  *string
	}{{first.RunID, &firstHash, &firstSnapshot}, {second.RunID, &secondHash, &secondSnapshot}} {
		if err := pool.QueryRow(context.Background(), `
			SELECT s.content_hash, s.id::text FROM test_case_snapshots s
			JOIN runs r ON r.test_case_snapshot_id = s.id
			WHERE r.id = $1`, mustUUID(t, pair.runID)).Scan(pair.hash, pair.snap); err != nil {
			t.Fatal(err)
		}
	}
	if firstHash != secondHash {
		t.Errorf("the same test case must freeze to the same content hash: %q vs %q", firstHash, secondHash)
	}
	if firstSnapshot == secondSnapshot {
		t.Error("each run freezes its own snapshot; sharing one would let a later edit rewrite history")
	}

	// Iron rule 4: the first run and the version it ran are untouched by any of this.
	_, before := f.getRun(t, first.RunID)
	if before.Status != "queued" {
		t.Errorf("the earlier run changed to %q", before.Status)
	}

	status, body := f.compare(t, first.RunID, second.RunID)
	if status != http.StatusOK {
		t.Fatalf("comparing the two runs: got %d (%s)", status, body.Error)
	}
	if body.Runs[0].SkillVersionID != f.versionID || body.Runs[1].SkillVersionID != next.versionID {
		t.Error("the comparison shows which version each run used")
	}
	if body.VersionDiffURL == "" {
		t.Error("what changed between the two versions is the point of the comparison (WS-003)")
	}
	// Neither has been evaluated, and neither pretends otherwise.
	for i, side := range body.Runs {
		if side.Evaluation != nil {
			t.Errorf("side %d invented a judgement: %+v", i, side.Evaluation)
		}
	}
}
