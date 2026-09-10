package apiserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func creationMeasureLimits() creation.Limits {
	return creation.Limits{
		MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 24, MaxToolCalls: 8,
		CallTimeout: 90 * time.Second, SessionTimeout: 72 * time.Hour,
		Retention: 30 * 24 * time.Hour, MaxOutputTokens: 16000,
	}
}

const creationMeasureLoopBudget = 12

type sessionRow struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Turns          int       `json:"turns"`
	ModelCalls     int       `json:"model_calls"`
	ToolCalls      int       `json:"tool_calls"`
	AutoConfirms   int       `json:"auto_confirms"`
	Clarifications int       `json:"clarifications"`
	CostUSD        *float64  `json:"cost_usd,omitempty"`
	UsageUnknown   bool      `json:"usage_unknown"`
	SecondsPerCall []float64 `json:"seconds_per_call"`
	P50Seconds     float64   `json:"p50_seconds"`
	P95Seconds     float64   `json:"p95_seconds"`
	FinalState     string    `json:"final_state"`
	Draft          bool      `json:"draft"`
	Blocked        bool      `json:"blocked"`
	CriteriaCount  int       `json:"criteria_count"`
	TestCaseID     bool      `json:"test_case_id"`
	Error          string    `json:"error,omitempty"`

	RunStatus  string `json:"run_status,omitempty"`
	EvalStatus string `json:"eval_status,omitempty"`
	Overall    string `json:"overall,omitempty"`

	Met     *bool  `json:"met"`
	MetNote string `json:"met_note,omitempty"`

	RevisedAfterRun *bool `json:"revised_after_run,omitempty"`

	RevisedOverall string `json:"revised_overall,omitempty"`
	RevisedMet     *bool  `json:"revised_met,omitempty"`
	RevisedNote    string `json:"revised_note,omitempty"`

	SearchHit *bool `json:"search_hit,omitempty"`

	CatalogOffers int `json:"catalog_offers,omitempty"`

	DuplicateOffers int    `json:"duplicate_offers,omitempty"`
	SearchNote      string `json:"search_note,omitempty"`

	Rounds   int `json:"rounds,omitempty"`
	MetRound int `json:"met_round,omitempty"`

	MetByOwner  *bool `json:"met_by_owner"`
	KeptByOwner *bool `json:"kept_by_owner"`
}

