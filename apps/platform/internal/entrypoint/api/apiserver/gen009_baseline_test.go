package apiserver_test

// GEN-009 ③ — the verdict distribution for generated skills.
//
// ============================ WHAT THIS IS ==================================
// One loop, no fake on any leg: a task description goes to the real generation
// path, the package it produces is put in the real object store, a test case is
// created with the SAME three acceptance criteria the 45 curated skills were
// measured against (tools/content/seed_testcases.py, BASELINE_CRITERIA), the run
// executes in a real sandbox through the real gateway, and the real judge
// returns a verdict. Every row lands in a JSON file for the report.
//
// The yardstick is deliberately the curated one and not a better one. Criteria
// written per description would measure "did it do the task", which is a
// stronger question — and a judgement somebody has to make. The owner chose the
// curated three on 2026-08-28, so these numbers sit beside CONTENT-008's on the
// same axis. What that buys is comparability; what it costs is stated in the
// report and here: this measures that a generated skill LOADS and PRODUCES
// something, not that it did what was asked.
//
// ============================ WHAT THIS IS NOT ==============================
// Not GEN-009 ④ — "would a person keep it" needs a person, and nothing here
// substitutes for that.
//
// Not a SEC-009 item: the runtime is runc.
//
// Usage:
//
//	GEN009_CORPUS=<file.json>   [{"id","group","description"}, ...]
//	GEN009_OUT=<file.json>      where the rows are written
//	SKILLHUB_E2E_LLM_URL        a running apps/llm pointed at a real gateway
//	SKILLHUB_E2E_SANDBOX_URL / _TOKEN
//	SKILLHUB_MODEL_GATEWAY_URL / _KEY
//	OBJSTORE_*                  the real object store
//	SKILLHUB_E2E_PUBLIC_HOST    an address the sandbox host can reach us on
//
// It spends money: about $0.006 to generate, $0.017 to run and a judge call per
// description.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// baselineCriteria is seed_testcases.py's BASELINE_CRITERIA, verbatim. Copied
// rather than imported because the Python tool is the thing being matched, and a
// paraphrase here would quietly change the yardstick the numbers claim to share.
var baselineCriteria = []string{
	"trace 中出現對指定 Skill 的 skill_activation 事件。",
	"/out/artifacts/ 至少產出一個檔案。",
	"最終回覆說明了這次產出哪些檔案。",
}

type gen009Case struct {
	ID          string `json:"id"`
	Group       string `json:"group"`
	Description string `json:"description"`
}

type gen009Row struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	// Generation
	Generated bool     `json:"generated"`
	Blocked   bool     `json:"blocked"`
	Findings  []string `json:"findings,omitempty"`
	SkillName string   `json:"skill_name,omitempty"`
	Attempts  int      `json:"attempts,omitempty"`
	// Run
	RunStatus    string `json:"run_status,omitempty"`
	FailureClass string `json:"failure_class,omitempty"`
	// Evaluation
	EvalStatus string            `json:"eval_status,omitempty"`
	Overall    string            `json:"overall,omitempty"`
	Criteria   map[string]string `json:"criteria,omitempty"`
	Note       string            `json:"note,omitempty"`
}

func TestGeneratedSkillsRunAndAreJudged(t *testing.T) {
	corpusPath := os.Getenv("GEN009_CORPUS")
	if corpusPath == "" {
		t.Skip("set GEN009_CORPUS; this test generates, runs and judges — it spends money")
	}
	llmURL := os.Getenv("SKILLHUB_E2E_LLM_URL")
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if llmURL == "" || sandboxURL == "" {
		t.Fatal("SKILLHUB_E2E_LLM_URL and SKILLHUB_E2E_SANDBOX_URL are both required")
	}
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus []gen009Case
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus) == 0 {
		t.Fatal("the corpus is empty; a census over nothing is a zero, not a pass")
	}

	pool := requireDB(t)
	ctx := context.Background()

	store, err := objstore.New(
		os.Getenv("OBJSTORE_ENDPOINT"), os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"), objstoreBucket(), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}

	a := newAPIWithLLM(t, pool, llmURL)
	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = run.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required")
	}
	a.runs.PollInterval = time.Second
	a.runs.MaxAttempts = 1

	// The sandbox host pushes trace back, and httptest binds loopback. Same
	// second listener the e2e test uses, for the same reason.
	public := httptest.NewUnstartedServer(a.handler)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	public.Listener = listener
	public.Start()
	defer public.Close()
	a.runs.TraceSigner = a.traceSigner
	a.runs.TraceIngestBaseURL = fmt.Sprintf("http://%s:%d",
		os.Getenv("SKILLHUB_E2E_PUBLIC_HOST"), listener.Addr().(*net.TCPAddr).Port)

	// The judge is the worker's, not the API's. cmd/worker sets
	// Evaluations.Judge = deps.LLM; the API's own evaluation service reaches no
	// judge on purpose (a read must never pay for a model call), so the field is
	// set on a copy rather than on a.evaluations.
	judging := *a.evaluations
	judging.Judge = &llmclient.Client{BaseURL: llmURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	startWorkerWith(t, a.runs, &judging)

	c := a.login(t, "gen009-baseline")
	ws := workspaceOf(t, pool, c)

	rows := make([]gen009Row, 0, len(corpus))
	for i, tc := range corpus {
		row := gen009Row{ID: tc.ID, Group: tc.Group}
		t.Run(tc.ID, func(t *testing.T) {
			res, err := a.versions.GenerateSkill(ctx, ws, tc.Description)
			if err != nil {
				row.Note = "generate: " + err.Error()
				t.Logf("%s generate failed: %v", tc.ID, err)
				return
			}
			row.Attempts = res.Attempts
			if res.Report.Blocked {
				row.Blocked = true
				for _, f := range res.Report.Findings {
					row.Findings = append(row.Findings, f.Code)
				}
				t.Logf("%s blocked: %v", tc.ID, row.Findings)
				return
			}
			row.Generated = true
			row.SkillName = res.Skill.Name

			// The bytes the sandbox will execute have to be in the real store:
			// the API's own package store is in-memory, and a pre-signed URL
			// over a key that only exists in this process points at nothing.
			key := res.Version.PackageObjectKey
			pkg, ok := a.packages[key]
			if !ok {
				row.Note = "the generated package is not in the API's store under " + key
				return
			}
			if err := store.Put(ctx, key, pkg); err != nil {
				row.Note = "put package: " + err.Error()
				return
			}

			skillID := uuidText(res.Skill.ID)
			f := fixture{
				client:    c,
				skillID:   skillID,
				versionID: uuidText(res.Version.ID),
				testCaseID: seedGen009TestCase(t, pool, c.workspaceID, skillID,
					res.Skill.Name, tc.Description),
			}

			code, view := f.startNoFatal(t)
			if code != http.StatusCreated && code != http.StatusOK {
				row.Note = fmt.Sprintf("POST run: %d %s", code, view.Error)
				return
			}
			final := waitForTerminalSoft(t, f.client, view.RunID, 8*time.Minute)
			row.RunStatus = final.Status
			row.FailureClass = final.FailureClass.Value

			ev := waitForEvaluation(t, f.client, view.RunID, 4*time.Minute)
			row.EvalStatus = ev.Status
			row.Overall = ev.Overall
			row.Criteria = map[string]string{}
			for _, r := range ev.CriterionResults {
				row.Criteria[r.Text] = r.Result
			}
			t.Logf("%s: run=%s eval=%s overall=%s", tc.ID, row.RunStatus, row.EvalStatus, row.Overall)
		})
		rows = append(rows, row)
		writeGen009(t, rows)
		t.Logf("--- %d/%d done", i+1, len(corpus))
	}
	writeGen009(t, rows)
}

