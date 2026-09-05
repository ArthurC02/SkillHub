package apiserver_test

// 02:GEN-012 / 05 R-45 — the paid measurement harness for the interactive
// creation journey against the single-shot generate path. 15 multi-turn
// sessions (5 text, 5 diagram, 5 reference), the same 15 tasks through
// versions.GenerateSkill once each. See
// docs/plans/mvp/m5/creation-measure/README.md for how to run this and what
// the owner still has to do afterward (attach a Run for "met", read the
// drafts for "kept" — this harness cannot do either).
//
// Usage (spends money — one command runs everything):
//
//	CREATION_MEASURE_CORPUS    docs/plans/mvp/m5/gen-modes-batch/corpus.json
//	CREATION_MEASURE_DIAGRAMS  dir with D01.png... (pwsh .../draw.ps1 -Corpus ... -OutDir ...)
//	CREATION_MEASURE_OUT       dir for results.json and *.SKILL.md
//	SKILLHUB_E2E_LLM_URL       a running apps/llm pointed at a real gateway
//	LITELLM_API_KEY            the creation gateway key (see with-service-key.mjs)

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

// creationMeasureLimits is 05 R-45's ruled values (裁定 2026-09-06).
func creationMeasureLimits() creation.Limits {
	return creation.Limits{
		MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 24, MaxToolCalls: 8,
		CallTimeout: 90 * time.Second, SessionTimeout: 72 * time.Hour,
		Retention: 30 * 24 * time.Hour, MaxOutputTokens: 16000,
	}
}

// creationMeasureLoopBudget bounds one interactive session's driver loop: a
// stuck session (a model that never confirms, never drafts) must stop rather
// than spend the whole per-session ceiling in a tight loop.
const creationMeasureLoopBudget = 12

// sessionRow is one interactive session's outcome (results.json "interactive").
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
	// MetByOwner/KeptByOwner are left null for the owner to fill in after a Run
	// is attached (met) and a person has read the SKILL.md (kept) — the two
	// halves of 05 R-45's threshold this harness cannot produce on its own.
	MetByOwner  *bool `json:"met_by_owner"`
	KeptByOwner *bool `json:"kept_by_owner"`
}

// singleShotRow is one single-shot generation's outcome ("single_shot").
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
}
type creationMeasureResults struct {
	Interactive []sessionRow              `json:"interactive"`
	SingleShot  []singleShotRow           `json:"single_shot"`
	Thresholds  creationMeasureThresholds `json:"thresholds"`
	Summary     creationMeasureSummary    `json:"summary"`
}

// percentile is the nearest-rank percentile over a sorted copy of xs; p in
// [0,100]. Good enough at the sizes here (15 sessions, up to a few dozen
// per-call seconds), not a statistics library.
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

// measureTask is one of the 15 tasks driven both ways.
type measureTask struct {
	ID          string
	Kind        string // "text" | "diagram" | "reference"
	Description string
	Diagram     *ingest.GenerateDiagram
	// ReferenceMD is the reference skill's SKILL.md, imported per-session so
	// each session's reference resolves against its own workspace.
	ReferenceMD string
}

