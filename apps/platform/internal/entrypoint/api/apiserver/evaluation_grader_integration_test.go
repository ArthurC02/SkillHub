package apiserver_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
)

func createGrader(t *testing.T, c *client, path, name, instructions string) map[string]any {
	t.Helper()
	status, body := c.doJSON(t, http.MethodPost, path,
		fmt.Sprintf(`{"name":%q,"instructions":%q,"reason":"first version"}`, name, instructions))
	if status != http.StatusCreated {
		t.Fatalf("create grader %s: got %d %v", name, status, body)
	}
	return body
}

func listedGraders(t *testing.T, c *client, path string) map[string]map[string]any {
	t.Helper()
	status, body := c.doJSON(t, http.MethodGet, path, "")
	if status != http.StatusOK {
		t.Fatalf("list graders: got %d %v", status, body)
	}
	byID := map[string]map[string]any{}
	graders, _ := body["graders"].([]any)
	for _, g := range graders {
		grader, _ := g.(map[string]any)
		id, _ := grader["id"].(string)
		byID[id] = grader
	}
	return byID
}

func TestAWorkspaceGraderIsVersionedSeenOnlyByItsWorkspaceAndAudited(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice, bob := a.login(t, freshName("grader-alice")), a.login(t, freshName("grader-bob"))

	first := createGrader(t, alice, "/evaluation/graders", "語氣", "fail when the reply sounds curt")
	id, _ := first["id"].(string)
	if first["scope"] != "workspace" || first["version"] != float64(1) {
		t.Fatalf("created = %v, want this workspace's version 1", first)
	}
	status, second := alice.doJSON(t, http.MethodPost, "/evaluation/graders/"+id+"/versions",
		`{"instructions":"fail when the reply sounds curt or cold","reason":"cold replies slipped through"}`)
	if status != http.StatusCreated || second["version"] != float64(2) || second["version_id"] == first["version_id"] {
		t.Fatalf("revised = %d %v, want version 2 under a new version id", status, second)
	}

	if listed := listedGraders(t, alice, "/evaluation/graders")[id]; listed["version"] != float64(2) ||
		listed["instructions"] != "fail when the reply sounds curt or cold" {
		t.Errorf("alice lists %v, want the latest version", listed)
	}
	if _, seen := listedGraders(t, bob, "/evaluation/graders")[id]; seen {
		t.Error("another workspace sees alice's grader")
	}

	refusals := []struct {
		name string
		c    *client
		path string
		body string
		want int
	}{
		{"another workspace revising it", bob, "/evaluation/graders/" + id + "/versions", `{"instructions":"x","reason":"y"}`, http.StatusNotFound},
		{"a version with no reason", alice, "/evaluation/graders/" + id + "/versions", `{"instructions":"x","reason":"  "}`, http.StatusBadRequest},
		{"instructions one past the limit", alice, "/evaluation/graders/" + id + "/versions",
			fmt.Sprintf(`{"instructions":%q,"reason":"y"}`, strings.Repeat("字", 4001)), http.StatusBadRequest},
		{"a second grader with the same name", alice, "/evaluation/graders", `{"name":"語氣","instructions":"x","reason":"y"}`, http.StatusConflict},
	}
	for _, c := range refusals {
		if status, body := c.c.doJSON(t, http.MethodPost, c.path, c.body); status != c.want {
			t.Errorf("%s: got %d %v, want %d", c.name, status, body, c.want)
		}
	}
	if status, _ := alice.doJSON(t, http.MethodPost, "/evaluation/graders/"+id+"/versions",
		fmt.Sprintf(`{"instructions":%q,"reason":"at the limit"}`, strings.Repeat("字", 4000))); status != http.StatusCreated {
		t.Errorf("instructions at the limit: got %d, want 201", status)
	}

	var audited int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_events
		WHERE resource_id = $1 AND action = 'evaluation_grader.version_create' AND workspace_id IS NOT NULL`,
		mustUUID(t, id)).Scan(&audited); err != nil || audited != 3 {
		t.Errorf("audit events = %d (%v), want one per version", audited, err)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE evaluation_grader_versions SET instructions = 'rewritten' WHERE grader_id = $1`, mustUUID(t, id)); err == nil {
		t.Error("a saved grader version was rewritten in place")
	}
}

