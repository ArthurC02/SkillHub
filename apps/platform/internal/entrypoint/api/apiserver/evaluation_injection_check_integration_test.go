package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/credit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/judging"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/llmadapter"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/modelport"
)

func screenerServer(t *testing.T, status int, answer llmclient.CheckInjectionResponse) modelport.Screener {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/check-injection" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(srv.Close)
	return llmadapter.ScreenerOrNone(&llmclient.Client{BaseURL: srv.URL})
}

type screeningCase struct {
	name        string
	status      int
	probability float64
	on          bool
	wantShown   string
	wantStored  bool
}

const screenedTokens = 1820

func TestAJudgedEvaluationRecordsTheInjectionCheckBesideItsVerdict(t *testing.T) {
	for _, tc := range []screeningCase{
		{"flagged", http.StatusOK, 0.9, true, "flagged", true},
		{"clear", http.StatusOK, 0.1, true, "clear", true},
		{"the check does not answer", http.StatusServiceUnavailable, 0, true, "unchecked", false},
		{"switched off", http.StatusOK, 0.9, false, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := requireDB(t)
			c, runID := evaluateWithScreening(t, pool, tc)
			expectShownCheck(t, c, runID, tc)
			expectStoredCheck(t, pool, runID, tc)
			expectScreeningUsageAndCost(t, pool, runID, tc)
		})
	}
}

func evaluateWithScreening(t *testing.T, pool *pgxpool.Pool, tc screeningCase) (*client, string) {
	t.Helper()
	a := newAPI(t, pool)
	c := a.login(t, freshName("eval-screening"))
	skillID := seedSkill(t, pool, c.workspaceID, "dedupe")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows and saved the result to output.xlsx.")
	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "the reply says so",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "agent_output", Quote: "Removed 17 duplicate rows"}}},
			{CriterionID: "c2", Result: "passed", Reason: "output.xlsx is in the manifest",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")}}},
		},
		Overall: "met", Summary: "both conditions were met",
	}, "judge-run@2026-08-17")
	a.evaluations.Credit = &credit.Service{Store: credit.NewPostgresStore(pool)}
	a.evaluations.Judging.Screener = screenerServer(t, tc.status,
		llmclient.CheckInjectionResponse{Probability: tc.probability, Model: "gpt-6-luna", InputTokens: screenedTokens})
	a.evaluations.Judging.ScreeningPolicy = func(context.Context) (judging.ScreeningPolicy, error) {
		return judging.ScreeningPolicy{On: tc.on, USDPerMillionTokens: 0.25}, nil
	}
	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return c, runID
}

func expectShownCheck(t *testing.T, c *client, runID string, tc screeningCase) {
	t.Helper()
	status, raw := c.doJSON(t, http.MethodGet, "/runs/"+runID+"/evaluation", "")
	if status != http.StatusOK || raw["status"] != "completed" || raw["overall"] != "met" {
		t.Fatalf("GET evaluation: %d %v, want a completed, met verdict whatever the check said", status, raw)
	}
	check, _ := raw["injection_check"].(map[string]any)
	switch {
	case tc.wantShown == "" && check != nil:
		t.Errorf("injection_check = %v, want it absent while the check is off", check)
	case tc.wantShown != "" && (check == nil || check["status"] != tc.wantShown):
		t.Errorf("injection_check = %v, want status %q", check, tc.wantShown)
	}
}

func expectStoredCheck(t *testing.T, pool *pgxpool.Pool, runID string, tc screeningCase) {
	t.Helper()
	var stored *string
	var probability *float64
	if err := pool.QueryRow(context.Background(),
		`SELECT injection_check, injection_probability FROM evaluations WHERE run_id = $1 AND superseded_at IS NULL`,
		mustUUID(t, runID)).Scan(&stored, &probability); err != nil {
		t.Fatal(err)
	}
	if (tc.wantShown == "") != (stored == nil) || (stored != nil && *stored != tc.wantShown) {
		t.Errorf("stored injection_check = %v, want %q", stored, tc.wantShown)
	}
	if tc.wantStored != (probability != nil) || (probability != nil && *probability != tc.probability) {
		t.Errorf("stored probability = %v, want %v present=%v", probability, tc.probability, tc.wantStored)
	}
}

func expectScreeningUsageAndCost(t *testing.T, pool *pgxpool.Pool, runID string, tc screeningCase) {
	t.Helper()
	usageRows := countRow(t, pool, `SELECT count(*) FROM evaluation_model_usage u JOIN evaluations e ON e.id = u.evaluation_id
		WHERE e.run_id = $1 AND u.operation = 'injection_check' AND u.prompt_tokens = $2 AND u.model = 'gpt-6-luna'`,
		mustUUID(t, runID), screenedTokens)
	costRows := countRow(t, pool, `SELECT count(*) FROM cost_events
		WHERE ref_id = $1 AND kind = 'injection_check' AND cost_source = 'estimated' AND usd_micros = 455 AND prompt_tokens = $2`,
		mustUUID(t, runID), screenedTokens)
	want := 0
	if tc.wantStored {
		want = 1
	}
	if usageRows != want || costRows != want {
		t.Errorf("usage rows %d, estimated cost rows at 455 micros %d; want %d of each", usageRows, costRows, want)
	}
}
