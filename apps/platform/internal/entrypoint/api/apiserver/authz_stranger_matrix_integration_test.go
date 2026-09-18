package apiserver_test

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

type strangerCase struct {
	pattern string

	query string

	body string

	want int

	unprobed string
}

var strangerRoutes = []strangerCase{
	{pattern: "GET /api/skills/{id}", want: http.StatusNotFound},
	{pattern: "GET /api/skills/{id}/files", want: http.StatusNotFound},
	{pattern: "GET /skills/{id}/versions", want: http.StatusOK},
	{pattern: "GET /skills/{id}/diff", query: "?from={versionId}&to={versionId}", want: http.StatusNotFound},
	{pattern: "GET /skills/{id}/runs/preflight",
		query: "?version_id={versionId}&test_case_id={testCaseId}", want: http.StatusNotFound},
	{pattern: "GET /skills/{id}/versions/{versionId}/packaging/preview",
		query: "?target=standard", want: http.StatusNotFound},

	{pattern: "GET /test-cases/{id}", want: http.StatusNotFound},
	{pattern: "GET /test-cases/{id}/datasets", want: http.StatusNotFound},

	{pattern: "GET /runs/{id}", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/artifacts", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/trace", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/evaluation", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/evaluation/revisions", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/suggestions", want: http.StatusNotFound},
	{pattern: "GET /runs/{id}/comparison", query: "?against=" + anotherRunID, want: http.StatusNotFound},

	{pattern: "GET /downloads/{artifactId}", want: http.StatusNotFound},
	{pattern: "GET /downloads/{artifactId}/records", want: http.StatusNotFound},
	{pattern: "GET /downloads/{artifactId}/content", want: http.StatusNotFound},

	{pattern: "GET /admin/credits/{workspace_id}", want: http.StatusNotFound},

	{pattern: "POST /skills/{id}/fork", body: "{}", want: http.StatusNotFound},
	{pattern: "DELETE /skills/{id}", want: http.StatusNotFound},
	{pattern: "POST /skills/{id}/takedown", body: `{"reason":"a stranger asked"}`, want: http.StatusNotFound},
	{pattern: "PUT /skills/{id}/category", body: `{"category":"data"}`, want: http.StatusNotFound},

	{pattern: "PATCH /test-cases/{id}", body: `{"name":"renamed by a stranger"}`, want: http.StatusNotFound},
	{pattern: "DELETE /test-cases/{id}", want: http.StatusNotFound},
	{pattern: "POST /test-cases/{id}/criteria", body: `{"text":"added by a stranger"}`, want: http.StatusNotFound},
	{pattern: "PATCH /test-cases/{id}/criteria/{criterionId}",
		body: `{"text":"rewritten by a stranger"}`, want: http.StatusNotFound},
	{pattern: "DELETE /test-cases/{id}/criteria/{criterionId}", want: http.StatusNotFound},
	{pattern: "DELETE /test-cases/{id}/datasets/{datasetId}", want: http.StatusNotFound},

	{pattern: "POST /runs/{id}/cancel", want: http.StatusNotFound},
	{pattern: "DELETE /runs/{id}/artifacts/{artifactId}", want: http.StatusNoContent},
	{pattern: "DELETE /downloads/{artifactId}", want: http.StatusNoContent},
	{pattern: "POST /skills/{id}/versions/{versionId}/packaging",
		body: `{"target":"standard"}`, want: http.StatusNotFound},

	{pattern: "PUT /admin/skills/{id}/restriction",
		body: `{"reason":"a stranger asked"}`, want: http.StatusNotFound},
	{pattern: "DELETE /admin/skills/{id}/restriction", want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/redistribution",
		body: `{"redistribution":"allowed"}`, want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/tier", body: `{"tier":"curated"}`, want: http.StatusNotFound},
	{pattern: "PUT /admin/skills/{id}/takedown", body: `{"reason":"a stranger asked"}`, want: http.StatusNotFound},
	{pattern: "POST /admin/credits/{workspace_id}/grants",
		body: `{"credits":1000,"reason":"a stranger asked"}`, want: http.StatusNotFound},

	{pattern: "POST /skills/{id}/runs/preflight/confirm",
		unprobed: "the confirmation carries the owner's summary hash, which a stranger cannot compute; " +
			"the read it confirms is probed above"},
	{pattern: "POST /skills/{id}/runs",
		unprobed: "starting a run needs a confirmed summary hash; the preflight read it follows is probed above"},
	{pattern: "POST /skills/{id}/versions",
		unprobed: "a new version is a multipart package upload; this matrix sends JSON"},
	{pattern: "POST /test-cases/{id}/datasets",
		unprobed: "a dataset is a multipart file upload; this matrix sends JSON"},
	{pattern: "POST /test-cases/{id}/criteria/suggest",
		unprobed: "suggesting criteria calls the model service, which this matrix does not stand up"},
	{pattern: "PUT /runs/{id}/evaluation/feedback",
		unprobed: "feedback is keyed to an evaluation revision the stranger cannot name; " +
			"the evaluation reads it follows are probed above"},
	{pattern: "GET /suggestions/{id}/diff",
		unprobed: "no suggestion exists in this world: the judge stub returns a verdict without suggestions"},
	{pattern: "PUT /suggestions/{id}/decision", unprobed: "no suggestion exists in this world"},
	{pattern: "POST /skills/{id}/versions/from-suggestions", unprobed: "no suggestion exists in this world"},
	{pattern: "GET /creation-sessions/{session_id}",
		unprobed: "interactive creation is behind a deployment flag this world does not turn on"},
	{pattern: "GET /creation-sessions/{session_id}/events",
		unprobed: "interactive creation is behind a deployment flag this world does not turn on"},
	{pattern: "POST /creation-sessions/{session_id}/actions",
		unprobed: "interactive creation is behind a deployment flag this world does not turn on"},
	{pattern: "POST /internal/trace/{token}",
		unprobed: "the sandbox ingest path authenticates a run token, not a session; " +
			"the anonymous matrix covers it"},
}

type aliceWorld struct {
	skillID       string
	versionID     string
	testCaseID    string
	criterionID   string
	datasetID     string
	runID         string
	artifactID    string
	runArtifactID string
	workspaceID   string

	secrets map[string]string
}

func (w aliceWorld) resolve(path string) string {
	switch {
	case strings.HasPrefix(path, "/runs/"):
		path = strings.ReplaceAll(path, "{id}", w.runID)
		path = strings.ReplaceAll(path, "{artifactId}", w.runArtifactID)
	case strings.HasPrefix(path, "/test-cases/"):
		path = strings.ReplaceAll(path, "{id}", w.testCaseID)
	case strings.HasPrefix(path, "/downloads/"):
		path = strings.ReplaceAll(path, "{id}", w.artifactID)
	default:
		path = strings.ReplaceAll(path, "{id}", w.skillID)
	}
	replacements := []struct{ placeholder, value string }{
		{"{versionId}", w.versionID},
		{"{testCaseId}", w.testCaseID},
		{"{criterionId}", w.criterionID},
		{"{datasetId}", w.datasetID},
		{"{artifactId}", w.artifactID},
		{"{workspace_id}", w.workspaceID},
		{"{runId}", w.runID},
	}
	for _, r := range replacements {
		path = strings.ReplaceAll(path, r.placeholder, r.value)
	}
	return path
}

func (w aliceWorld) leaked(requested, body string) []string {
	var found []string
	for name, secret := range w.secrets {
		if secret == "" || strings.Contains(requested, secret) {
			continue
		}
		if strings.Contains(body, secret) {
			found = append(found, name)
		}
	}
	sort.Strings(found)
	return found
}

func callAs(t *testing.T, c *client, method, path, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(answer)
}

func TestEveryRouteThatNamesAResourceIsInTheStrangerMatrix(t *testing.T) {
	t.Parallel()
	declared := make(map[string]bool, len(strangerRoutes))
	for _, tc := range strangerRoutes {
		if declared[tc.pattern] {
			t.Errorf("the matrix declares %q twice", tc.pattern)
		}
		if (tc.unprobed == "") == (tc.want == 0) {
			t.Errorf("%q must either say what a stranger gets or why it is not probed, not both or neither",
				tc.pattern)
		}
		declared[tc.pattern] = true
	}

	for _, p := range mountedPatterns(t) {
		if !strings.Contains(p, "{") {
			continue
		}
		if !declared[p] {
			t.Errorf("route %q names a resource but has no entry in strangerRoutes; add one saying what "+
				"a logged-in stranger gets from another workspace's copy of it, or why it is not probed yet", p)
		}
		delete(declared, p)
	}
	for p := range declared {
		t.Errorf("strangerRoutes covers %q, which is no longer mounted; delete the entry or restore the route", p)
	}
}

const anotherRunID = "00000000-0000-0000-0000-0000000000ff"

const (
	aliceSkillName  = "matrix-private-widget"
	alicePromptText = "the ledger Alice never showed anyone"
)

func newAliceWorld(t *testing.T, a *api, pool *pgxpool.Pool) (aliceWorld, *client) {
	t.Helper()
	alice := a.login(t, "matrix-alice")
	skillID, versionID := importFiles(t, a, pool, alice, map[string]string{
		"SKILL.md": "---\nname: " + aliceSkillName + "\ndescription: Alice's own tool.\n---\n\nProse.\n",
		"LICENSE":  mitText,
	})
	allowRedistribution(t, pool, skillID)

	code, testCase := postJSON(t, alice, "/test-cases",
		`{"skill_id":"`+skillID+`","name":"alice's own case","user_prompt":"`+alicePromptText+`"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST test case: got %d, body %v", code, testCase)
	}
	testCaseID, _ := testCase["test_case_id"].(string)

	code, withCriterion := postJSON(t, alice, "/test-cases/"+testCaseID+"/criteria",
		`{"text":"the ledger comes back deduplicated"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST criterion: got %d, body %v", code, withCriterion)
	}
	criteria, _ := withCriterion["acceptance_criteria"].([]any)
	if len(criteria) != 1 {
		t.Fatalf("the test case has %d criteria: %v", len(criteria), withCriterion)
	}
	criterionID, _ := criteria[0].(map[string]any)["id"].(string)

	code, dataset := alice.upload(t, "/test-cases/"+testCaseID+"/datasets", "rows.csv", csvBytes(128))
	if code != http.StatusCreated {
		t.Fatalf("POST dataset: got %d, body %v", code, dataset)
	}
	datasetID, _ := dataset["dataset_id"].(string)

	judge := judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: criterionID, Result: "passed", Reason: "the reply says the duplicates were removed",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed the duplicate rows"},
				}},
		},
		Overall: "met", Summary: "the duplicates were removed",
	}, "judge-run@2026-08-18")
	evaluator := *a.evaluations
	evaluator.Judge = judge
	withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1}, &evaluator)

	f := fixture{client: alice, skillID: skillID, versionID: versionID, testCaseID: testCaseID}
	created := f.start(t)
	waitForStatus(t, alice, created.RunID, string(gen.RunStatusSucceeded))

	seedFinalOutput(t, pool, alice.workspaceID, created.RunID, "Removed the duplicate rows.")
	if err := evaluator.Evaluate(t.Context(),
		mustUUID(t, alice.workspaceID), mustUUID(t, created.RunID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	code, built := postJSON(t, alice, packagingPath(skillID, versionID), `{"target":"standard"}`)
	if code != http.StatusCreated {
		t.Fatalf("POST packaging: got %d, body %v", code, built)
	}
	artifactID, _ := built["artifact_id"].(string)
	runArtifactID := seedRunArtifact(t, a, pool, alice.workspaceID, created.RunID, "alice-private.txt")

	world := aliceWorld{
		skillID: skillID, versionID: versionID, testCaseID: testCaseID, criterionID: criterionID,
		datasetID: datasetID, runID: created.RunID, artifactID: artifactID, runArtifactID: runArtifactID,
		workspaceID: alice.workspaceID,
		secrets: map[string]string{
			"the version id":      versionID,
			"the test case id":    testCaseID,
			"the criterion id":    criterionID,
			"the dataset id":      datasetID,
			"the run id":          created.RunID,
			"the artifact id":     artifactID,
			"the run artifact id": runArtifactID,
			"the skill name":      aliceSkillName,
			"the private prompt":  alicePromptText,
		},
	}
	for name, secret := range world.secrets {
		if secret == "" {
			t.Fatalf("%s is empty: the matrix would be probing a resource that does not exist", name)
		}
	}
	return world, alice
}

func TestALoggedInStrangerGetsNothingFromAnotherWorkspacesResources(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	world, alice := newAliceWorld(t, a, pool)
	bob := a.login(t, "matrix-bob")

	probed := 0
	for _, tc := range strangerRoutes {
		if tc.unprobed != "" {
			continue
		}
		probed++
		method, pattern, _ := strings.Cut(tc.pattern, " ")
		target := world.resolve(pattern) + world.resolve(tc.query)
		t.Run(tc.pattern, func(t *testing.T) {
			status, body := callAs(t, bob, method, target, tc.body)
			if status == http.StatusBadRequest && tc.want != http.StatusBadRequest {
				t.Fatalf("answered 400 (%s); a request that never reaches the authorization decision "+
					"proves nothing about isolation, so this route is uncovered", strings.TrimSpace(body))
			}
			if status != tc.want {
				t.Errorf("a stranger got %d (%s), want %d", status, strings.TrimSpace(body), tc.want)
			}
			if leaked := world.leaked(target, body); len(leaked) > 0 {
				t.Errorf("the answer handed a stranger %v", leaked)
			}
		})
	}
	if probed < 35 {
		t.Fatalf("the matrix probed %d routes; it covered more than that when it was written, "+
			"so entries have been turned into exemptions rather than fixed", probed)
	}

	if got := alice.status(t, http.MethodGet, "/runs/"+world.runID); got != http.StatusOK {
		t.Errorf("after the stranger's sweep the owner gets %d for their own run, want 200", got)
	}
	if got := alice.status(t, http.MethodGet, "/test-cases/"+world.testCaseID); got != http.StatusOK {
		t.Errorf("after the stranger's sweep the owner gets %d for their own test case, want 200", got)
	}
	if resp, _ := alice.fetchContent(t, world.artifactID); resp.StatusCode != http.StatusOK {
		t.Errorf("after the stranger's sweep the owner gets %d for their own download, want 200", resp.StatusCode)
	}
	status, listed := callAs(t, alice, http.MethodGet, "/runs/"+world.runID+"/artifacts", "")
	if status != http.StatusOK || !strings.Contains(listed, world.runArtifactID) {
		t.Errorf("after the stranger's sweep the owner lists their run artifacts as %d (%s); "+
			"a stranger's delete answered 204 and it has to have deleted nothing", status, strings.TrimSpace(listed))
	}
}