func TestAPlatformGraderIsDefinedByOperatorsAndUsableByEveryWorkspace(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator, member := a.login(t, freshName("grader-operator")), a.login(t, freshName("grader-member"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	name := freshName("禮貌")

	if status, _ := member.doJSON(t, http.MethodPost, "/admin/evaluation/graders",
		fmt.Sprintf(`{"name":%q,"instructions":"x","reason":"y"}`, name)); status != http.StatusNotFound {
		t.Errorf("a member defining a platform grader: got %d, want 404", status)
	}
	created := createGrader(t, operator, "/admin/evaluation/graders", name, "fail when the reply is rude")
	id, _ := created["id"].(string)
	if created["scope"] != "platform" {
		t.Fatalf("created = %v, want a platform grader", created)
	}
	if listed := listedGraders(t, member, "/evaluation/graders")[id]; listed["scope"] != "platform" {
		t.Errorf("the member lists %v, want the platform grader", listed)
	}
	if _, listed := listedGraders(t, operator, "/admin/evaluation/graders")[id]; !listed {
		t.Error("the admin list leaves out the platform grader")
	}
	if status, _ := member.doJSON(t, http.MethodPost, "/evaluation/graders/"+id+"/versions",
		`{"instructions":"x","reason":"y"}`); status != http.StatusNotFound {
		t.Errorf("a member revising a platform grader through the workspace route: got %d, want 404", status)
	}
	if status, body := operator.doJSON(t, http.MethodPost, "/admin/evaluation/graders/"+id+"/versions",
		`{"instructions":"fail when the reply is rude or dismissive","reason":"dismissive replies counted"}`); status != http.StatusCreated ||
		body["version"] != float64(2) {
		t.Errorf("operator revision: got %d %v, want version 2", status, body)
	}
}

func checkNaming(versionID string) string {
	return fmt.Sprintf(`{"text":"reads well","check":{"kind":"prompt_grader","values":[%q]}}`, versionID)
}

func TestACheckMayNameOnlyAGraderVersionItsWorkspaceCanSee(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	operator := a.login(t, freshName("grader-check-operator"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	alice, bob := a.login(t, freshName("grader-check-alice")), a.login(t, freshName("grader-check-bob"))
	alicesVersion, _ := createGrader(t, alice, "/evaluation/graders", "語氣", "fail when curt")["version_id"].(string)
	platformVersion, _ := createGrader(t, operator, "/admin/evaluation/graders", freshName("通用"), "fail when rude")["version_id"].(string)
	bobsCase := seedTestCase(t, pool, bob.workspaceID, seedSkill(t, pool, bob.workspaceID, "grader-check"))

	cases := []struct {
		name    string
		version string
		want    int
	}{
		{"another workspace's grader version", alicesVersion, http.StatusBadRequest},
		{"a version id that does not exist", "00000000-0000-4000-8000-000000000000", http.StatusBadRequest},
		{"a value that is not a version id", "語氣", http.StatusBadRequest},
		{"a platform grader version", platformVersion, http.StatusCreated},
	}
	for _, c := range cases {
		if status, body := bob.doJSON(t, http.MethodPost, "/test-cases/"+bobsCase+"/criteria", checkNaming(c.version)); status != c.want {
			t.Errorf("%s: got %d %v, want %d", c.name, status, body, c.want)
		}
	}
}

func TestAPromptGraderBehindAPassingGateIsJudgedWithItsOwnInstructions(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, freshName("eval-grader-owner"))
	versionID, _ := createGrader(t, c, "/evaluation/graders", "語氣", "fail when the reply sounds curt")["version_id"].(string)
	skillID := seedSkill(t, pool, c.workspaceID, "graded")
	runID, _ := seedEvaluatableRunWithCriteria(t, pool, c.workspaceID, skillID, fmt.Sprintf(`[
		{"id":"c1","text":"reports 17 rows removed in a friendly tone","source":"user",
		 "check":{"kind":"sequence","children":[{"kind":"contains_values","values":["17"]},
		          {"kind":"prompt_grader","values":[%q]}]}}]`, versionID), nil)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")
	judge, sent := countingJudge(t, llmclient.CriterionVerdict{CriterionID: "c1@0.1", Result: "failed", Reason: "the reply is curt",
		EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")}}})
	a.evaluations.Judge = judge

	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if len(*sent) != 1 || len((*sent)[0].Criteria) != 1 {
		t.Fatalf("judge requests = %+v, want one asking about one item", *sent)
	}
	if asked := (*sent)[0].Criteria[0]; asked.ID != "c1@0.1" || asked.Instructions != "fail when the reply sounds curt" {
		t.Errorf("asked %+v, want the grader leaf with its instructions", asked)
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	r := body.CriterionResults[0]
	if r.Result != "failed" || r.Source != "model" || len(r.Steps) != 3 || !strings.HasPrefix(r.Steps[1].Kind, "prompt_grader") {
		t.Errorf("c1 = %+v, want the grader's failure labelled model with every step", r)
	}
}