type singleShotRow struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Generated bool     `json:"generated"`
	Blocked   bool     `json:"blocked"`
	Attempts  int      `json:"attempts"`
	CostUSD   *float64 `json:"cost_usd,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type creationMeasureThresholds struct {
	FormatPassMin int     `json:"format_pass_min"`
	MetMin        int     `json:"met_min"`
	KeptMin       int     `json:"kept_min"`
	CostMedianMax float64 `json:"cost_median_max"`
	P50SecondsMax float64 `json:"p50_seconds_max"`
	P95SecondsMax float64 `json:"p95_seconds_max"`
}
type creationMeasureSummary struct {
	FormatPass int     `json:"format_pass"`
	CostMedian float64 `json:"cost_median"`
	P50Seconds float64 `json:"p50_seconds"`
	P95Seconds float64 `json:"p95_seconds"`

	MetFirstCount         int `json:"met_first_count"`
	MetCount              int `json:"met_count"`
	MetDenominator        int `json:"met_denominator"`
	DiagramMetCount       int `json:"diagram_met_count"`
	DiagramMetDenominator int `json:"diagram_met_denominator"`
}
type creationMeasureResults struct {
	Interactive []sessionRow              `json:"interactive"`
	SingleShot  []singleShotRow           `json:"single_shot"`
	Thresholds  creationMeasureThresholds `json:"thresholds"`
	Summary     creationMeasureSummary    `json:"summary"`
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	idx := int(p/100*float64(len(sorted)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
func median(xs []float64) float64 { return percentile(xs, 50) }

type measureTask struct {
	ID          string
	Kind        string
	Description string
	Diagram     *ingest.GenerateDiagram

	ReferenceMD string
}

func creationMessage(t *testing.T, c *client, v creation.View, message string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": message,
	}, 200)
}

func creationAttachRun(t *testing.T, c *client, v creation.View, runID string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "attach_run", "run_id": runID,
	}, 200)
}

type trialRun struct {
	store *objstore.Client
	pool  *pgxpool.Pool
}

func withTrialRunning(t *testing.T, a *api, pool *pgxpool.Pool, llmURL string, traceSigner *trace.Signer) *trialRun {
	t.Helper()
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if sandboxURL == "" {
		return nil
	}
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
	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = run.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required alongside SKILLHUB_E2E_SANDBOX_URL")
	}
	a.runs.PollInterval = time.Second
	a.runs.MaxAttempts = 1
	a.runs.TraceSigner = traceSigner

	public := httptest.NewUnstartedServer(a.handler)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	public.Listener = listener
	public.Start()
	t.Cleanup(public.Close)
	a.runs.TraceIngestBaseURL = fmt.Sprintf("http://%s:%d",
		os.Getenv("SKILLHUB_E2E_PUBLIC_HOST"), listener.Addr().(*net.TCPAddr).Port)

	judging := *a.evaluations
	judging.Judge = &llmclient.Client{BaseURL: llmURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	startWorkerWith(t, a.runs, &judging)

	return &trialRun{store: store, pool: pool}
}

type trialOutcome struct {
	runID, runStatus, evalStatus, overall, note string
	met                                         *bool
}

func trialCandidate(t *testing.T, a *api, ctx context.Context, c *client, trial *trialRun, candidate *creation.Candidate) trialOutcome {
	t.Helper()
	if candidate == nil {
		return trialOutcome{note: "no candidate to run"}
	}
	if candidate.TestCaseID == "" {
		return trialOutcome{note: "candidate has no test_case_id"}
	}
	var key string
	if err := trial.pool.QueryRow(ctx, "SELECT package_object_key FROM skill_versions WHERE id = $1",
		mustUUID(t, candidate.VersionID)).Scan(&key); err != nil {
		return trialOutcome{note: "package_object_key lookup: " + err.Error()}
	}
	pkg, ok := a.packages[key]
	if !ok {
		return trialOutcome{note: "the candidate package is not in the API's store under " + key}
	}
	if err := trial.store.Put(ctx, key, pkg); err != nil {
		return trialOutcome{note: "put package: " + err.Error()}
	}
	f := fixture{client: c, skillID: candidate.SkillID, versionID: candidate.VersionID, testCaseID: candidate.TestCaseID}
	code, rv := f.startNoFatal(t)
	if code != http.StatusCreated && code != http.StatusOK {
		return trialOutcome{note: fmt.Sprintf("POST run: %d %s", code, rv.Error)}
	}
	out := trialOutcome{runID: rv.RunID}
	final := waitForTerminalSoft(t, c, rv.RunID, 8*time.Minute)
	out.runStatus = final.Status
	ev := waitForEvaluation(t, c, rv.RunID, 4*time.Minute)
	out.evalStatus = ev.Status
	out.overall = ev.Overall
	switch ev.Status {
	case "completed", "failed":
		met := ev.Overall == "met"
		out.met = &met
	default:
		out.note = "evaluation did not finish: " + ev.Status
	}
	return out
}

func attachTrialRun(t *testing.T, a *api, ctx context.Context, c *client, s *creation.Service, trial *trialRun, v creation.View, row sessionRow, outDir string) (sessionRow, creation.View) {
	t.Helper()
	rounds := 3
	if n, err := strconv.Atoi(os.Getenv("CREATION_MEASURE_ROUNDS")); err == nil && n > 0 {
		rounds = n
	}
	last := trialCandidate(t, a, ctx, c, trial, v.Snapshot.Candidate)
	row.RunStatus, row.EvalStatus, row.Overall, row.Met, row.MetNote = last.runStatus, last.evalStatus, last.overall, last.met, last.note
	if last.runID == "" {
		return row, v
	}
	row.Rounds = 1
	if last.met != nil && *last.met {
		row.MetRound = 1
	}
	for round := 2; round <= rounds && row.MetRound == 0; round++ {
		beforeHash := ""
		if v.Snapshot.Draft != nil {
			beforeHash = v.Snapshot.Draft.ContentHash
		}
		v = creationAttachRun(t, c, v, last.runID)
		answered := 0

		for i := 0; i < creation.MaxNudges+10; i++ {
			switch v.State {
			case "queued":
				v = creationStep(t, s, v)
				row.ModelCalls++
				continue
			case "waiting_confirmation":
				if v.Snapshot.PendingAction == "" {
					break
				}
				v = creationAct(t, c, v, v.Snapshot.PendingAction)
				row.AutoConfirms++
				continue
			case "waiting_input":

				if answered >= 2 {
					break
				}
				answered++
				row.Clarifications++
				v = creationMessage(t, c, v, "照沒過的條件改草稿；條件或範例輸入驗不到的，就改條件或範例輸入。")
				continue
			}
			break
		}
		afterHash := ""
		if v.Snapshot.Draft != nil {
			afterHash = v.Snapshot.Draft.ContentHash
		}
		revised := beforeHash != afterHash
		if round == 2 {
			row.RevisedAfterRun = &revised
		}
		if !revised {
			break
		}
		if v.State != "draft_ready" {
			row.RevisedNote = "revised draft did not settle: " + v.State
			break
		}

		v = materializeThrough(t, c, v, &row)
		if v.Snapshot.Draft != nil {
			dumpDraftMD(t, outDir, row.ID, fmt.Sprintf("interactive-r%d", round), v.Snapshot.Draft.Skill.Name, v.Snapshot.Draft.Skill.Description, v.Snapshot.Draft.Skill.Body)
		}
		last = trialCandidate(t, a, ctx, c, trial, v.Snapshot.Candidate)
		row.RevisedOverall, row.RevisedMet, row.RevisedNote = last.overall, last.met, last.note
		if last.runID == "" {
			break
		}
		row.Rounds = round
		if last.met != nil && *last.met {
			row.MetRound = round
		}
	}
	return row, v
}

func revisedMetLabel(row sessionRow) string {
	if row.RevisedMet == nil {
		if row.RevisedNote != "" {
			return "n/a (" + row.RevisedNote + ")"
		}
		return "n/a"
	}
	return fmt.Sprintf("%v (%s, rounds=%d, met_round=%d)", *row.RevisedMet, row.RevisedOverall, row.Rounds, row.MetRound)
}

func TestCreationMeasureFifteenSessionsAgainstSingleShot(t *testing.T) {
	corpusPath := os.Getenv("CREATION_MEASURE_CORPUS")
	diagramDir := os.Getenv("CREATION_MEASURE_DIAGRAMS")
	outDir := os.Getenv("CREATION_MEASURE_OUT")
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	gatewayKey := os.Getenv("LITELLM_API_KEY")
	if corpusPath == "" || diagramDir == "" || outDir == "" || base == "" || gatewayKey == "" {
		t.Skip("set CREATION_MEASURE_CORPUS, CREATION_MEASURE_DIAGRAMS, CREATION_MEASURE_OUT, SKILLHUB_E2E_LLM_URL and LITELLM_API_KEY; this test spends money")
	}
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus modesCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}

	textOnly := len(corpus.Diagram) < 5 || len(corpus.Reference) < 10
	if textOnly && len(corpus.Reference) == 0 {
		t.Fatal("corpus has no tasks")
	}
	pool := requireDB(t)

	llm := &llmclient.Client{BaseURL: base, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	limits := creationMeasureLimits()
	set, err := worker.BuildWorkers(pool, worker.Deps{CreationLimits: limits, LLM: llm})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.IssueKey = func(context.Context, string, string, float64, time.Duration) (string, error) {
		return gatewayKey, nil
	}
	set.Creation.RevokeKey = func(context.Context, string) error { return nil }
	transient := httptest.NewServer(set.Creation.TransientHandler("creation-measure"))
	t.Cleanup(transient.Close)
	packages := packageStore{}

	traceSigner := &trace.Signer{Secret: []byte("creation-measure-trace-secret")}
	app, err := apiserver.NewApp(apiserver.Config{
		Pool: pool, Store: packages, LLM: llm, OAuth: &identity.GitHubOAuth{}, DevLogin: true,
		GenerateExposed: true, CreationExposed: true, CreationLimits: limits,
		CreationTransient: creation.TransientClient(transient.URL, "creation-measure", 95*time.Second),
		TraceSigner:       traceSigner,
	})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.ResolveReference = app.CreationSvc.ResolveReference
	handler := app.Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a := &api{
		Server: server, auth: app.Auth, app: app, packages: packages, handler: handler,
		versions: app.Versions, runs: app.RunSvc, evaluations: app.EvalSvc, traceSigner: traceSigner,
	}
	ctx := context.Background()
	trial := withTrialRunning(t, a, pool, base, traceSigner)

	var tasks []measureTask
	if textOnly {
		for _, r := range corpus.Reference {
			tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description})
		}
	}
	for i := 0; i < 5 && !textOnly; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description})
	}
	for i := 0; i < 5 && !textOnly; i++ {
		d := corpus.Diagram[i]
		ext, mediaType := d.Media, "image/png"
		if ext == "jpg" {
			mediaType = "image/jpeg"
		}
		img, err := os.ReadFile(filepath.Join(diagramDir, d.ID+"."+ext))
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, measureTask{ID: d.ID, Kind: "diagram", Diagram: &ingest.GenerateDiagram{MediaType: mediaType, Data: img}})
	}
	for i := 5; i < 10 && !textOnly; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "reference", Description: r.Description, ReferenceMD: r.Reference.SkillMD})
	}

	var results creationMeasureResults
	results.Thresholds = creationMeasureThresholds{
		FormatPassMin: 14, MetMin: 6, KeptMin: 12,
		CostMedianMax: 0.5, P50SecondsMax: 60, P95SecondsMax: 90,
	}
	flush := func() {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "results.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, task := range tasks {
		row := runInteractiveSession(t, a, set.Creation, ctx, task, limits, outDir, trial, llm)
		results.Interactive = append(results.Interactive, row)
		flush()
		t.Logf("interactive %s (%s): state=%s draft=%v cost=%s met=%s revised_met=%s search_hit=%s calls=%d", task.ID, task.Kind, row.FinalState, row.Draft, costLabel(row.CostUSD), metLabel(row), revisedMetLabel(row), searchLabel(row), row.ModelCalls)
	}
	for _, task := range tasks {
		row := runSingleShot(t, a, ctx, task, outDir)
		results.SingleShot = append(results.SingleShot, row)
		flush()
		t.Logf("single-shot %s (%s): generated=%v attempts=%d cost=%s", task.ID, task.Kind, row.Generated, row.Attempts, costLabel(row.CostUSD))
	}

	for _, row := range results.Interactive {
		if row.Draft && !row.Blocked {
			results.Summary.FormatPass++
		}
		if row.Met != nil {
			within := row.MetRound > 0
			if row.Kind == "diagram" {
				results.Summary.DiagramMetDenominator++
				if within {
					results.Summary.DiagramMetCount++
				}
				continue
			}
			results.Summary.MetDenominator++
			if *row.Met {
				results.Summary.MetFirstCount++
			}
			if within {
				results.Summary.MetCount++
			}
		}
	}
	for _, row := range results.SingleShot {
		if row.Generated {
			results.Summary.FormatPass++
		}
	}
	var costs, allSeconds []float64
	for _, row := range results.Interactive {
		if row.CostUSD != nil {
			costs = append(costs, *row.CostUSD)
		}
		allSeconds = append(allSeconds, row.SecondsPerCall...)
	}
	results.Summary.CostMedian = median(costs)
	results.Summary.P50Seconds = percentile(allSeconds, 50)
	results.Summary.P95Seconds = percentile(allSeconds, 95)
	flush()

	if len(results.Interactive) != len(tasks) || len(results.SingleShot) != len(tasks) {
		t.Fatalf("expected %d+%d rows, got %d+%d", len(tasks), len(tasks), len(results.Interactive), len(results.SingleShot))
	}
}

func dumpDraftMD(t *testing.T, outDir, id, suffix, name, description, body string) {
	t.Helper()
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(outDir, id+"-"+suffix+".SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
}

var measureRunNonce = time.Now().UTC().Format("0102-150405")

func runInteractiveSession(t *testing.T, a *api, s *creation.Service, ctx context.Context, task measureTask, limits creation.Limits, outDir string, trial *trialRun, llm *llmclient.Client) sessionRow {
	t.Helper()
	row := sessionRow{ID: task.ID, Kind: task.Kind}

	c := a.login(t, "creation-measure-"+measureRunNonce+"-"+strings.ToLower(task.ID))

	initialMessage := task.Description
	if task.Kind == "diagram" {
		initialMessage = ""
	}

	var refID string
	if task.Kind == "reference" {

		refID, _ = importFilesEnriched(t, a, testPool, c, map[string]string{"SKILL.md": task.ReferenceMD}, llm)
		markCatalog(t, testPool, c.workspaceID)
	}
	v := creationPost(t, c, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": initialMessage, "budget_usd": limits.MaxCostUSD,
	}, 200)

	defer func() {
		data, err := json.MarshalIndent(map[string]any{"state": v.State, "brief": v.Snapshot.Brief, "acceptance_criteria": v.Snapshot.AcceptanceCriteria, "messages": v.Snapshot.Messages}, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(outDir, task.ID+"-interactive.transcript.json"), data, 0o600)
		}
	}()

	if task.Kind == "diagram" {
		encoded := base64.StdEncoding.EncodeToString(task.Diagram.Data)
		v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
			"diagram": map[string]string{"media_type": task.Diagram.MediaType, "data": encoded},
		}, 200)
		row.ModelCalls++
	}
	if task.Kind == "reference" {

		for v.State == "queued" {
			v = creationStep(t, s, v)
			row.ModelCalls++
		}
		if os.Getenv("CREATION_MEASURE_SEARCH") == "1" {

			searched := v.Snapshot.CatalogChecked
			for _, m := range v.Snapshot.Messages {
				if m.Role == "tool" && strings.Contains(m.Content, "目錄") {
					searched = true
				}
			}
			if v.State == "waiting_confirmation" && v.Snapshot.PendingAction == "confirm_references" {
				searched = true
			}
			if !searched {
				row.SearchNote = "model did not search"
			} else {
				hit := false
				if v.State == "waiting_confirmation" && v.Snapshot.PendingAction == "confirm_references" {
					for _, ref := range v.Snapshot.References {
						if ref.SkillID == refID {
							hit = true
						}
					}
				}
				row.SearchHit = &hit
			}
		} else {
			v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
				"command_id": creationID(t), "expected_revision": v.Revision, "kind": "select_references",
				"reference_skill_ids": []string{refID},
			}, 200)
			v = creationAct(t, c, v, "confirm_references")
		}
	}

	clarifications := 0
	for i := 0; i < creationMeasureLoopBudget; i++ {
		switch v.State {
		case "saved", "failed", "cancelled", "needs_reupload":
			row.FinalState = v.State
			return finishSession(t, v, row, outDir)
		case "queued":
			start := time.Now()
			v = creationStep(t, s, v)
			row.SecondsPerCall = append(row.SecondsPerCall, time.Since(start).Seconds())
			row.ModelCalls++
			if v.Snapshot.SpentUSD != nil {
				row.CostUSD = v.Snapshot.SpentUSD
			}
			row.UsageUnknown = v.Snapshot.UsageUnknown
			row.ToolCalls = v.Snapshot.ToolCalls
		case "waiting_confirmation":
			kind := v.Snapshot.PendingAction
			if kind == "" {
				row.FinalState = v.State
				row.Error = "waiting_confirmation with no pending action"
				return finishSession(t, v, row, outDir)
			}
			if kind == "confirm_references" && task.Kind != "reference" {

				row.CatalogOffers = len(v.Snapshot.References)
				kind = "decline_references"
			}

			code, body := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind, "content_hash": func() string {
				if v.Snapshot.Draft == nil {
					return ""
				}
				return v.Snapshot.Draft.ContentHash
			}()})
			if code != 200 {
				row.FinalState = v.State
				row.Error = fmt.Sprintf("%s refused: %d %s", kind, code, body)
				return finishSession(t, v, row, outDir)
			}
			if err := json.Unmarshal([]byte(body), &v); err != nil {
				t.Fatal(err)
			}
			row.AutoConfirms++
		case "waiting_input":
			if clarifications >= 2 {
				row.FinalState = v.State
				return finishSession(t, v, row, outDir)
			}
			clarifications++
			row.Clarifications++
			v = creationMessage(t, c, v, "請依合理假設補上缺的資訊，然後繼續。")
		case "draft_ready":
			v = materializeThrough(t, c, v, &row)
			row.FinalState = v.State
			row = finishSession(t, v, row, outDir)
			if trial != nil {

				row, v = attachTrialRun(t, a, ctx, c, s, trial, v, row, outDir)
			}
			return row
		default:
			row.FinalState = v.State
			row.Error = "unexpected state: " + v.State
			return finishSession(t, v, row, outDir)
		}
		row.Turns++
	}
	row.FinalState = v.State
	row.Error = "loop budget exhausted"
	return finishSession(t, v, row, outDir)
}

func materializeThrough(t *testing.T, c *client, v creation.View, row *sessionRow) creation.View {
	t.Helper()

	act := func(kind string) bool {
		code, body := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind, "content_hash": v.Snapshot.Draft.ContentHash})
		if code != 200 {
			row.Error = fmt.Sprintf("%s refused: %d %s", kind, code, body)
			return false
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatal(err)
		}
		return true
	}
	if !act("materialize") {
		return v
	}
	if v.State == "waiting_confirmation" && v.Snapshot.PendingAction == "confirm_duplicate" {
		row.DuplicateOffers += len(v.Snapshot.Duplicates)
		act("confirm_duplicate")
	}
	return v
}

func finishSession(t *testing.T, v creation.View, row sessionRow, outDir string) sessionRow {
	t.Helper()
	row.CriteriaCount = len(v.Snapshot.AcceptanceCriteria)
	if v.Snapshot.Draft != nil {
		row.Draft = true
		row.Blocked = v.Snapshot.Draft.Blocked
		dumpDraftMD(t, outDir, row.ID, "interactive", v.Snapshot.Draft.Skill.Name, v.Snapshot.Draft.Skill.Description, v.Snapshot.Draft.Skill.Body)
	}
	if v.Snapshot.Candidate != nil && v.Snapshot.Candidate.TestCaseID != "" {
		row.TestCaseID = true
	}
	return row
}

func runSingleShot(t *testing.T, a *api, ctx context.Context, task measureTask, outDir string) singleShotRow {
	t.Helper()
	row := singleShotRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-single-"+measureRunNonce+"-"+strings.ToLower(task.ID))
	ws := workspaceOf(t, testPool, c)

	in := ingest.GenerateInput{TaskDescription: task.Description}
	switch task.Kind {
	case "diagram":
		in.Diagram = task.Diagram
	case "reference":
		refID, _ := importFiles(t, a, testPool, c, map[string]string{"SKILL.md": task.ReferenceMD})
		in.ReferenceSkillIDs = []pgtype.UUID{mustUUID(t, refID)}
	}
	res, err := a.versions.GenerateSkill(ctx, ws, in)
	if err != nil {
		row.Error = err.Error()
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	row.Attempts, row.CostUSD = res.Attempts, res.CostUSD
	if res.Report.Blocked {
		row.Blocked = true
		return row
	}
	row.Generated = true
	data, err := a.packages.Get(ctx, res.Version.PackageObjectKey)
	if err != nil {
		t.Logf("single-shot %s: stored package unreadable: %v", task.ID, err)
		return row
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	md, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	if err := os.WriteFile(filepath.Join(outDir, task.ID+"-single.SKILL.md"), md, 0o600); err != nil {
		t.Fatal(err)
	}
	return row
}

func costLabel(c *float64) string {
	if c == nil {
		return "unknown"
	}
	return fmt.Sprintf("$%.4f", *c)
}

func metLabel(row sessionRow) string {
	if row.Met != nil {
		return fmt.Sprintf("%v (%s)", *row.Met, row.Overall)
	}
	if row.MetNote != "" {
		return "null: " + row.MetNote
	}
	return "null"
}

func boolLabel(b *bool) string {
	if b == nil {
		return "n/a"
	}
	return fmt.Sprintf("%v", *b)
}

func searchLabel(row sessionRow) string {
	if row.SearchNote != "" {
		return "n/a (" + row.SearchNote + ")"
	}
	return boolLabel(row.SearchHit)
}