// creationMessage posts a "message" command, unlike creationAct which never
// carries a message body.
func creationMessage(t *testing.T, c *client, v creation.View, message string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": message,
	}, 200)
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
	if len(corpus.Diagram) < 5 || len(corpus.Reference) < 10 {
		t.Fatal("corpus does not have enough items for 5 diagram + 5 text + 5 reference tasks")
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
	app, err := apiserver.NewApp(apiserver.Config{
		Pool: pool, Store: packages, LLM: llm, OAuth: &identity.GitHubOAuth{}, DevLogin: true,
		GenerateExposed: true, CreationExposed: true, CreationLimits: limits,
		CreationTransient: creation.TransientClient(transient.URL, "creation-measure", 95*time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.ResolveReference = app.CreationSvc.ResolveReference
	handler := app.Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a := &api{Server: server, auth: app.Auth, app: app, packages: packages, handler: handler, versions: app.Versions}
	ctx := context.Background()

	// Build the 15 tasks: text = reference[0..4] (description only), diagram =
	// diagram[0..4], reference = reference[5..9] (description + its own
	// reference skill).
	var tasks []measureTask
	for i := 0; i < 5; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description})
	}
	for i := 0; i < 5; i++ {
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
	for i := 5; i < 10; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "reference", Description: r.Description, ReferenceMD: r.Reference.SkillMD})
	}

	var results creationMeasureResults
	results.Thresholds = creationMeasureThresholds{
		FormatPassMin: 14, MetMin: 9, KeptMin: 12,
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
		row := runInteractiveSession(t, a, set.Creation, ctx, task, limits, outDir)
		results.Interactive = append(results.Interactive, row)
		flush()
		t.Logf("interactive %s (%s): state=%s draft=%v cost=%v calls=%d", task.ID, task.Kind, row.FinalState, row.Draft, row.CostUSD, row.ModelCalls)
	}
	for _, task := range tasks {
		row := runSingleShot(t, a, ctx, task, outDir)
		results.SingleShot = append(results.SingleShot, row)
		flush()
		t.Logf("single-shot %s (%s): generated=%v attempts=%d cost=%v", task.ID, task.Kind, row.Generated, row.Attempts, row.CostUSD)
	}

	for _, row := range results.Interactive {
		if row.Draft && !row.Blocked {
			results.Summary.FormatPass++
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

	if len(results.Interactive) != 15 || len(results.SingleShot) != 15 {
		t.Fatalf("expected 15+15 rows, got %d+%d", len(results.Interactive), len(results.SingleShot))
	}
}

// dumpDraftMD writes a generated skill's markdown (frontmatter + body) to
// <outDir>/<id>-<suffix>.SKILL.md, for the person who has to read 30 of these
// for "kept" (05 R-45).
func dumpDraftMD(t *testing.T, outDir, id, suffix, name, description, body string) {
	t.Helper()
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(outDir, id+"-"+suffix+".SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runInteractiveSession drives one multi-turn session to a terminal state (or
// until the loop/message budget runs out), materializing a draft if one is
// reached, and dumps the resulting draft.
func runInteractiveSession(t *testing.T, a *api, s *creation.Service, ctx context.Context, task measureTask, limits creation.Limits, outDir string) sessionRow {
	t.Helper()
	row := sessionRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-"+strings.ToLower(task.ID))

	initialMessage := task.Description
	if task.Kind == "diagram" {
		initialMessage = "請依這張流程圖建立 Skill。"
	}
	v := creationPost(t, c, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": initialMessage, "budget_usd": limits.MaxCostUSD,
	}, 200)

	if task.Kind == "diagram" {
		encoded := base64.StdEncoding.EncodeToString(task.Diagram.Data)
		v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
			"diagram": map[string]string{"media_type": task.Diagram.MediaType, "data": encoded},
		}, 200)
		row.ModelCalls++ // the diagram action is one synchronous model call, outside the queued loop below
	}
	if task.Kind == "reference" {
		refID, _ := importFiles(t, a, testPool, c, map[string]string{"SKILL.md": task.ReferenceMD})
		v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "select_references",
			"reference_skill_ids": []string{refID},
		}, 200)
		v = creationAct(t, c, v, "confirm_references")
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
			v = creationAct(t, c, v, kind)
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
			v = creationAct(t, c, v, "materialize")
			row.FinalState = v.State
			return finishSession(t, v, row, outDir)
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

// finishSession fills in the draft-derived fields from the session's final
// snapshot and dumps the draft body, if any.
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

// runSingleShot calls versions.GenerateSkill once for the same task, the
// comparison arm (02:GEN-012).
func runSingleShot(t *testing.T, a *api, ctx context.Context, task measureTask, outDir string) singleShotRow {
	t.Helper()
	row := singleShotRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-single-"+strings.ToLower(task.ID))
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
