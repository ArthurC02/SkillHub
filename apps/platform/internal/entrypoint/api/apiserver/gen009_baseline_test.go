package apiserver_test

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
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

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

	Generated bool     `json:"generated"`
	Blocked   bool     `json:"blocked"`
	Findings  []string `json:"findings,omitempty"`
	SkillName string   `json:"skill_name,omitempty"`
	Attempts  int      `json:"attempts,omitempty"`

	RunStatus    string `json:"run_status,omitempty"`
	FailureClass string `json:"failure_class,omitempty"`

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

	judging := *a.evaluations
	judging.Judge = &llmclient.Client{BaseURL: llmURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	startWorkerWith(t, a.runs, &judging)

	c := a.login(t, "gen009-baseline")
	ws := workspaceOf(t, pool, c)

	rows := make([]gen009Row, 0, len(corpus))
	for i, tc := range corpus {
		row := gen009Row{ID: tc.ID, Group: tc.Group}
		t.Run(tc.ID, func(t *testing.T) {
			res, err := a.versions.GenerateSkill(ctx, ws, ingest.GenerateInput{TaskDescription: tc.Description})
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

func (f fixture) startNoFatal(t *testing.T) (int, runView) {
	t.Helper()
	hash := f.confirmPermissions(t)
	return f.postJSON(t, "/skills/"+f.skillID+"/runs",
		`{"version_id":"`+f.versionID+`","test_case_id":"`+f.testCaseID+
			`","confirmed_summary_hash":"`+hash+`"}`)
}

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