// seedGen009TestCase is seedTestCase with the curated yardstick and a prompt
// that names the skill: PDM-011 measured the autonomous trigger rate at 0, so a
// benchmark prompt that does not name the skill measures the model's guessing.
func seedGen009TestCase(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID, skillName, task string) string {
	t.Helper()
	criteria := make([]map[string]string, 0, len(baselineCriteria))
	for i, text := range baselineCriteria {
		criteria = append(criteria, map[string]string{"id": fmt.Sprintf("c%d", i+1), "text": text})
	}
	blob, err := json.Marshal(criteria)
	if err != nil {
		t.Fatal(err)
	}
	prompt := fmt.Sprintf(
		"請使用 %s 這個 Skill 完成以下任務，並把產出的檔案寫到 /out/artifacts/。\n\n%s",
		skillName, task)
	var id string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO test_cases (workspace_id, skill_id, name, user_prompt, acceptance_criteria)
		VALUES ($1, $2, 'gen009 baseline', $3, $4::jsonb)
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, skillID), prompt, string(blob),
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// startNoFatal is fixture.start without the t.Fatal: one description failing to
// start is a row in the census, not the end of the batch.
func (f fixture) startNoFatal(t *testing.T) (int, runView) {
	t.Helper()
	hash := f.confirmPermissions(t)
	return f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
}

// waitForTerminalSoft records what the run became instead of failing the batch.
func waitForTerminalSoft(t *testing.T, c *client, runID string, within time.Duration) runView {
	t.Helper()
	deadline := time.Now().Add(within)
	var last runView
	for time.Now().Before(deadline) {
		_, view := c.getRun(t, runID)
		last = view
		switch view.Status {
		case "succeeded", "failed", "cancelled", "timed_out":
			return view
		}
		time.Sleep(2 * time.Second)
	}
	last.Status = "did_not_finish"
	return last
}

// waitForEvaluation polls the production read path. The evaluation is enqueued
// by the run.succeeded/failed consumer, so this is waiting on the outbox and the
// judge, not on a call this test makes.
func waitForEvaluation(t *testing.T, c *client, runID string, within time.Duration) evaluationBody {
	t.Helper()
	deadline := time.Now().Add(within)
	var last evaluationBody
	for time.Now().Before(deadline) {
		status, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
		last = body
		if status == http.StatusOK && (body.Status == "completed" || body.Status == "failed") {
			return body
		}
		time.Sleep(3 * time.Second)
	}
	if last.Status == "" {
		last.Status = "never_appeared"
	}
	return last
}

func writeGen009(t *testing.T, rows []gen009Row) {
	t.Helper()
	out := os.Getenv("GEN009_OUT")
	if out == "" {
		return
	}
	blob, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, append(blob, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = strings.TrimSpace("")
}

// TestGen009YardstickMatchesTheCuratedOne is the only assertion in this file and
// it guards a claim, not a behaviour: report §9 says these numbers sit on the
// same axis as CONTENT-008's because the criteria are the curated ones. That
// claim rests on a hand-copy, and a hand-copy of a string in another language's
// source is exactly the kind of thing this repo has watched drift in silence.
// Unlike the census above this runs every time, with no gate and no money.
func TestGen009YardstickMatchesTheCuratedOne(t *testing.T) {
	const tool = "../../../../../../tools/content/seed_testcases.py"
	src, err := os.ReadFile(tool)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range baselineCriteria {
		if !strings.Contains(string(src), `"`+c+`"`) {
			t.Errorf("criterion is not in %s verbatim, so the yardstick this file "+
				"claims to share with the curated skills is not the same yardstick: %q", tool, c)
		}
	}
}
